package p2p

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	consensusv1 "github.com/cometbft/cometbft/api/cometbft/consensus/v1"
	p2pproto "github.com/cometbft/cometbft/api/cometbft/p2p/v1"
	statesyncv1 "github.com/cometbft/cometbft/api/cometbft/statesync/v1"
	"github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/p2p/conn"
)

const (
	controllerConsensusStateChannel    = byte(0x20)
	controllerStateSyncSnapshotChannel = byte(0x60)
)

type mediationSubmission struct {
	destination ID
	channelID   byte
	message     proto.Message
	wrapped     []byte
	mode        SendMode
}

type mediationNativeMessage struct {
	channelID byte
	wrapped   []byte
}

type recordingMediationSink struct {
	selected         map[byte]bool
	mu               sync.Mutex
	submissions      []mediationSubmission
	directRejections int
	invariant        error
	submitResult     bool
	submitted        chan mediationSubmission
}

func (s *recordingMediationSink) Selected(channelID byte, _ proto.Message) bool {
	return s.selected[channelID]
}

func (s *recordingMediationSink) SelectedRoute(channelID byte) bool {
	return s.selected[channelID]
}

func (s *recordingMediationSink) Submit(destination ID, channelID byte, message proto.Message, wrapped []byte, mode SendMode) bool {
	submission := mediationSubmission{destination: destination, channelID: channelID, message: message, wrapped: wrapped, mode: mode}
	s.mu.Lock()
	s.submissions = append(s.submissions, submission)
	result := s.submitResult
	s.mu.Unlock()
	if s.submitted != nil {
		s.submitted <- submission
	}
	return result
}

func (s *recordingMediationSink) LocalInvariant(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invariant = err
}

func (s *recordingMediationSink) DirectInboundRejected(_ ID, _ byte) {
	s.mu.Lock()
	s.directRejections++
	s.mu.Unlock()
}

type mediationRecordingReactor struct {
	BaseReactor
	channels []*conn.ChannelDescriptor
	received chan Envelope
}

func newMediationRecordingReactor(channels []*conn.ChannelDescriptor) *mediationRecordingReactor {
	r := &mediationRecordingReactor{channels: channels, received: make(chan Envelope, 8)}
	r.BaseReactor = *NewBaseReactor("mediation-recording", r)
	return r
}

func (r *mediationRecordingReactor) GetChannels() []*conn.ChannelDescriptor { return r.channels }
func (*mediationRecordingReactor) AddPeer(Peer)                             {}
func (*mediationRecordingReactor) RemovePeer(Peer, any)                     {}
func (r *mediationRecordingReactor) Receive(envelope Envelope)              { r.received <- envelope }

func newMediationPeer(
	t *testing.T,
	connection net.Conn,
	id string,
	outbound bool,
	descriptor *conn.ChannelDescriptor,
	reactor Reactor,
	sink MediatedMessageSink,
	onPeerError ...func(Peer, any),
) *peer {
	t.Helper()
	peerInfo := testNodeInfo(ID(id), "remote").(DefaultNodeInfo)
	peerInfo.Channels = []byte{descriptor.ID}
	options := []PeerOption{}
	if sink != nil {
		options = append(options, PeerMediationSink(sink))
	}
	errorHandler := func(Peer, any) {}
	if len(onPeerError) > 0 {
		errorHandler = onPeerError[0]
	}
	p := newPeer(
		newPeerConn(outbound, false, connection, nil),
		conn.DefaultMConnConfig(),
		peerInfo,
		map[byte]Reactor{descriptor.ID: reactor},
		map[byte]proto.Message{descriptor.ID: descriptor.MessageType},
		[]*conn.ChannelDescriptor{descriptor},
		errorHandler,
		options...,
	)
	p.SetLogger(log.TestingLogger())
	require.NoError(t, p.Start())
	t.Cleanup(func() { _ = p.Stop() })
	return p
}

func requireNoDirectBytes(t *testing.T, connection net.Conn) {
	t.Helper()
	require.NoError(t, connection.SetReadDeadline(time.Now().Add(20*time.Millisecond)))
	buffer := make([]byte, 1)
	_, err := connection.Read(buffer)
	var timeout net.Error
	require.ErrorAs(t, err, &timeout)
	require.True(t, timeout.Timeout())
}

