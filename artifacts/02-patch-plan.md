# CometBFT controller-mediation patch plan

Status: **PASS**. Both upstream gates, specification hashes, target revision `d22299509a50140b74d81b113c4d78e4cf501994` (`v1.0.1`), critical symbols, clean baseline, and focused regression anchors were verified. No binding correction or scope expansion is required.

## Architecture

The patch keeps the Prompt 01 binding intact. `p2p.(*peer).send` remains the single selected outbound boundary for blocking sends, non-blocking sends, broadcasts, replies, gossip, and retries. It will perform the existing channel/running checks and native wrap/marshal, then make an exclusive choice: selected traffic goes to a node-owned mediation sink; unselected traffic goes to the existing `MConnection` continuation. A selected failure never takes the direct continuation.

The node-owned service uses the exact core JSON envelope. `Data` contains a six-byte deterministic adapter header—version, Channel ID, and big-endian native length—followed by the exact existing wrapped protobuf bytes. An explicit type switch labels the ten bound Consensus and four State Sync types. IDs use a concurrent, process-lifetime counter per destination, starting at zero and surviving reconnection.

After construction-time reactor options, enabled `Node.OnStart` freezes the selected Switch route/prototype/Reactor descriptors into an immutable P2P registry. Callback validation uses that registry, the existing codec and unwrap behavior, existing pure semantic validation, and a current authenticated `PeerSet` lookup. It queues only copied identity, label, route, ID, and wrapped-byte data. A single event-driven worker re-resolves the peer and decodes again before invoking the owning `Reactor.Receive`; it holds no HTTP, queue, registry, or peer-map lock across native dispatch. This preserves callback acceptance order and duplicates without claiming protocol execution.

The service owns finite HTTP timeouts, bounded registration retries, item-and-byte queue limits, a bounded callback body, a bounded counter registry, and one worker per direction. Outbound backpressure before acceptance returns the existing send failure. A valid callback that cannot be admitted, controller failure after readiness, or any accepted-work loss is fatal. Message-supplied malformed, oversized, wrong-destination, unknown-source, unknown-route, decode, or type-mismatch input rejects only that request.

Node owns readiness, partial-start rollback, shutdown, and the first typed fatal cause. Enabled startup freezes routes, binds the callback, starts workers, registers, enables selected submission, and only then starts ordinary P2P activity. Enabled runtime fatal errors cause orderly Node shutdown and return non-zero through the existing command. Disabled runner and signal behavior remain unchanged. Existing `[instrumentation]` retains its Prometheus meaning; the new default-off section is `[experimental_controller]`.

## Ordered implementation

1. **F1 — configuration:** add and test the distinct default-off configuration and every bound/timeout/address relationship. This checkpoint compiles independently.
2. **F2 — P2P boundary (atomic):** add the mediation interface, exclusive outbound branch, direct-inbound selected gate, frozen native registry, codec/dispatch reuse, peer-construction propagation, and generated metric. Tests prove no fallback and disabled/unselected preservation.
3. **F3 — node service (atomic):** add adapter framing and inventory, registration/submission, bounded queues and body, immutable callback handoff, current-peer reinjection, ID registry, diagnostics, readiness, rollback, and idempotent accounting. The only fake controller is in the new node test file.
4. **F4 — fatal owner/testnet (atomic):** make enabled command execution return the typed fatal after cleanup and extend the existing per-node testnet rewrite for unique IDs/advertised callback addresses.
5. **F5 — functional evidence:** require non-empty test discovery before every filtered test, run changed-package tests, and create `artifacts/03-functional-report.json` plus `artifacts/instrumentation-manifest.json` from the actual patch.
6. **R1 — concurrency/lifecycle refinement:** race-test and harden admission accounting, counter creation, callback total order, peer replacement, blocking native dispatch, fatal/signal races, rollback, drain, and repeated stop. A known unsafe interleaving blocks refinement PASS.
7. **R2 — final evidence:** run unit, regression, race, vet, lint, and JSON checks; update the manifest and write `artifacts/04-refinement-report.json` from actual results.

## Ownership and risk disposition

`controllerMediation` owns its HTTP client/server, listener, two queues, retained-byte reservations, counter registry, workers, readiness, and first fatal. `Node` owns service start/stop and process-visible fatal propagation. `Switch` owns the construction-time registry and current `PeerSet`; the immutable snapshot is the only asynchronous route view. No protocol handler, channel/message schema, consensus rule, state transition, persistent format, or timeout is changed.

All eleven Prompt 01 implementation constraints and all seven unresolved risks have exactly one primary owner in the JSON plan. Functional work establishes bounded safe ownership; Prompt 04 is assigned only rare interleaving proofs and hardening, not a known fundamental gap. Resource exhaustion outcomes are explicit: outbound pre-acceptance pressure is synchronous rejection; malformed/oversized callback input is message-scoped rejection; inability to accept a valid callback or controller failure is fatal control-channel failure; counter/registry/accounting corruption is fatal local invariant failure; registration exhaustion fails startup.

The precise path sets, new-file labels, symbol dependencies, discovery-before-filter commands, clause coverage, and actual planning commands are recorded in `artifacts/02-patch-plan.json`.
