package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	consensusv1 "github.com/cometbft/cometbft/api/cometbft/consensus/v1"
	cryptov1 "github.com/cometbft/cometbft/api/cometbft/crypto/v1"
	bitsv1 "github.com/cometbft/cometbft/api/cometbft/libs/bits/v1"
	statesyncv1 "github.com/cometbft/cometbft/api/cometbft/statesync/v1"
	typesv1 "github.com/cometbft/cometbft/api/cometbft/types/v1"
	"github.com/cometbft/cometbft/config"
	cs "github.com/cometbft/cometbft/internal/consensus"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/libs/service"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/p2p/conn"
	"github.com/cometbft/cometbft/statesync"
)

const (
	controllerTestLocalID  = p2p.ID("1111111111111111111111111111111111111111")
	controllerTestRemoteID = p2p.ID("2222222222222222222222222222222222222222")
)

type controllerTestReactor struct {
	p2p.BaseReactor
	channels []*conn.ChannelDescriptor
	received chan p2p.Envelope
	entered  chan struct{}
	release  chan struct{}
}

func newControllerTestReactor(channels []*conn.ChannelDescriptor) *controllerTestReactor {
	reactor := &controllerTestReactor{channels: channels, received: make(chan p2p.Envelope, 32)}
	reactor.BaseReactor = *p2p.NewBaseReactor("controller-test", reactor)
	return reactor
}

func (r *controllerTestReactor) GetChannels() []*conn.ChannelDescriptor { return r.channels }
func (*controllerTestReactor) AddPeer(p2p.Peer)                         {}
func (*controllerTestReactor) RemovePeer(p2p.Peer, any)                 {}
func (r *controllerTestReactor) Receive(envelope p2p.Envelope) {
	if r.entered != nil {
		r.entered <- struct{}{}
		<-r.release
	}
	r.received <- envelope
}

type controllerTestPeer struct {
	service.BaseService
	id      p2p.ID
	mu      sync.Mutex
	data    map[string]any
	removed bool
}

func newControllerTestPeer(t *testing.T, id p2p.ID) *controllerTestPeer {
	t.Helper()
	peer := &controllerTestPeer{id: id, data: make(map[string]any)}
	peer.BaseService = *service.NewBaseService(log.NewNopLogger(), "controller-test-peer", peer)
	require.NoError(t, peer.Start())
	t.Cleanup(func() { _ = peer.Stop() })
	return peer
}

func (*controllerTestPeer) FlushStop()                    {}
func (p *controllerTestPeer) ID() p2p.ID                  { return p.id }
func (*controllerTestPeer) RemoteIP() net.IP              { return net.ParseIP("127.0.0.1") }
func (*controllerTestPeer) RemoteAddr() net.Addr          { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1")} }
func (*controllerTestPeer) IsOutbound() bool              { return false }
func (*controllerTestPeer) IsPersistent() bool            { return false }
func (*controllerTestPeer) CloseConn() error              { return nil }
func (*controllerTestPeer) NodeInfo() p2p.NodeInfo        { return nil }
func (*controllerTestPeer) Status() conn.ConnectionStatus { return conn.ConnectionStatus{} }
func (*controllerTestPeer) SocketAddr() *p2p.NetAddress   { return nil }
func (*controllerTestPeer) Send(p2p.Envelope) bool        { return true }
func (*controllerTestPeer) TrySend(p2p.Envelope) bool     { return true }
func (p *controllerTestPeer) Set(key string, value any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.data[key] = value
}

func (p *controllerTestPeer) Get(key string) any {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.data[key]
}

func (p *controllerTestPeer) SetRemovalFailed() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.removed = true
}

func (p *controllerTestPeer) GetRemovalFailed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.removed
}

func controllerTestRegistry(t *testing.T) (*p2p.NativeRouteRegistry, *controllerTestReactor, *p2p.PeerSet) {
	t.Helper()
	sw := p2p.NewSwitch(config.DefaultP2PConfig(), nil)
	registrations := append(
		append([]*conn.ChannelDescriptor{}, (*cs.Reactor)(nil).GetChannels()...),
		(*statesync.Reactor)(nil).GetChannels()...,
	)
	reactor := newControllerTestReactor(registrations)
	sw.AddReactor("SELECTED-PRODUCTION-REGISTRATIONS", reactor)
	registry, err := sw.FreezeNativeRoutes(selectedControllerRoutes)
	require.NoError(t, err)
	return registry, reactor, p2p.NewPeerSet()
}

