package p2p

import (
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/cosmos/gogoproto/proto"

	"github.com/cometbft/cometbft/types"
)

// SendMode preserves the blocking contract of the originating peer API.
type SendMode uint8

const (
	SendModeBlocking SendMode = iota
	SendModeNonBlocking
)

// MediatedMessageSink is the narrow node-owned boundary used by peer send and
// receive paths. Implementations must never fall back to the native transport
// after Selected returns true.
type MediatedMessageSink interface {
	Selected(channelID byte, message proto.Message) bool
	SelectedRoute(channelID byte) bool
	Submit(destination ID, channelID byte, message proto.Message, wrapped []byte, mode SendMode) bool
	LocalInvariant(err error)
	DirectInboundRejected(source ID, channelID byte)
}

// NativeRoute is an immutable copy of an ordinary Switch channel binding.
type NativeRoute struct {
	ChannelID       byte
	ReceiveCapacity int
	prototype       proto.Message
	reactor         Reactor
}

// NativeRouteRegistry is frozen before enabled P2P startup and is safe for
// concurrent reads. It reuses ordinary codec and Reactor boundaries.
type NativeRouteRegistry struct {
	routes      map[byte]NativeRoute
	onPeerError func(Peer, any)
}

func newNativeRouteRegistry(routes map[byte]NativeRoute, onPeerError func(Peer, any)) *NativeRouteRegistry {
	return &NativeRouteRegistry{routes: routes, onPeerError: onPeerError}
}

// Routes returns selected channel IDs in stable order.
func (r *NativeRouteRegistry) Routes() []byte {
	routes := make([]byte, 0, len(r.routes))
	for channelID := range r.routes {
		routes = append(routes, channelID)
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i] < routes[j] })
	return routes
}

// ReceiveCapacity returns the ordinary filled receive limit for a route.
func (r *NativeRouteRegistry) ReceiveCapacity(channelID byte) (int, bool) {
	route, ok := r.routes[channelID]
	return route.ReceiveCapacity, ok
}

// Decode applies the ordinary prototype clone, protobuf decode, and unwrap.
func (r *NativeRouteRegistry) Decode(channelID byte, wrapped []byte) (proto.Message, error) {
	route, ok := r.routes[channelID]
	if !ok {
		return nil, fmt.Errorf("unknown mediated channel %X", channelID)
	}
	return decodeNativeMessage(route.prototype, wrapped)
}

// Dispatch invokes the frozen ordinary Reactor entry with current peer
// attribution. Panics follow the existing peer-error path.
func (r *NativeRouteRegistry) Dispatch(channelID byte, source Peer, message proto.Message) (err error) {
	route, ok := r.routes[channelID]
	if !ok || route.reactor == nil {
		return fmt.Errorf("missing mediated route %X", channelID)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			if r.onPeerError != nil {
				r.onPeerError(source, recovered)
			}
			err = fmt.Errorf("native reactor panic: %v", recovered)
		}
	}()
	route.reactor.Receive(Envelope{ChannelID: channelID, Src: source, Message: message})
	return nil
}

func decodeNativeMessage(prototype proto.Message, wrapped []byte) (proto.Message, error) {
	if prototype == nil {
		return nil, errors.New("nil message prototype")
	}
	message := proto.Clone(prototype)
	if err := proto.Unmarshal(wrapped, message); err != nil {
		return nil, fmt.Errorf("unmarshaling message into %s: %w", reflect.TypeOf(prototype), err)
	}
	if unwrapper, ok := message.(types.Unwrapper); ok {
		unwrapped, err := unwrapper.Unwrap()
		if err != nil {
			return nil, fmt.Errorf("unwrapping message: %w", err)
		}
		message = unwrapped
	}
	return message, nil
}

// PeerMediationSink installs mediation before the peer receive callback is built.
func PeerMediationSink(sink MediatedMessageSink) PeerOption {
	return func(p *peer) { p.mediation = sink }
}