func TestControllerMediationSelectedSendModesAndNoFallback(t *testing.T) {
	for _, test := range []struct {
		name      string
		mode      SendMode
		route     byte
		prototype proto.Message
		message   proto.Message
		outbound  bool
		send      func(Peer, Envelope) bool
	}{
		{name: "blocking state-sync reply", mode: SendModeBlocking, route: controllerStateSyncSnapshotChannel, prototype: &statesyncv1.Message{}, message: &statesyncv1.SnapshotsResponse{Height: 1, Format: 1, Chunks: 1, Hash: []byte{1}}, send: func(peer Peer, envelope Envelope) bool { return peer.Send(envelope) }},
		{name: "blocking consensus gossip from dialed peer", mode: SendModeBlocking, route: 0x21, prototype: &consensusv1.Message{}, message: &consensusv1.Proposal{}, outbound: true, send: func(peer Peer, envelope Envelope) bool { return peer.Send(envelope) }},
		{name: "non-blocking consensus reply", mode: SendModeNonBlocking, route: 0x23, prototype: &consensusv1.Message{}, message: &consensusv1.VoteSetBits{}, send: func(peer Peer, envelope Envelope) bool { return peer.TrySend(envelope) }},
		{name: "non-blocking direct", mode: SendModeNonBlocking, route: controllerConsensusStateChannel, prototype: &consensusv1.Message{}, message: &consensusv1.HasVote{Height: 1, Type: 1}, send: func(peer Peer, envelope Envelope) bool { return peer.TrySend(envelope) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			descriptor := &conn.ChannelDescriptor{ID: test.route, Priority: 1, MessageType: test.prototype}
			local, remote := net.Pipe()
			t.Cleanup(func() { _ = remote.Close() })
			sink := &recordingMediationSink{
				selected: map[byte]bool{test.route: true}, submitResult: true,
			}
			reactor := newMediationRecordingReactor([]*conn.ChannelDescriptor{descriptor})
			peer := newMediationPeer(t, local, "0123456789012345678901234567890123456789", test.outbound, descriptor, reactor, sink)
			require.Equal(t, test.outbound, peer.IsOutbound())

			require.True(t, test.send(peer, Envelope{ChannelID: test.route, Message: test.message}))
			sink.mu.Lock()
			require.Len(t, sink.submissions, 1)
			submission := sink.submissions[0]
			sink.mu.Unlock()
			require.Equal(t, test.mode, submission.mode)
			require.Same(t, test.message, submission.message)
			require.Equal(t, test.route, submission.channelID)
			require.NotEmpty(t, submission.wrapped)
			requireNoDirectBytes(t, remote)
		})
	}

	t.Run("admission rejection has no fallback", func(t *testing.T) {
		descriptor := &conn.ChannelDescriptor{ID: controllerStateSyncSnapshotChannel, Priority: 1, MessageType: &statesyncv1.Message{}}
		local, remote := net.Pipe()
		t.Cleanup(func() { _ = remote.Close() })
		sink := &recordingMediationSink{selected: map[byte]bool{controllerStateSyncSnapshotChannel: true}}
		reactor := newMediationRecordingReactor([]*conn.ChannelDescriptor{descriptor})
		peer := newMediationPeer(t, local, "1123456789012345678901234567890123456789", false, descriptor, reactor, sink)
		require.False(t, peer.TrySend(Envelope{ChannelID: controllerStateSyncSnapshotChannel, Message: &statesyncv1.SnapshotsRequest{}}))
		requireNoDirectBytes(t, remote)
	})
}