func controllerTestConfig(controllerAddress string) *config.ControllerMediationConfig {
	cfg := config.DefaultControllerMediationConfig()
	cfg.Enabled = true
	cfg.ControllerAddress = controllerAddress
	cfg.CallbackListenAddress = "127.0.0.1:0"
	cfg.CallbackAdvertiseAddress = "127.0.0.1:31001"
	cfg.NodeID = 1
	cfg.RegistrationAttempts = 1
	cfg.RegistrationRetryDelay = time.Millisecond
	cfg.RequestTimeout = 500 * time.Millisecond
	cfg.SendTimeout = 100 * time.Millisecond
	cfg.ShutdownTimeout = time.Second
	return cfg
}

func newControllerTestService(t *testing.T, handler http.HandlerFunc) (*controllerMediation, *controllerTestReactor, *controllerTestPeer) {
	t.Helper()
	controller := httptest.NewServer(handler)
	t.Cleanup(controller.Close)
	registry, reactor, peers := controllerTestRegistry(t)
	peer := newControllerTestPeer(t, controllerTestRemoteID)
	require.NoError(t, peers.Add(peer))
	mediation := newControllerMediation(controllerTestConfig(strings.TrimPrefix(controller.URL, "http://")), controllerTestLocalID,
		registry, peers, p2p.NopMetrics(), log.NewNopLogger(), nil)
	require.NoError(t, mediation.Start())
	t.Cleanup(mediation.Stop)
	return mediation, reactor, peer
}

func controllerCallbackBodyForMessage(t *testing.T, id string, message proto.Message) []byte {
	t.Helper()
	label, route, selected := selectedMessageBinding(message)
	require.True(t, selected)
	wrapped, err := proto.Marshal(message.(interface{ Wrap() proto.Message }).Wrap())
	require.NoError(t, err)
	payload, err := encodeAdapterPayload(route, wrapped)
	require.NoError(t, err)
	messageID := fmt.Sprintf("%s_%s_%s", controllerTestRemoteID, controllerTestLocalID, id)
	body, err := json.Marshal(controllerMessage{From: string(controllerTestRemoteID), To: string(controllerTestLocalID), Data: payload, Type: label, ID: messageID})
	require.NoError(t, err)
	return body
}

func controllerCallbackBody(t *testing.T, id string) []byte {
	t.Helper()
	return controllerCallbackBodyForMessage(t, id, &statesyncv1.SnapshotsRequest{})
}

