package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmos/gogoproto/proto"

	consensusv1 "github.com/cometbft/cometbft/api/cometbft/consensus/v1"
	statesyncv1 "github.com/cometbft/cometbft/api/cometbft/statesync/v1"
	cfg "github.com/cometbft/cometbft/config"
	cs "github.com/cometbft/cometbft/internal/consensus"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/statesync"
)

const (
	controllerAdapterVersion = byte(1)
	controllerAdapterHeader  = 6
	controllerIDMaxBytes     = 128
)

var selectedControllerRoutes = []byte{
	cs.StateChannel,
	cs.DataChannel,
	cs.VoteChannel,
	cs.VoteSetBitsChannel,
	statesync.SnapshotChannel,
	statesync.ChunkChannel,
}

type controllerMessage struct {
	From string `json:"From"`
	To   string `json:"To"`
	Data []byte `json:"Data"`
	Type string `json:"Type"`
	ID   string `json:"ID"`
}

type controllerRegistration struct {
	ID    int    `json:"id"`
	Alias string `json:"alias"`
	Addr  string `json:"addr"`
}

type adapterPayload struct {
	Version   byte
	ChannelID byte
	Wrapped   []byte
}

type outboundMediationItem struct {
	message controllerMessage
	bytes   int64
}

type reinjectionItem struct {
	message   controllerMessage
	channelID byte
	wrapped   []byte
	bytes     int64
}

type mediationFatalError struct {
	Class string
	Err   error
}

func (e *mediationFatalError) Error() string {
	return fmt.Sprintf("controller mediation %s: %v", e.Class, e.Err)
}

func (e *mediationFatalError) Unwrap() error { return e.Err }

type controllerLifecycleState uint8

const (
	controllerLifecycleNew controllerLifecycleState = iota
	controllerLifecycleStarting
	controllerLifecycleRunning
	controllerLifecycleStopping
	controllerLifecycleStopped
)

type controllerMediation struct {
	config   *cfg.ControllerMediationConfig
	localID  p2p.ID
	registry *p2p.NativeRouteRegistry
	peers    p2p.IPeerSet
	metrics  *p2p.Metrics
	logger   log.Logger
	onFatal  func(error)

	client   *http.Client
	server   *http.Server
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc

	outboundQueue chan outboundMediationItem
	outboundSlots chan struct{}
	callbackQueue chan reinjectionItem
	callbackSlots chan struct{}
	outboundBytes int64
	callbackBytes int64
	submitGate    chan struct{}
	callbackMu    sync.Mutex
	counterMu     sync.Mutex
	counters      map[p2p.ID]uint64
	stateMu       sync.Mutex
	ready         bool
	everReady     bool
	stopping      bool
	fatalErr      error
	fatalC        chan error
	workers       sync.WaitGroup
	lifecycleMu   sync.Mutex
	lifecycle     controllerLifecycleState
}