func TestControllerMediationBroadcastAndTryBroadcast(t *testing.T) {
	for _, test := range []struct {
		name      string
		mode      SendMode
		route     byte
		prototype proto.Message
		message   proto.Message
		send      func(*Switch, Envelope)
	}{
		{name: "state-sync retry broadcast", mode: SendModeBlocking, route: controllerStateSyncSnapshotChannel, prototype: &statesyncv1.Message{}, message: &statesyncv1.SnapshotsRequest{}, send: func(sw *Switch, envelope Envelope) { sw.Broadcast(envelope) }},
		{name: "consensus try-broadcast", mode: SendModeNonBlocking, route: controllerConsensusStateChannel, prototype: &consensusv1.Message{}, message: &consensusv1.HasVote{Height: 1, Type: 1}, send: func(sw *Switch, envelope Envelope) { sw.TryBroadcast(envelope) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			descriptor := &conn.ChannelDescriptor{ID: test.route, Priority: 1, MessageType: test.prototype}
			local, remote := net.Pipe()
			t.Cleanup(func() { _ = remote.Close() })
			sink := &recordingMediationSink{
				selected: map[byte]bool{test.route: true}, submitResult: true, submitted: make(chan mediationSubmission, 1),
			}
			reactor := newMediationRecordingReactor([]*conn.ChannelDescriptor{descriptor})
			peer := newMediationPeer(t, local, "2123456789012345678901234567890123456789", false, descriptor, reactor, sink)
			sw := NewSwitch(config.DefaultP2PConfig(), nil)
			require.NoError(t, sw.peers.Add(peer))
			t.Cleanup(func() { sw.peers.Remove(peer) })

			test.send(sw, Envelope{ChannelID: test.route, Message: test.message})
			select {
			case submission := <-sink.submitted:
				require.Equal(t, test.mode, submission.mode)
				require.IsType(t, test.message, submission.message)
			case <-time.After(time.Second):
				t.Fatal("broadcast did not reach selected peer boundary")
			}
			requireNoDirectBytes(t, remote)
		})
	}
}

func TestControllerMediationUnselectedAndDisabledRemainNative(t *testing.T) {
	descriptor := &conn.ChannelDescriptor{ID: testCh, Priority: 1, MessageType: &p2pproto.Message{}}
	for _, test := range []struct {
		name           string
		sink           MediatedMessageSink
		verifyOutbound bool
	}{
		{name: "unselected with mediation", sink: &recordingMediationSink{selected: map[byte]bool{}, submitResult: true}, verifyOutbound: true},
		{name: "disabled", sink: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			local, remote := net.Pipe()
			reactor := newMediationRecordingReactor([]*conn.ChannelDescriptor{descriptor})
			peer := newMediationPeer(t, local, "3123456789012345678901234567890123456789", false, descriptor, reactor, test.sink)
			nativeOutbound := make(chan mediationNativeMessage, 1)
			remoteMConn := conn.NewMConnectionWithConfig(remote, []*conn.ChannelDescriptor{descriptor}, func(channelID byte, wrapped []byte) {
				nativeOutbound <- mediationNativeMessage{channelID: channelID, wrapped: append([]byte(nil), wrapped...)}
			}, func(any) {}, conn.DefaultMConnConfig())
			remoteMConn.SetLogger(log.TestingLogger())
			require.NoError(t, remoteMConn.Start())
			t.Cleanup(func() { _ = remoteMConn.Stop() })

			if test.verifyOutbound {
				message := &p2pproto.PexRequest{}
				require.True(t, peer.Send(Envelope{ChannelID: testCh, Message: message}))
				select {
				case native := <-nativeOutbound:
					require.Equal(t, byte(testCh), native.channelID)
					require.Equal(t, mustMarshalP2P(t, message), native.wrapped)
				case <-time.After(time.Second):
					t.Fatal("unselected outbound message did not use the native MConnection")
				}
				sink := test.sink.(*recordingMediationSink)
				sink.mu.Lock()
				require.Empty(t, sink.submissions)
				sink.mu.Unlock()
			}

			require.True(t, remoteMConn.Send(testCh, mustMarshalP2P(t, &p2pproto.PexRequest{})))
			select {
			case envelope := <-reactor.received:
				require.Same(t, peer, envelope.Src)
				require.Equal(t, byte(testCh), envelope.ChannelID)
				require.IsType(t, &p2pproto.PexRequest{}, envelope.Message)
			case <-time.After(time.Second):
				t.Fatal("ordinary inbound path did not deliver")
			}
		})
	}
}

