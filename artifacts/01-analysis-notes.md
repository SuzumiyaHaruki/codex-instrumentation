# Revision-local binding notes

## Boundary rationale

`p2p.(*peer).send` is the narrowest existing outbound boundary at which the destination's authenticated `Peer.ID()`, the native concrete protobuf message, the Channel ID, and the blocking/non-blocking continuation are all present. `Peer.Send` and `Peer.TrySend` both enter it, and `Switch.Broadcast` and `Switch.TryBroadcast` expand to those peer APIs. Intercepting in `MConnection.Send`/`TrySend` would be too low: the native message has already been wrapped and serialized and the peer identity is no longer an argument. Intercepting each Reactor caller would duplicate policy and leave reply or future call sites as bypass risks.

The existing inbound native boundary is the owning `Reactor.Receive(p2p.Envelope)`. The ordinary path reaches it from the authenticated peer's `MConnection` callback after Channel lookup, wrapper-prototype clone, protobuf decode, unwrap, byte metrics, and source attribution. A callback adapter can reuse the Switch route registry, current `PeerSet` lookup, the same wrapper/codec rules, and this receive entry without creating another semantic message representation.

## Rejected alternatives

- Per-Reactor outbound hooks are broader and incomplete by construction; Consensus and State Sync contain blocking, non-blocking, broadcast, direct-reply, gossip, and retry producers.
- A transport replacement is unnecessary and would disturb authentication and peer lifecycle. The selected boundary composes with `MultiplexTransport`.
- Calling a Reactor synchronously from the HTTP handler is unsafe as an acceptance definition. Consensus receive may block on `peerMsgQueue`; native validation may synchronously stop/remove the source peer; State Sync receive may hold Reactor locks and send replies. A bounded native-handoff queue is the plausible acceptance boundary, with immutable route/wrapped-byte/identity metadata queued and connection-sensitive objects resolved again before dispatch.
- Treating callback `2xx` as protocol completion is rejected. It can mean only validated bounded handoff; later dispatch failure or accepted-work loss must be surfaced as an integrity failure.

## Planning risks

- The existing CLI runner returns startup errors, but after startup it blocks forever and `BaseService.Quit()` carries no error cause. Runtime instrumentation fatal errors therefore need an orderly, typed path to the node owner and non-zero command completion.
- `Config.Instrumentation` already means Prometheus metrics. Controller-mediation configuration must preserve that public meaning and avoid an ambiguous reuse of the existing section name.
- The selected Consensus route maximum is 1 MiB, State Sync Snapshot is 4 MiB, and State Sync Chunk is 16 MiB. HTTP request and decoded adapter bounds must account for route plus exact wrapped bytes without exceeding native route limits.
- Peer replacement is keyed by stable authenticated ID, but queued work must not retain a stale `Peer` pointer. Validate a current source before acceptance and re-resolve at dispatch; delayed work for a disconnected source is not deliverable.
- `Switch.AddReactor`/`RemoveReactor` and route maps are not goroutine-safe. Normal node assembly makes registrations before startup, while `CustomReactors` can replace them during construction. The planner must establish a frozen registry or safe lookup ownership rather than assuming concurrent mutation is harmless.