func newControllerMediation(
	config *cfg.ControllerMediationConfig,
	localID p2p.ID,
	registry *p2p.NativeRouteRegistry,
	peers p2p.IPeerSet,
	metrics *p2p.Metrics,
	logger log.Logger,
	onFatal func(error),
) *controllerMediation {
	ctx, cancel := context.WithCancel(context.Background())
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: time.Second}).DialContext,
		ResponseHeaderTimeout: minDuration(config.RequestTimeout, 2*time.Second),
	}
	mediation := &controllerMediation{
		config:        config,
		localID:       localID,
		registry:      registry,
		peers:         peers,
		metrics:       metrics,
		logger:        logger,
		onFatal:       onFatal,
		client:        &http.Client{Transport: transport, Timeout: config.RequestTimeout},
		ctx:           ctx,
		cancel:        cancel,
		outboundQueue: make(chan outboundMediationItem, config.OutboundQueueCapacity),
		outboundSlots: make(chan struct{}, config.OutboundQueueCapacity),
		submitGate:    make(chan struct{}, 1),
		callbackQueue: make(chan reinjectionItem, config.CallbackQueueCapacity),
		callbackSlots: make(chan struct{}, config.CallbackQueueCapacity),
		counters:      make(map[p2p.ID]uint64),
		fatalC:        make(chan error, 1),
		lifecycle:     controllerLifecycleNew,
	}
	mediation.submitGate <- struct{}{}
	return mediation
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func (m *controllerMediation) Start() (err error) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.lifecycle != controllerLifecycleNew {
		return errors.New("controller mediation lifecycle has already transitioned")
	}
	m.lifecycle = controllerLifecycleStarting
	defer func() {
		if err != nil {
			m.stopLocked()
			m.lifecycle = controllerLifecycleStopped
			return
		}
		m.lifecycle = controllerLifecycleRunning
	}()

	listener, err := net.Listen("tcp", m.config.CallbackListenAddress)
	if err != nil {
		return fmt.Errorf("bind controller callback: %w", err)
	}
	m.listener = listener
	mux := http.NewServeMux()
	mux.HandleFunc("/message", m.serveMessage)
	m.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: minDuration(m.config.RequestTimeout, 2*time.Second),
		ReadTimeout:       m.config.RequestTimeout,
		WriteTimeout:      m.config.RequestTimeout,
		IdleTimeout:       30 * time.Second,
	}

	m.workers.Add(2)
	go m.submissionWorker()
	go m.reinjectionWorker()
	go func() {
		err := m.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.fatal("controller/control-channel failure", fmt.Errorf("callback listener terminated: %w", err))
		}
	}()

	if err := m.register(); err != nil {
		return err
	}
	m.stateMu.Lock()
	if m.fatalErr != nil {
		err := m.fatalErr
		m.stateMu.Unlock()
		return err
	}
	m.ready = true
	m.everReady = true
	m.stateMu.Unlock()
	m.logger.Info("Controller mediation ready", "event", "registration_accepted", "From", string(m.localID))
	m.record("registration_accepted", "", "registration")
	return nil
}

func (m *controllerMediation) Stop() {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.lifecycle == controllerLifecycleStopped {
		return
	}
	m.lifecycle = controllerLifecycleStopping
	m.stopLocked()
	m.lifecycle = controllerLifecycleStopped
}

func (m *controllerMediation) stopLocked() {
	m.stateMu.Lock()
	m.ready = false
	m.stopping = true
	m.stateMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), m.config.ShutdownTimeout)
	if m.server != nil {
		_ = m.server.Shutdown(ctx)
	}
	cancel()

	// Interrupt controller I/O before waiting for workers. Accepted messages
	// that can no longer be submitted are reported through the fatal path.
	m.cancel()

	<-m.submitGate
	close(m.outboundQueue)
	m.submitGate <- struct{}{}
	m.callbackMu.Lock()
	close(m.callbackQueue)
	m.callbackMu.Unlock()

	done := make(chan struct{})
	go func() {
		m.workers.Wait()
		close(done)
	}()
	timer := time.NewTimer(m.config.ShutdownTimeout)
	defer timer.Stop()
	workersStopped := false
	select {
	case <-done:
		workersStopped = true
	case <-timer.C:
		m.fatal("local invariant failure", errors.New("shutdown accepted-work timeout"))
	}
	if workersStopped {
		m.releaseQueuedReservations()
	}
	m.client.CloseIdleConnections()
}

func (m *controllerMediation) releaseQueuedReservations() {
	for item := range m.outboundQueue {
		<-m.outboundSlots
		atomic.AddInt64(&m.outboundBytes, -item.bytes)
	}
	for item := range m.callbackQueue {
		<-m.callbackSlots
		atomic.AddInt64(&m.callbackBytes, -item.bytes)
	}
}

func (*controllerMediation) Selected(_ byte, message proto.Message) bool {
	_, _, ok := selectedMessageBinding(message)
	return ok
}

func (*controllerMediation) SelectedRoute(channelID byte) bool {
	for _, selected := range selectedControllerRoutes {
		if channelID == selected {
			return true
		}
	}
	return false
}