func TestControllerMediationDirectInboundGate(t *testing.T) {
	descriptor := &conn.ChannelDescriptor{ID: controllerStateSyncSnapshotChannel, Priority: 1, MessageType: &statesyncv1.Message{}}
	for _, test := range []struct {
		name    string
		enabled bool
	}{
		{name: "enabled rejects selected route", enabled: true},
		{name: "disabled delivers same selected route", enabled: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			local, remote := net.Pipe()
			var mediationSink MediatedMessageSink
			var sink *recordingMediationSink
			if test.enabled {
				sink = &recordingMediationSink{selected: map[byte]bool{controllerStateSyncSnapshotChannel: true}, submitResult: true}
				mediationSink = sink
			}
			reactor := newMediationRecordingReactor([]*conn.ChannelDescriptor{descriptor})
			peer := newMediationPeer(t, local, "4123456789012345678901234567890123456789", false, descriptor, reactor, mediationSink)
			remoteMConn := conn.NewMConnectionWithConfig(remote, []*conn.ChannelDescriptor{descriptor}, func(byte, []byte) {}, func(any) {}, conn.DefaultMConnConfig())
			remoteMConn.SetLogger(log.TestingLogger())
			require.NoError(t, remoteMConn.Start())
			t.Cleanup(func() { _ = remoteMConn.Stop() })

			require.True(t, remoteMConn.Send(controllerStateSyncSnapshotChannel, mustMarshalStateSync(t, &statesyncv1.SnapshotsRequest{})))
			if test.enabled {
				require.Eventually(t, func() bool {
					sink.mu.Lock()
					defer sink.mu.Unlock()
					return sink.directRejections == 1
				}, time.Second, time.Millisecond)
				select {
				case <-reactor.received:
					t.Fatal("selected direct inbound message reached the native reactor")
				case <-time.After(20 * time.Millisecond):
				}
				return
			}

			select {
			case envelope := <-reactor.received:
				require.Same(t, peer, envelope.Src)
				require.Equal(t, controllerStateSyncSnapshotChannel, envelope.ChannelID)
				require.IsType(t, &statesyncv1.SnapshotsRequest{}, envelope.Message)
			case <-time.After(time.Second):
				t.Fatal("disabled direct inbound message did not reach the native reactor")
			}
		})
	}
}

func TestControllerMediationDisabledReceiveErrorsRemainNative(t *testing.T) {
	descriptor := &conn.ChannelDescriptor{ID: testCh, Priority: 1, MessageType: &p2pproto.Message{}}
	for _, test := range []struct {
		name       string
		wrapped    []byte
		wantPrefix string
	}{
		{name: "unmarshal", wrapped: []byte{0xff}, wantPrefix: "unmarshaling message:"},
		{name: "unwrap", wrapped: mustMarshalP2PWrapper(t, &p2pproto.Message{}), wantPrefix: "unwrapping message:"},
	} {
		t.Run(test.name, func(t *testing.T) {
			local, remote := net.Pipe()
			errors := make(chan any, 1)
			reactor := newMediationRecordingReactor([]*conn.ChannelDescriptor{descriptor})
			newMediationPeer(t, local, "6123456789012345678901234567890123456789", false, descriptor, reactor, nil, func(_ Peer, recovered any) {
				errors <- recovered
			})
			remoteMConn := conn.NewMConnectionWithConfig(remote, []*conn.ChannelDescriptor{descriptor}, func(byte, []byte) {}, func(any) {}, conn.DefaultMConnConfig())
			remoteMConn.SetLogger(log.TestingLogger())
			require.NoError(t, remoteMConn.Start())
			t.Cleanup(func() { _ = remoteMConn.Stop() })

			require.True(t, remoteMConn.Send(testCh, test.wrapped))
			select {
			case recovered := <-errors:
				require.Contains(t, fmt.Sprint(recovered), test.wantPrefix)
			case <-time.After(time.Second):
				t.Fatal("disabled receive error did not reach the ordinary peer-error path")
			}
		})
	}
}

type mediationCaptureTransport struct {
	accepted chan MediatedMessageSink
	dialed   chan MediatedMessageSink
}

func (*mediationCaptureTransport) NetAddress() NetAddress { return NetAddress{} }
func (t *mediationCaptureTransport) Accept(config peerConfig) (Peer, error) {
	t.accepted <- config.mediation
	return nil, ErrTransportClosed{}
}

func (t *mediationCaptureTransport) Dial(_ NetAddress, config peerConfig) (Peer, error) {
	t.dialed <- config.mediation
	return nil, errors.New("dial capture complete")
}
func (*mediationCaptureTransport) Cleanup(Peer) {}