func postControllerCallback(t *testing.T, mediation *controllerMediation, body []byte) int {
	t.Helper()
	response, err := http.Post("http://"+mediation.listener.Addr().String()+"/message", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

type controllerInventoryCase struct {
	name    string
	message proto.Message
	route   byte
}

func controllerInventoryCases() []controllerInventoryCase {
	bitArray := bitsv1.BitArray{Bits: 1, Elems: []uint64{1}}
	partSetHeader := typesv1.PartSetHeader{Total: 1, Hash: bytes.Repeat([]byte{1}, 32)}
	proposal := typesv1.Proposal{
		Type: typesv1.SignedMsgType(32), Height: 1, Round: 0, PolRound: -1,
		BlockID:   typesv1.BlockID{Hash: bytes.Repeat([]byte{5}, 32), PartSetHeader: partSetHeader},
		Signature: []byte{6},
	}
	part := typesv1.Part{Index: 0, Bytes: []byte{1}, Proof: cryptov1.Proof{Total: 1, Index: 0, LeafHash: bytes.Repeat([]byte{2}, 32)}}
	vote := func(messageType typesv1.SignedMsgType) *consensusv1.Vote {
		return &consensusv1.Vote{Vote: &typesv1.Vote{
			Type: messageType, Height: 1, Round: 0,
			ValidatorAddress: bytes.Repeat([]byte{3}, 20), ValidatorIndex: 0, Signature: []byte{4},
		}}
	}
	return []controllerInventoryCase{
		{name: "round-state", route: cs.StateChannel, message: &consensusv1.NewRoundStep{Height: 1, Round: 0, Step: 1, LastCommitRound: -1}},
		{name: "new-valid-block", route: cs.StateChannel, message: &consensusv1.NewValidBlock{Height: 1, Round: 0, BlockPartSetHeader: partSetHeader, BlockParts: &bitArray}},
		{name: "has-vote", route: cs.StateChannel, message: &consensusv1.HasVote{Height: 1, Round: 0, Type: typesv1.SignedMsgType(1), Index: 0}},
		{name: "has-proposal-block-part", route: cs.StateChannel, message: &consensusv1.HasProposalBlockPart{Height: 1, Round: 0, Index: 0}},
		{name: "vote-set-majority", route: cs.StateChannel, message: &consensusv1.VoteSetMaj23{Height: 1, Round: 0, Type: typesv1.SignedMsgType(1)}},
		{name: "proposal", route: cs.DataChannel, message: &consensusv1.Proposal{Proposal: proposal}},
		{name: "proposal-pol", route: cs.DataChannel, message: &consensusv1.ProposalPOL{Height: 1, ProposalPolRound: 0, ProposalPol: bitArray}},
		{name: "block-part", route: cs.DataChannel, message: &consensusv1.BlockPart{Height: 1, Round: 0, Part: part}},
		{name: "prevote-nil-block", route: cs.VoteChannel, message: vote(typesv1.SignedMsgType(1))},
		{name: "precommit-nil-block", route: cs.VoteChannel, message: vote(typesv1.SignedMsgType(2))},
		{name: "vote-set-bits", route: cs.VoteSetBitsChannel, message: &consensusv1.VoteSetBits{Height: 1, Round: 0, Type: typesv1.SignedMsgType(1), Votes: bitArray}},
		{name: "snapshot-request", route: statesync.SnapshotChannel, message: &statesyncv1.SnapshotsRequest{}},
		{name: "snapshot-response", route: statesync.SnapshotChannel, message: &statesyncv1.SnapshotsResponse{Height: 1, Format: 1, Chunks: 1, Hash: []byte{1}}},
		{name: "chunk-request", route: statesync.ChunkChannel, message: &statesyncv1.ChunkRequest{Height: 1, Format: 1, Index: 0}},
		{name: "chunk-response", route: statesync.ChunkChannel, message: &statesyncv1.ChunkResponse{Height: 1, Format: 1, Index: 0, Chunk: []byte{1}}},
	}
}

func TestControllerMediationAdapterInventoryRoundTrip(t *testing.T) {
	registry, _, _ := controllerTestRegistry(t)
	productionRoutes := append([]byte{}, selectedControllerRoutes...)
	require.ElementsMatch(t, productionRoutes, registry.Routes())
	require.Len(t, controllerInventoryCases(), 15)

	seen := make(map[byte]int)
	for _, test := range controllerInventoryCases() {
		t.Run(test.name, func(t *testing.T) {
			label, route, selected := selectedMessageBinding(test.message)
			require.True(t, selected, "%T", test.message)
			require.NotEmpty(t, label)
			require.Equal(t, test.route, route)
			require.NoError(t, validateSelectedMessage(test.message))
			wrapped, err := proto.Marshal(test.message.(interface{ Wrap() proto.Message }).Wrap())
			require.NoError(t, err)
			payload, err := encodeAdapterPayload(route, wrapped)
			require.NoError(t, err)
			adapterDecoded, err := decodeAdapterPayload(payload)
			require.NoError(t, err)
			nativeDecoded, err := registry.Decode(adapterDecoded.ChannelID, adapterDecoded.Wrapped)
			require.NoError(t, err)
			require.IsType(t, test.message, nativeDecoded)
			require.NoError(t, validateSelectedMessage(nativeDecoded))
			nativeWrapped, err := proto.Marshal(nativeDecoded.(interface{ Wrap() proto.Message }).Wrap())
			require.NoError(t, err)
			require.Equal(t, wrapped, nativeWrapped)
			seen[route]++
		})
	}
	for _, route := range productionRoutes {
		require.NotZero(t, seen[route], "registered route %X has no valid representative", route)
	}
}

func TestControllerMediationCallbackOrderAndDuplication(t *testing.T) {
	submittedBodies := make(chan []byte, 3)
	controller := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/message" {
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			submittedBodies <- body
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(controller.Close)
	controllerAddress := strings.TrimPrefix(controller.URL, "http://")

	senderRegistry, _, senderPeers := controllerTestRegistry(t)
	senderPeer := newControllerTestPeer(t, controllerTestRemoteID)
	require.NoError(t, senderPeers.Add(senderPeer))
	sender := newControllerMediation(controllerTestConfig(controllerAddress), controllerTestLocalID,
		senderRegistry, senderPeers, p2p.NopMetrics(), log.NewNopLogger(), nil)
	require.NoError(t, sender.Start())
	t.Cleanup(sender.Stop)

	receiverRegistry, receiverReactor, receiverPeers := controllerTestRegistry(t)
	receiverPeer := newControllerTestPeer(t, controllerTestLocalID)
	require.NoError(t, receiverPeers.Add(receiverPeer))
	receiverConfig := controllerTestConfig(controllerAddress)
	receiverConfig.NodeID = 2
	receiver := newControllerMediation(receiverConfig, controllerTestRemoteID,
		receiverRegistry, receiverPeers, p2p.NopMetrics(), log.NewNopLogger(), nil)
	require.NoError(t, receiver.Start())
	t.Cleanup(receiver.Stop)

	byName := make(map[string]controllerInventoryCase)
	for _, test := range controllerInventoryCases() {
		byName[test.name] = test
	}
	submissionOrder := []controllerInventoryCase{byName["block-part"], byName["precommit-nil-block"], byName["proposal"]}
	for _, item := range submissionOrder {
		require.True(t, sender.Submit(controllerTestRemoteID, item.route, item.message, mustWrapped(t, item.message.(interface{ Wrap() proto.Message })), p2p.SendModeNonBlocking))
	}

	submitted := make(map[string][]byte, len(submissionOrder))
	submittedOuter := make(map[string]controllerMessage, len(submissionOrder))
	for range submissionOrder {
		select {
		case body := <-submittedBodies:
			var outer controllerMessage
			require.NoError(t, json.Unmarshal(body, &outer))
			submitted[outer.Type] = body
			submittedOuter[outer.Type] = outer
		case <-time.After(time.Second):
			t.Fatal("fake controller did not observe all submissions")
		}
	}
	select {
	case <-receiverReactor.received:
		t.Fatal("selected submission reached native receive before delayed callback delivery")
	case <-time.After(20 * time.Millisecond):
	}

	forwardTypes := []string{"Proposal", "BlockPart", "Vote", "Vote"}
	ordinaryMessages := []controllerInventoryCase{byName["proposal"], byName["block-part"], byName["precommit-nil-block"], byName["precommit-nil-block"]}
	ordinary := make([]p2p.Envelope, 0, len(ordinaryMessages))
	for _, item := range ordinaryMessages {
		decoded, err := receiverRegistry.Decode(item.route, mustWrapped(t, item.message.(interface{ Wrap() proto.Message })))
		require.NoError(t, err)
		require.NoError(t, receiverRegistry.Dispatch(item.route, receiverPeer, decoded))
		ordinary = append(ordinary, <-receiverReactor.received)
	}

	for _, messageType := range forwardTypes {
		outer := submittedOuter[messageType]
		statusCode := postControllerCallback(t, receiver, submitted[messageType])
		require.Equal(t, http.StatusAccepted, statusCode, "ID %s must report reinjection acceptance", outer.ID)
	}

	for index, expected := range ordinary {
		select {
		case actual := <-receiverReactor.received:
			require.Equal(t, expected.ChannelID, actual.ChannelID)
			require.Same(t, expected.Src, actual.Src)
			require.IsType(t, expected.Message, actual.Message)
			expectedBytes, err := proto.Marshal(expected.Message.(interface{ Wrap() proto.Message }).Wrap())
			require.NoError(t, err)
			actualBytes, err := proto.Marshal(actual.Message.(interface{ Wrap() proto.Message }).Wrap())
			require.NoError(t, err)
			require.Equal(t, expectedBytes, actualBytes)
			if index >= 2 {
				require.Equal(t, submittedOuter["Vote"].ID, submittedOuter[forwardTypes[index]].ID)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for native reinjection")
		}
	}
	require.NoError(t, receiver.FatalError())
}

func TestControllerMediationCallbackRejections(t *testing.T) {
	mediation, reactor, _ := newControllerTestService(t, func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
	validBody := controllerCallbackBody(t, "7")
	var valid controllerMessage
	require.NoError(t, json.Unmarshal(validBody, &valid))
	marshalOuter := func(outer controllerMessage) []byte {
		body, err := json.Marshal(outer)
		require.NoError(t, err)
		return body
	}

	statusCode := postControllerCallback(t, mediation, []byte("{"))
	require.Equal(t, http.StatusBadRequest, statusCode, "malformed callback")

	originalMaxBody := mediation.config.CallbackMaxBodyBytes
	mediation.config.CallbackMaxBodyBytes = int64(len(validBody) - 1)
	statusCode = postControllerCallback(t, mediation, validBody)
	require.Equal(t, http.StatusRequestEntityTooLarge, statusCode, "oversized callback")
	mediation.config.CallbackMaxBodyBytes = originalMaxBody

	wrongDestination := valid
	wrongDestination.To = string(controllerTestRemoteID)
	wrongDestination.ID = fmt.Sprintf("%s_%s_7", wrongDestination.From, wrongDestination.To)
	statusCode = postControllerCallback(t, mediation, marshalOuter(wrongDestination))
	require.Equal(t, http.StatusUnprocessableEntity, statusCode, "wrong destination")

	unknownSource := valid
	unknownSource.From = "not-a-peer"
	unknownSource.ID = fmt.Sprintf("%s_%s_7", unknownSource.From, unknownSource.To)
	statusCode = postControllerCallback(t, mediation, marshalOuter(unknownSource))
	require.Equal(t, http.StatusUnprocessableEntity, statusCode, "unknown source")

	unknownRoute := valid
	decodedPayload, err := decodeAdapterPayload(unknownRoute.Data)
	require.NoError(t, err)
	unknownRoute.Data, err = encodeAdapterPayload(0xFF, decodedPayload.Wrapped)
	require.NoError(t, err)
	statusCode = postControllerCallback(t, mediation, marshalOuter(unknownRoute))
	require.Equal(t, http.StatusUnprocessableEntity, statusCode, "unknown route")

	invalidAdapter := valid
	invalidAdapter.Data = []byte{controllerAdapterVersion}
	statusCode = postControllerCallback(t, mediation, marshalOuter(invalidAdapter))
	require.Equal(t, http.StatusBadRequest, statusCode, "invalid adapter payload")

	invalidNative := controllerCallbackBodyForMessage(t, "8", &statesyncv1.SnapshotsResponse{})
	statusCode = postControllerCallback(t, mediation, invalidNative)
	require.Equal(t, http.StatusUnprocessableEntity, statusCode, "semantically invalid native payload")

	statusCode = postControllerCallback(t, mediation, controllerCallbackBody(t, "9"))
	require.Equal(t, http.StatusAccepted, statusCode)
	select {
	case <-reactor.received:
	case <-time.After(time.Second):
		t.Fatal("valid callback was not delivered after malformed callback")
	}
	require.NoError(t, mediation.FatalError())
}

func TestControllerMediationNoForwardNoReceive(t *testing.T) {
	mediation, reactor, _ := newControllerTestService(t, func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
	require.True(t, mediation.Submit(controllerTestRemoteID, statesync.SnapshotChannel, &statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeNonBlocking))
	select {
	case <-reactor.received:
		t.Fatal("message reached native receive path without callback")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestControllerMediationBoundsAndFailures(t *testing.T) {
	t.Run("non-2xx after readiness", func(t *testing.T) {
		mediation, _, _ := newControllerTestService(t, func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/message" {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		})
		require.True(t, mediation.Submit(controllerTestRemoteID, statesync.SnapshotChannel, &statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeNonBlocking))
		select {
		case err := <-mediation.FatalC():
			require.ErrorContains(t, err, "controller/control-channel failure")
		case <-time.After(time.Second):
			t.Fatal("controller failure did not become fatal")
		}
	})

	t.Run("submission timeout after readiness", func(t *testing.T) {
		mediation, _, _ := newControllerTestService(t, func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/message" {
				time.Sleep(100 * time.Millisecond)
				writer.WriteHeader(http.StatusNoContent)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		})
		mediation.config.RequestTimeout = 30 * time.Millisecond
		require.True(t, mediation.Submit(controllerTestRemoteID, statesync.SnapshotChannel, &statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeNonBlocking))
		select {
		case err := <-mediation.FatalC():
			require.ErrorContains(t, err, "context deadline exceeded")
		case <-time.After(time.Second):
			t.Fatal("controller timeout did not become fatal")
		}
	})

	t.Run("controller outage after readiness", func(t *testing.T) {
		controller := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }))
		registry, _, peers := controllerTestRegistry(t)
		peer := newControllerTestPeer(t, controllerTestRemoteID)
		require.NoError(t, peers.Add(peer))
		mediation := newControllerMediation(controllerTestConfig(strings.TrimPrefix(controller.URL, "http://")), controllerTestLocalID,
			registry, peers, p2p.NopMetrics(), log.NewNopLogger(), nil)
		require.NoError(t, mediation.Start())
		t.Cleanup(mediation.Stop)
		controller.Close()
		require.True(t, mediation.Submit(controllerTestRemoteID, statesync.SnapshotChannel, &statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeNonBlocking))
		select {
		case err := <-mediation.FatalC():
			require.ErrorContains(t, err, "controller/control-channel failure")
		case <-time.After(time.Second):
			t.Fatal("post-readiness controller outage did not become fatal")
		}
	})

	t.Run("callback listener termination", func(t *testing.T) {
		mediation, _, _ := newControllerTestService(t, func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
		require.NoError(t, mediation.listener.Close())
		select {
		case err := <-mediation.FatalC():
			require.ErrorContains(t, err, "callback listener terminated")
		case <-time.After(time.Second):
			t.Fatal("callback listener failure did not become fatal")
		}
	})

	t.Run("counter registry exhaustion is a local invariant", func(t *testing.T) {
		mediation, _, _ := newControllerTestService(t, func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
		mediation.config.CounterRegistryCapacity = 1
		require.True(t, mediation.Submit(controllerTestRemoteID, statesync.SnapshotChannel, &statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeNonBlocking))
		otherDestination := p2p.ID("3333333333333333333333333333333333333333")
		require.False(t, mediation.Submit(otherDestination, statesync.SnapshotChannel, &statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeNonBlocking))
		select {
		case err := <-mediation.FatalC():
			require.ErrorContains(t, err, "local invariant failure")
		case <-time.After(time.Second):
			t.Fatal("counter exhaustion did not become fatal")
		}
	})
}

func TestControllerMediationOwnerFatalPropagation(t *testing.T) {
	controller := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/message" {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(controller.Close)
	registry, _, peers := controllerTestRegistry(t)
	peer := newControllerTestPeer(t, controllerTestRemoteID)
	require.NoError(t, peers.Add(peer))
	owner := &Node{}
	mediation := newControllerMediation(controllerTestConfig(strings.TrimPrefix(controller.URL, "http://")), controllerTestLocalID,
		registry, peers, p2p.NopMetrics(), log.NewNopLogger(), owner.recordControllerFatal)
	require.NoError(t, mediation.Start())
	t.Cleanup(mediation.Stop)
	require.True(t, mediation.Submit(controllerTestRemoteID, statesync.SnapshotChannel, &statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeNonBlocking))
	require.Eventually(t, func() bool { return owner.FatalError() != nil }, time.Second, time.Millisecond)
	require.ErrorContains(t, owner.FatalError(), "controller/control-channel failure")
}

func TestControllerMediationCallbackQueueFailure(t *testing.T) {
	controller := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }))
	t.Cleanup(controller.Close)
	registry, reactor, peers := controllerTestRegistry(t)
	peer := newControllerTestPeer(t, controllerTestRemoteID)
	require.NoError(t, peers.Add(peer))
	reactor.entered = make(chan struct{}, 1)
	reactor.release = make(chan struct{})
	config := controllerTestConfig(strings.TrimPrefix(controller.URL, "http://"))
	config.CallbackQueueCapacity = 1
	mediation := newControllerMediation(config, controllerTestLocalID, registry, peers, p2p.NopMetrics(), log.NewNopLogger(), nil)
	require.NoError(t, mediation.Start())
	t.Cleanup(mediation.Stop)

	require.Equal(t, http.StatusAccepted, postControllerCallback(t, mediation, controllerCallbackBody(t, "10")))
	select {
	case <-reactor.entered:
	case <-time.After(time.Second):
		t.Fatal("first callback did not enter native dispatch")
	}
	require.Equal(t, http.StatusAccepted, postControllerCallback(t, mediation, controllerCallbackBody(t, "11")))
	require.Equal(t, http.StatusServiceUnavailable, postControllerCallback(t, mediation, controllerCallbackBody(t, "12")))
	select {
	case err := <-mediation.FatalC():
		require.ErrorContains(t, err, "controller/control-channel failure")
	case <-time.After(time.Second):
		t.Fatal("full callback queue did not become fatal")
	}
	close(reactor.release)
}

func TestControllerMediationLifecycle(t *testing.T) {
	registry, _, peers := controllerTestRegistry(t)
	mediation := newControllerMediation(controllerTestConfig("127.0.0.1:1"), controllerTestLocalID, registry, peers, p2p.NopMetrics(), log.NewNopLogger(), nil)
	require.Error(t, mediation.Start())
	mediation.Stop()
	mediation.Stop()
}

func TestControllerMediationConcurrentIDs(t *testing.T) {
	registry, _, peers := controllerTestRegistry(t)
	mediation := newControllerMediation(controllerTestConfig("127.0.0.1:1"), controllerTestLocalID, registry, peers, p2p.NopMetrics(), log.NewNopLogger(), nil)
	const count = 100
	ids := make(chan string, count)
	var wait sync.WaitGroup
	for i := 0; i < count; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			id, err := mediation.nextID(controllerTestRemoteID)
			require.NoError(t, err)
			ids <- id
		}()
	}
	wait.Wait()
	close(ids)
	unique := make(map[string]struct{}, count)
	for id := range ids {
		unique[id] = struct{}{}
	}
	require.Len(t, unique, count)
}

func TestControllerMediationNonBlockingAdmissionContention(t *testing.T) {
	mediation, _, _ := newControllerTestService(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	<-mediation.submitGate
	started := time.Now()
	accepted := mediation.Submit(controllerTestRemoteID, statesync.SnapshotChannel,
		&statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeNonBlocking)
	elapsed := time.Since(started)
	mediation.submitGate <- struct{}{}
	require.False(t, accepted)
	require.Less(t, elapsed, 50*time.Millisecond)
}

func TestControllerMediationBlockingAdmissionBound(t *testing.T) {
	mediation, _, _ := newControllerTestService(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	mediation.config.SendTimeout = 40 * time.Millisecond
	<-mediation.submitGate
	started := time.Now()
	accepted := mediation.Submit(controllerTestRemoteID, statesync.SnapshotChannel,
		&statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeBlocking)
	elapsed := time.Since(started)
	mediation.submitGate <- struct{}{}
	require.False(t, accepted)
	require.GreaterOrEqual(t, elapsed, 30*time.Millisecond)
	require.Less(t, elapsed, 250*time.Millisecond)
}

func TestControllerMediationReconnectUsesCurrentPeer(t *testing.T) {
	mediation, reactor, original := newControllerTestService(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	peers := mediation.peers.(*p2p.PeerSet)
	require.True(t, peers.Remove(original))
	statusCode := postControllerCallback(t, mediation, controllerCallbackBody(t, "2"))
	require.Equal(t, http.StatusConflict, statusCode)

	replacement := newControllerTestPeer(t, controllerTestRemoteID)
	require.NoError(t, peers.Add(replacement))
	statusCode = postControllerCallback(t, mediation, controllerCallbackBody(t, "3"))
	require.Equal(t, http.StatusAccepted, statusCode)
	select {
	case envelope := <-reactor.received:
		require.Same(t, replacement, envelope.Src)
	case <-time.After(time.Second):
		t.Fatal("replacement peer did not receive callback")
	}
}

func TestControllerMediationReResolvesPeerBeforeDispatch(t *testing.T) {
	mediation, reactor, original := newControllerTestService(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	reactor.entered = make(chan struct{}, 1)
	reactor.release = make(chan struct{})
	statusCode := postControllerCallback(t, mediation, controllerCallbackBody(t, "4"))
	require.Equal(t, http.StatusAccepted, statusCode)
	select {
	case <-reactor.entered:
	case <-time.After(time.Second):
		t.Fatal("first native dispatch did not start")
	}

	peers := mediation.peers.(*p2p.PeerSet)
	require.True(t, peers.Remove(original))
	replacement := newControllerTestPeer(t, controllerTestRemoteID)
	require.NoError(t, peers.Add(replacement))
	statusCode = postControllerCallback(t, mediation, controllerCallbackBody(t, "5"))
	require.Equal(t, http.StatusAccepted, statusCode)
	close(reactor.release)

	first := <-reactor.received
	second := <-reactor.received
	require.Same(t, original, first.Src)
	require.Same(t, replacement, second.Src)
}

func TestControllerMediationConcurrentLifecycleTransitions(t *testing.T) {
	registrationEntered := make(chan struct{})
	releaseRegistration := make(chan struct{})
	controller := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/replica" {
			close(registrationEntered)
			<-releaseRegistration
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer controller.Close()
	registry, _, peers := controllerTestRegistry(t)
	mediation := newControllerMediation(controllerTestConfig(strings.TrimPrefix(controller.URL, "http://")),
		controllerTestLocalID, registry, peers, p2p.NopMetrics(), log.NewNopLogger(), nil)
	startResult := make(chan error, 1)
	go func() { startResult <- mediation.Start() }()
	<-registrationEntered
	stopDone := make(chan struct{})
	go func() {
		mediation.Stop()
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("Stop overlapped an incomplete Start transition")
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseRegistration)
	require.NoError(t, <-startResult)
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("serialized Stop did not complete")
	}
	require.False(t, mediation.isReady())
	require.Error(t, mediation.Start())
	mediation.Stop()
}

func TestControllerMediationShutdownReleasesQueuedAccounting(t *testing.T) {
	releaseSubmission := make(chan struct{})
	mediation, _, _ := newControllerTestService(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/message" {
			<-releaseSubmission
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	for i := 0; i < 3; i++ {
		require.True(t, mediation.Submit(controllerTestRemoteID, statesync.SnapshotChannel,
			&statesyncv1.SnapshotsRequest{}, mustWrapped(t, &statesyncv1.SnapshotsRequest{}), p2p.SendModeNonBlocking))
	}
	close(releaseSubmission)
	select {
	case <-mediation.FatalC():
	case <-time.After(time.Second):
		t.Fatal("submission failure did not become fatal")
	}
	mediation.Stop()
	require.Zero(t, atomic.LoadInt64(&mediation.outboundBytes))
	require.Zero(t, len(mediation.outboundSlots))
}

func TestControllerMediationBlockingNativeDispatchShutdown(t *testing.T) {
	mediation, reactor, _ := newControllerTestService(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	reactor.entered = make(chan struct{}, 1)
	reactor.release = make(chan struct{})
	mediation.config.ShutdownTimeout = 40 * time.Millisecond
	statusCode := postControllerCallback(t, mediation, controllerCallbackBody(t, "6"))
	require.Equal(t, http.StatusAccepted, statusCode)
	select {
	case <-reactor.entered:
	case <-time.After(time.Second):
		t.Fatal("native dispatch did not enter the blocking reactor")
	}

	started := time.Now()
	mediation.Stop()
	elapsed := time.Since(started)
	require.GreaterOrEqual(t, elapsed, 30*time.Millisecond)
	require.Less(t, elapsed, 250*time.Millisecond)
	require.Error(t, mediation.FatalError())
	close(reactor.release)
	workersDone := make(chan struct{})
	go func() {
		mediation.workers.Wait()
		close(workersDone)
	}()
	select {
	case <-workersDone:
	case <-time.After(time.Second):
		t.Fatal("reinjection worker did not terminate after native dispatch returned")
	}
}

func mustWrapped(t *testing.T, message interface{ Wrap() proto.Message }) []byte {
	t.Helper()
	wrapped, err := proto.Marshal(message.Wrap())
	require.NoError(t, err)
	return wrapped
}