func (m *controllerMediation) Submit(
	destination p2p.ID,
	channelID byte,
	message proto.Message,
	wrapped []byte,
	mode p2p.SendMode,
) bool {
	label, expectedRoute, ok := selectedMessageBinding(message)
	if !ok || expectedRoute != channelID {
		m.LocalInvariant(fmt.Errorf("selected outbound type/route mismatch: %T on %X", message, channelID))
		return false
	}
	capacity, ok := m.registry.ReceiveCapacity(channelID)
	if !ok || len(wrapped) > capacity {
		m.LocalInvariant(fmt.Errorf("selected outbound payload violates route %X capacity", channelID))
		return false
	}
	payload, err := encodeAdapterPayload(channelID, wrapped)
	if err != nil {
		m.LocalInvariant(err)
		return false
	}
	if !m.acquireSubmitAdmission(mode) {
		m.record("outbound_admission_rejected", "synchronous send rejection", label)
		return false
	}
	defer func() { m.submitGate <- struct{}{} }()
	if !m.isReady() {
		<-m.outboundSlots
		m.record("outbound_shutdown_rejected", "synchronous send rejection", label)
		return false
	}
	retained := int64(len(payload))
	if atomic.LoadInt64(&m.outboundBytes)+retained > m.config.OutboundQueueBytes {
		<-m.outboundSlots
		m.record("outbound_bytes_rejected", "synchronous send rejection", label)
		return false
	}
	atomic.AddInt64(&m.outboundBytes, retained)
	id, err := m.nextID(destination)
	if err != nil {
		atomic.AddInt64(&m.outboundBytes, -retained)
		<-m.outboundSlots
		m.LocalInvariant(err)
		return false
	}
	outer := controllerMessage{
		From: string(m.localID), To: string(destination), Data: payload, Type: label, ID: id,
	}
	m.outboundQueue <- outboundMediationItem{message: outer, bytes: retained}
	m.logger.Info("Controller message accepted for submission", "event", "submission_admission", "ID", id,
		"From", outer.From, "To", outer.To, "Type", outer.Type)
	m.record("submission_admission", "", label)
	return true
}

func (m *controllerMediation) acquireSubmitAdmission(mode p2p.SendMode) bool {
	if mode == p2p.SendModeNonBlocking {
		select {
		case <-m.submitGate:
		default:
			return false
		}
		select {
		case m.outboundSlots <- struct{}{}:
			return true
		default:
			m.submitGate <- struct{}{}
			return false
		}
	}
	timer := time.NewTimer(m.config.SendTimeout)
	defer timer.Stop()
	select {
	case <-m.submitGate:
	case <-timer.C:
		return false
	case <-m.ctx.Done():
		return false
	}
	select {
	case m.outboundSlots <- struct{}{}:
		return true
	case <-timer.C:
		m.submitGate <- struct{}{}
		return false
	case <-m.ctx.Done():
		m.submitGate <- struct{}{}
		return false
	}
}

func (m *controllerMediation) LocalInvariant(err error) {
	m.fatal("local invariant failure", err)
}

func (m *controllerMediation) DirectInboundRejected(source p2p.ID, channelID byte) {
	m.logger.Error("Rejected selected message from direct P2P path", "event", "direct_inbound_rejected",
		"From", string(source), "To", string(m.localID), "Type", fmt.Sprintf("channel_%02x", channelID),
		"error_class", "message-scoped rejection")
	m.record("direct_inbound_rejected", "message-scoped rejection", fmt.Sprintf("channel_%02x", channelID))
}