func TestControllerMediationAcceptedAndDialedPeerPropagation(t *testing.T) {
	transport := &mediationCaptureTransport{accepted: make(chan MediatedMessageSink, 1), dialed: make(chan MediatedMessageSink, 1)}
	sw := NewSwitch(config.DefaultP2PConfig(), transport)
	sink := &recordingMediationSink{selected: map[byte]bool{controllerStateSyncSnapshotChannel: true}, submitResult: true}
	sw.SetMediatedMessageSink(sink)
	require.NoError(t, sw.Start())
	t.Cleanup(func() { _ = sw.Stop() })
	select {
	case accepted := <-transport.accepted:
		require.Same(t, sink, accepted)
	case <-time.After(time.Second):
		t.Fatal("accepted-peer path did not receive mediation sink")
	}

	err := sw.addOutboundPeerWithConfig(&NetAddress{}, config.DefaultP2PConfig())
	require.ErrorContains(t, err, "dial capture complete")
	select {
	case dialed := <-transport.dialed:
		require.Same(t, sink, dialed)
	case <-time.After(time.Second):
		t.Fatal("dialed-peer path did not receive mediation sink")
	}
}

func TestControllerMediationRouteRegistry(t *testing.T) {
	sw := NewSwitch(config.DefaultP2PConfig(), nil)
	reactor := NewTestReactor([]*conn.ChannelDescriptor{{
		ID: 0x20, Priority: 1, RecvMessageCapacity: 1024, MessageType: &p2pproto.Message{},
	}}, true)
	sw.AddReactor("selected", reactor)

	registry, err := sw.FreezeNativeRoutes([]byte{0x20})
	require.NoError(t, err)
	require.Equal(t, []byte{0x20}, registry.Routes())
	capacity, ok := registry.ReceiveCapacity(0x20)
	require.True(t, ok)
	require.Equal(t, 1024, capacity)

	encoded := mustMarshalP2P(t, &p2pproto.PexRequest{})
	decoded, err := registry.Decode(0x20, encoded)
	require.NoError(t, err)
	require.IsType(t, &p2pproto.PexRequest{}, decoded)

	_, err = sw.FreezeNativeRoutes([]byte{0x21})
	require.Error(t, err)
}

type marshalFailureMessage struct{}

func (*marshalFailureMessage) Reset()         {}
func (*marshalFailureMessage) String() string { return "marshal failure" }
func (*marshalFailureMessage) ProtoMessage()  {}
func (*marshalFailureMessage) Marshal() ([]byte, error) {
	return nil, errors.New("deliberate serialization failure")
}

func TestControllerMediationSerializationFailureIsFatalWithoutFallback(t *testing.T) {
	descriptor := &conn.ChannelDescriptor{ID: controllerStateSyncSnapshotChannel, Priority: 1, MessageType: &statesyncv1.Message{}}
	local, remote := net.Pipe()
	t.Cleanup(func() { _ = remote.Close() })
	sink := &recordingMediationSink{selected: map[byte]bool{controllerStateSyncSnapshotChannel: true}, submitResult: true}
	reactor := newMediationRecordingReactor([]*conn.ChannelDescriptor{descriptor})
	peer := newMediationPeer(t, local, "5123456789012345678901234567890123456789", false, descriptor, reactor, sink)
	require.False(t, peer.TrySend(Envelope{ChannelID: controllerStateSyncSnapshotChannel, Message: &marshalFailureMessage{}}))
	sink.mu.Lock()
	require.ErrorContains(t, sink.invariant, "encoding selected channel")
	require.Empty(t, sink.submissions)
	sink.mu.Unlock()
	requireNoDirectBytes(t, remote)
}

func TestControllerMediationSwitchSinkLifecycleRace(_ *testing.T) {
	sw := NewSwitch(config.DefaultP2PConfig(), nil)
	sink := &recordingMediationSink{selected: map[byte]bool{testCh: true}, submitResult: true}
	const iterations = 1000
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		for i := 0; i < iterations; i++ {
			sw.SetMediatedMessageSink(sink)
			sw.SetMediatedMessageSink(nil)
		}
	}()
	go func() {
		defer wait.Done()
		for i := 0; i < iterations; i++ {
			_ = sw.mediatedMessageSink()
		}
	}()
	wait.Wait()
}

func mustMarshalP2P(t *testing.T, message interface{ Wrap() proto.Message }) []byte {
	t.Helper()
	bytes, err := proto.Marshal(message.Wrap())
	require.NoError(t, err)
	return bytes
}

func mustMarshalP2PWrapper(t *testing.T, message proto.Message) []byte {
	t.Helper()
	bytes, err := proto.Marshal(message)
	require.NoError(t, err)
	return bytes
}

func mustMarshalStateSync(t *testing.T, message interface{ Wrap() proto.Message }) []byte {
	t.Helper()
	bytes, err := proto.Marshal(message.Wrap())
	require.NoError(t, err)
	return bytes
}