func (m *controllerMediation) register() error {
	registration := controllerRegistration{ID: m.config.NodeID, Alias: string(m.localID), Addr: m.config.CallbackAdvertiseAddress}
	body, err := json.Marshal(registration)
	if err != nil {
		return fmt.Errorf("marshal registration: %w", err)
	}
	delay := m.config.RegistrationRetryDelay
	var lastErr error
	for attempt := 1; attempt <= m.config.RegistrationAttempts; attempt++ {
		lastErr = m.post("/replica", body)
		if lastErr == nil {
			return nil
		}
		m.logger.Error("Controller registration failed", "event", "registration_retry", "attempt", attempt, "err", lastErr)
		if attempt == m.config.RegistrationAttempts {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-m.ctx.Done():
			timer.Stop()
			return fmt.Errorf("registration canceled: %w", m.ctx.Err())
		}
		if delay < 2*time.Second {
			delay *= 2
			if delay > 2*time.Second {
				delay = 2 * time.Second
			}
		}
	}
	return fmt.Errorf("controller registration exhausted: %w", lastErr)
}

func (m *controllerMediation) submissionWorker() {
	defer m.workers.Done()
	for item := range m.outboundQueue {
		<-m.outboundSlots
		body, err := json.Marshal(item.message)
		if err == nil {
			err = m.post("/message", body)
		}
		atomic.AddInt64(&m.outboundBytes, -item.bytes)
		if err != nil {
			m.logger.Error("Controller submission failed", "event", "submission_failure", "ID", item.message.ID,
				"From", item.message.From, "To", item.message.To, "Type", item.message.Type,
				"error_class", "controller/control-channel failure", "err", err)
			m.record("submission_failure", "controller/control-channel failure", item.message.Type)
			m.fatal("controller/control-channel failure", fmt.Errorf("submit %s: %w", item.message.ID, err))
			return
		}
		m.logger.Info("Controller submission accepted", "event", "submission_accepted", "ID", item.message.ID,
			"From", item.message.From, "To", item.message.To, "Type", item.message.Type)
		m.record("submission_accepted", "", item.message.Type)
	}
}

func (m *controllerMediation) post(path string, body []byte) error {
	ctx, cancel := context.WithTimeout(m.ctx, m.config.RequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+m.config.ControllerAddress+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := m.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("controller returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (m *controllerMediation) serveMessage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		m.rejectCallback(writer, http.StatusUnsupportedMediaType, "callback_content_type", errors.New("Content-Type must be application/json"), "unknown", controllerMessage{})
		return
	}
	if !m.isReady() {
		m.rejectCallback(writer, http.StatusServiceUnavailable, "callback_not_ready", errors.New("mediation is not ready"), "unknown", controllerMessage{})
		return
	}

	request.Body = http.MaxBytesReader(writer, request.Body, m.config.CallbackMaxBodyBytes)
	outer, err := decodeControllerMessage(request.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			m.rejectCallback(writer, http.StatusRequestEntityTooLarge, "callback_oversized", err, "unknown", outer)
		} else {
			m.rejectCallback(writer, http.StatusBadRequest, "callback_malformed", err, "unknown", outer)
		}
		return
	}
	if outer.To != string(m.localID) {
		m.rejectCallback(writer, http.StatusUnprocessableEntity, "callback_wrong_destination", errors.New("wrong destination"), outer.Type, outer)
		return
	}
	if err := validateControllerID(outer.From); err != nil {
		m.rejectCallback(writer, http.StatusUnprocessableEntity, "callback_unknown_source", err, outer.Type, outer)
		return
	}
	if err := validateControllerMessageID(outer); err != nil || outer.Type == "" {
		if err == nil {
			err = errors.New("type is required")
		}
		m.rejectCallback(writer, http.StatusBadRequest, "callback_field_invalid", err, outer.Type, outer)
		return
	}
	peer := m.peers.Get(p2p.ID(outer.From))
	if peer == nil || !peer.IsRunning() {
		m.rejectCallback(writer, http.StatusConflict, "callback_stale_source", errors.New("source peer is not currently connected"), outer.Type, outer)
		return
	}
	payload, err := decodeAdapterPayload(outer.Data)
	if err != nil {
		m.rejectCallback(writer, http.StatusBadRequest, "callback_adapter_invalid", err, outer.Type, outer)
		return
	}
	capacity, ok := m.registry.ReceiveCapacity(payload.ChannelID)
	if !ok || !m.SelectedRoute(payload.ChannelID) {
		m.rejectCallback(writer, http.StatusUnprocessableEntity, "callback_unknown_route", errors.New("unknown selected route"), outer.Type, outer)
		return
	}
	if len(payload.Wrapped) > capacity {
		m.rejectCallback(writer, http.StatusRequestEntityTooLarge, "native_payload_oversized", errors.New("native payload exceeds route capacity"), outer.Type, outer)
		return
	}
	decoded, err := m.registry.Decode(payload.ChannelID, payload.Wrapped)
	if err != nil {
		m.rejectCallback(writer, http.StatusUnprocessableEntity, "callback_decode_rejected", err, outer.Type, outer)
		return
	}
	label, expectedRoute, ok := selectedMessageBinding(decoded)
	if !ok || expectedRoute != payload.ChannelID || label != outer.Type {
		m.rejectCallback(writer, http.StatusUnprocessableEntity, "callback_type_mismatch", errors.New("type or route does not match decoded message"), outer.Type, outer)
		return
	}
	if err := validateSelectedMessage(decoded); err != nil {
		m.rejectCallback(writer, http.StatusUnprocessableEntity, "callback_semantic_rejection", err, outer.Type, outer)
		return
	}

	item := reinjectionItem{message: outer, channelID: payload.ChannelID, wrapped: append([]byte(nil), payload.Wrapped...), bytes: int64(len(payload.Wrapped))}
	if !m.acceptCallback(item) {
		if m.isStopping() {
			m.rejectCallback(writer, http.StatusServiceUnavailable, "callback_shutdown_rejected", errors.New("mediation shutdown is in progress"), outer.Type, outer)
			return
		}
		m.fatal("controller/control-channel failure", errors.New("valid callback could not enter bounded reinjection queue"))
		m.rejectCallback(writer, http.StatusServiceUnavailable, "callback_admission_fatal", errors.New("reinjection queue unavailable"), outer.Type, outer)
		return
	}
	m.logger.Info("Controller callback accepted for native reinjection", "event", "reinjection_acceptance", "ID", outer.ID,
		"From", outer.From, "To", outer.To, "Type", outer.Type)
	m.record("reinjection_acceptance", "", outer.Type)
	writer.WriteHeader(http.StatusAccepted)
}

func (m *controllerMediation) acceptCallback(item reinjectionItem) bool {
	m.callbackMu.Lock()
	defer m.callbackMu.Unlock()
	if !m.isReady() {
		return false
	}
	select {
	case m.callbackSlots <- struct{}{}:
	default:
		return false
	}
	if atomic.LoadInt64(&m.callbackBytes)+item.bytes > m.config.CallbackQueueBytes {
		<-m.callbackSlots
		return false
	}
	atomic.AddInt64(&m.callbackBytes, item.bytes)
	m.callbackQueue <- item
	return true
}

func (m *controllerMediation) reinjectionWorker() {
	defer m.workers.Done()
	for item := range m.callbackQueue {
		<-m.callbackSlots
		peer := m.peers.Get(p2p.ID(item.message.From))
		if peer == nil || !peer.IsRunning() {
			atomic.AddInt64(&m.callbackBytes, -item.bytes)
			m.reinjectionFailure(item, errors.New("source peer disconnected after callback acceptance"))
			return
		}
		decoded, err := m.registry.Decode(item.channelID, item.wrapped)
		if err == nil {
			err = m.registry.Dispatch(item.channelID, peer, decoded)
		}
		atomic.AddInt64(&m.callbackBytes, -item.bytes)
		if err != nil {
			m.reinjectionFailure(item, err)
			return
		}
		m.logger.Info("Controller message handed to native Reactor", "event", "native_dispatch", "ID", item.message.ID,
			"From", item.message.From, "To", item.message.To, "Type", item.message.Type)
		m.record("native_dispatch", "", item.message.Type)
	}
}

func (m *controllerMediation) reinjectionFailure(item reinjectionItem, err error) {
	m.logger.Error("Controller reinjection failed after acceptance", "event", "reinjection_failure", "ID", item.message.ID,
		"From", item.message.From, "To", item.message.To, "Type", item.message.Type,
		"error_class", "local invariant failure", "err", err)
	m.record("reinjection_failure", "local invariant failure", item.message.Type)
	m.fatal("local invariant failure", fmt.Errorf("accepted callback %s lost: %w", item.message.ID, err))
}

func (m *controllerMediation) rejectCallback(writer http.ResponseWriter, status int, event string, err error, label string, outer controllerMessage) {
	m.logger.Error("Controller callback rejected", "event", event, "ID", outer.ID, "From", outer.From, "To", outer.To,
		"Type", outer.Type, "error_class", "message-scoped rejection", "err", err)
	m.record(event, "message-scoped rejection", label)
	http.Error(writer, http.StatusText(status), status)
}

func (m *controllerMediation) nextID(destination p2p.ID) (string, error) {
	m.counterMu.Lock()
	defer m.counterMu.Unlock()
	next, exists := m.counters[destination]
	if !exists {
		if len(m.counters) >= m.config.CounterRegistryCapacity {
			return "", errors.New("counter registry exhausted")
		}
		m.counters[destination] = 1
		next = 0
	} else {
		if next == ^uint64(0) {
			return "", errors.New("message counter exhausted")
		}
		m.counters[destination] = next + 1
	}
	return fmt.Sprintf("%s_%s_%d", m.localID, destination, next), nil
}

func (m *controllerMediation) fatal(class string, err error) {
	if err == nil {
		return
	}
	fatalErr := &mediationFatalError{Class: class, Err: err}
	m.stateMu.Lock()
	if m.fatalErr != nil {
		m.stateMu.Unlock()
		return
	}
	m.fatalErr = fatalErr
	m.ready = false
	notifyOwner := m.everReady
	m.stateMu.Unlock()
	m.logger.Error("Controller mediation fatal", "event", "fatal", "error_class", class, "err", err)
	m.record("fatal", class, "internal")
	m.fatalC <- fatalErr
	if notifyOwner && m.onFatal != nil {
		m.onFatal(fatalErr)
	}
}

func (m *controllerMediation) FatalError() error {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.fatalErr
}

func (m *controllerMediation) FatalC() <-chan error { return m.fatalC }

func (m *controllerMediation) isReady() bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.ready && !m.stopping && m.fatalErr == nil
}

func (m *controllerMediation) isStopping() bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.stopping
}

func (m *controllerMediation) record(event, class, label string) {
	if m.metrics == nil || m.metrics.ControllerMediationTotal == nil {
		return
	}
	m.metrics.ControllerMediationTotal.With("event", event, "error_class", class, "message_type", label).Add(1)
}

func encodeAdapterPayload(channelID byte, wrapped []byte) ([]byte, error) {
	if uint64(len(wrapped)) > uint64(^uint32(0)) {
		return nil, errors.New("native payload length exceeds adapter field")
	}
	payload := make([]byte, controllerAdapterHeader+len(wrapped))
	payload[0] = controllerAdapterVersion
	payload[1] = channelID
	binary.BigEndian.PutUint32(payload[2:6], uint32(len(wrapped)))
	copy(payload[controllerAdapterHeader:], wrapped)
	return payload, nil
}

func decodeAdapterPayload(data []byte) (adapterPayload, error) {
	if len(data) < controllerAdapterHeader {
		return adapterPayload{}, errors.New("adapter payload is truncated")
	}
	if data[0] != controllerAdapterVersion {
		return adapterPayload{}, fmt.Errorf("unsupported adapter version %d", data[0])
	}
	length := int(binary.BigEndian.Uint32(data[2:6]))
	if length != len(data)-controllerAdapterHeader {
		return adapterPayload{}, errors.New("adapter payload length mismatch")
	}
	return adapterPayload{Version: data[0], ChannelID: data[1], Wrapped: append([]byte(nil), data[6:]...)}, nil
}

func decodeControllerMessage(reader io.Reader) (controllerMessage, error) {
	decoder := json.NewDecoder(reader)
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return controllerMessage{}, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return controllerMessage{}, err
	}
	required := []string{"From", "To", "Data", "Type", "ID"}
	if len(raw) != len(required) {
		return controllerMessage{}, errors.New("outer object must contain exactly five fields")
	}
	for _, name := range required {
		if _, ok := raw[name]; !ok {
			return controllerMessage{}, fmt.Errorf("missing exact outer field %s", name)
		}
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return controllerMessage{}, err
	}
	var outer controllerMessage
	if err := json.Unmarshal(body, &outer); err != nil {
		return controllerMessage{}, err
	}
	return outer, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateControllerID(value string) error {
	if len(value) != p2p.IDByteLength*2 {
		return errors.New("source identity has invalid length")
	}
	_, err := hex.DecodeString(value)
	return err
}

func validateControllerMessageID(message controllerMessage) error {
	if message.ID == "" || len(message.ID) > controllerIDMaxBytes {
		return errors.New("message ID is empty or oversized")
	}
	prefix := message.From + "_" + message.To + "_"
	if !strings.HasPrefix(message.ID, prefix) {
		return errors.New("message ID does not match From and To")
	}
	counter := strings.TrimPrefix(message.ID, prefix)
	if counter == "" {
		return errors.New("message ID counter is empty")
	}
	if _, err := strconv.ParseUint(counter, 10, 64); err != nil {
		return fmt.Errorf("message ID counter is invalid: %w", err)
	}
	return nil
}

func selectedMessageBinding(message proto.Message) (string, byte, bool) {
	switch message.(type) {
	case *consensusv1.NewRoundStep:
		return "NewRoundStep", cs.StateChannel, true
	case *consensusv1.NewValidBlock:
		return "NewValidBlock", cs.StateChannel, true
	case *consensusv1.HasVote:
		return "HasVote", cs.StateChannel, true
	case *consensusv1.HasProposalBlockPart:
		return "HasProposalBlockPart", cs.StateChannel, true
	case *consensusv1.VoteSetMaj23:
		return "VoteSetMaj23", cs.StateChannel, true
	case *consensusv1.Proposal:
		return "Proposal", cs.DataChannel, true
	case *consensusv1.ProposalPOL:
		return "ProposalPOL", cs.DataChannel, true
	case *consensusv1.BlockPart:
		return "BlockPart", cs.DataChannel, true
	case *consensusv1.Vote:
		return "Vote", cs.VoteChannel, true
	case *consensusv1.VoteSetBits:
		return "VoteSetBits", cs.VoteSetBitsChannel, true
	case *statesyncv1.SnapshotsRequest:
		return "SnapshotsRequest", statesync.SnapshotChannel, true
	case *statesyncv1.SnapshotsResponse:
		return "SnapshotsResponse", statesync.SnapshotChannel, true
	case *statesyncv1.ChunkRequest:
		return "ChunkRequest", statesync.ChunkChannel, true
	case *statesyncv1.ChunkResponse:
		return "ChunkResponse", statesync.ChunkChannel, true
	default:
		return "", 0, false
	}
}

func validateSelectedMessage(message proto.Message) error {
	switch message.(type) {
	case *consensusv1.NewRoundStep, *consensusv1.NewValidBlock, *consensusv1.HasVote,
		*consensusv1.HasProposalBlockPart, *consensusv1.VoteSetMaj23, *consensusv1.Proposal,
		*consensusv1.ProposalPOL, *consensusv1.BlockPart, *consensusv1.Vote, *consensusv1.VoteSetBits:
		_, err := cs.MsgFromProto(message)
		return err
	case *statesyncv1.SnapshotsRequest, *statesyncv1.SnapshotsResponse,
		*statesyncv1.ChunkRequest, *statesyncv1.ChunkResponse:
		return statesync.ValidateMessage(message)
	default:
		return fmt.Errorf("unselected native message %T", message)
	}
}
