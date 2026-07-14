# Controller-Mediated Message Delivery for CometBFT

## 1. Specification Profile

This target realization profile applies the [Controller-Mediated Message Delivery: Core Specification](./core.md) to CometBFT, including revisions derived from the Tendermint protocol lineage. This file and `core.md` are jointly normative, and target-specific requirements use the `CMT-*` namespace. Because CometBFT revisions may differ in package layout, P2P abstractions, message registries, Reactor interfaces, startup flow, and command structure, the profile fixes semantic obligations and evidence requirements but does not prescribe a package layout, symbol list, numeric Channel ID, or patch shape; all revision-local bindings must be derived from the checked-out source tree.

- **CMT-PROFILE-001:** The implementation MUST derive concrete files, symbols, registered messages, Channel descriptors, and lifecycle hooks from the checked-out source tree and MUST preserve that revision's established architecture and behavior.
- **CMT-PROFILE-002:** If a protocol concept named by this specification has been renamed, split, merged, or removed in the checked-out revision, the implementation MUST document the revision-local equivalent or an evidence-backed `not applicable` result. It MUST NOT force a historical abstraction into the codebase.

## 2. Semantic Scope

- **CMT-SCOPE-001:** Selected CometBFT messages MUST leave the sender only through the controller submission path and MUST reach the destination's ordinary Reactor or subsystem handler only after controller callback delivery.
- **CMT-SCOPE-002:** The implementation scope includes configuration, controller registration, outbound interception, callback reception, native decoding, native dispatch, lifecycle integration, diagnostics, and focused conformance tests.
- **CMT-SCOPE-003:** The implementation MUST NOT implement controller policy, add remote commands that mutate consensus state, change existing Protobuf definitions or persistent formats, or change ordinary behavior while instrumentation is disabled.
- **CMT-SCOPE-004:** Instrumentation is a communication-layer intervention. Mediating `Proposal`, `ProposalPOL`, or `BlockPart` traffic MUST NOT change proposal construction, proposer selection, proposal validity checks, locked or valid value handling, or the transition from Propose to Prevote. Mediating Prevote or Precommit `Vote` traffic MUST NOT change vote construction or signing, `nil`-vote semantics, voting-power accounting, quorum thresholds, decision rules, height/round/step transitions, or consensus timeouts.
- **CMT-SCOPE-005:** Round-state and vote-set messages such as `NewRoundStep`, `NewValidBlock`, `HasVote`, `VoteSetMaj23`, and `VoteSetBits` MUST remain ordinary Consensus Reactor inputs after controller delivery. The adapter MUST NOT interpret them as commands, synthesize missing proposal or vote state, or mutate peer consensus state outside the existing Reactor handler.
- **CMT-SCOPE-006:** This profile governs automatic node-side message instrumentation. It does not require the generated patch to observe whether a Proposal, BlockPart, Prevote, Precommit, or other reinjected message later changes Consensus State; emit protocol-execution acknowledgements; map events or state to TLA+; or implement model-guided scheduling, coverage, mutation, or fault policy.

## 3. Repository Binding Analysis

The implementation is analysis-guided. The obligations below establish the evidence-backed integration map that precedes and constrains production edits.

- **CMT-ANALYSIS-001:** Identify the configuration loading and validation path, node construction path, startup command path, and shutdown/error propagation path used by an ordinary node.
- **CMT-ANALYSIS-002:** Identify every outbound abstraction and Consensus or State Sync Reactor Gossip routine that can carry selected traffic, including blocking send, non-blocking send, peer send, broadcast, direct reply, retry, and helper paths. Trace native `Envelope` or equivalent values through the actual P2P boundary; do not assume that these paths converge without evidence.
- **CMT-ANALYSIS-003:** Identify the ordinary inbound path from a Channel and network bytes through route selection, native decoding, wrapper removal, Peer attribution, validation, and final Reactor or subsystem dispatch.
- **CMT-ANALYSIS-004:** Identify where the current revision registers Channel descriptors, message prototypes, codecs, and owning Reactors, and determine which of those registries can be reused by the adapter.
- **CMT-ANALYSIS-005:** Identify the stable authenticated Node ID, the in-memory Peer object presented to handlers, and all connection lifecycle events that create, replace, or remove the mapping between them. Distinguish P2P identity from validator address and voting identity.
- **CMT-ANALYSIS-006:** In both outbound and inbound directions, trace revision-local representatives for round-state Gossip, Proposal and BlockPart transfer, a Prevote, a Precommit, vote-set reconciliation, and every selected State Sync message family.
- **CMT-ANALYSIS-007:** Enumerate all discovered bypass risks. At minimum, compare blocking and non-blocking peer sends, broadcast expansion, replies created inside receive handlers, and direct invocation of lower transport layers.
- **CMT-ANALYSIS-008:** Record the resulting integration map, selected-message inventory, bypass-risk inventory, and changed-symbol-to-clause mapping in both the final report and the instrumentation manifest required by `CORE-PROFILE-009`. If the source tree is ambiguous or contradicts this specification, stop and report the ambiguity instead of guessing.
- **CMT-ANALYSIS-009:** For every candidate selected native type, record its protocol role, Channel descriptor, owning Reactor, wrapper and codec, outbound entry points, inbound handler, selected status, and source evidence. This binding table is a required intermediate artifact, not optional implementation commentary.
- **CMT-ANALYSIS-010:** The instrumentation manifest MUST additionally identify the concrete callback route, native Reactor or subsystem reinjection entry point, stable P2P identity source, peer-object lookup lifecycle, adapter payload version and fields, instrumentation configuration keys, and exact verification commands for the checked-out revision. It MUST mark revision-local uncertainty explicitly and MUST NOT include consensus-state extraction or formal-model mappings.
- **CMT-ANALYSIS-011:** Identify every construction-time option, custom Reactor facility, or registration hook that can replace or alter an owning Reactor, Channel descriptor, message prototype, wrapper, or codec after the ordinary registrations are assembled. The binding MUST distinguish unrelated extensions from overrides of selected Consensus or State Sync routes and MUST identify source evidence that can detect an incompatible override before instrumentation readiness.

## 4. Controlled Message Domain

- **CMT-SELECT-001:** Select every P2P message owned by the CometBFT Consensus Reactor that realizes or Gossips consensus progress. The concrete inventory MUST cover revision-local equivalents of: round-state notifications such as `NewRoundStep` and `NewValidBlock`; proposal dissemination such as `Proposal`, `ProposalPOL`, and `BlockPart`; signed `Vote` messages for both Prevote and Precommit, including `nil` votes; and vote-knowledge or reconciliation messages such as `HasVote`, `VoteSetMaj23`, and `VoteSetBits`.
- **CMT-SELECT-002:** Select every P2P message owned by the State Sync Reactor so synchronization traffic cannot bypass controller scheduling. The inventory MUST cover the revision's snapshot discovery, snapshot advertisement, chunk request/response, light-block request/response, and consensus-parameter request/response families when registered.
- **CMT-SELECT-003:** Derive the concrete selected routes and native types from the current revision's Consensus and State Sync Channel descriptors and message registries. The Consensus State, Data, Vote, and Vote Set Bits Channels, or their revision-local equivalents, MUST be accounted for. Historical numeric Channel IDs and the common type names above are orientation, not substitutes for authoritative registration data.
- **CMT-SELECT-004:** Traffic owned exclusively by transaction propagation, evidence propagation, block synchronization, peer exchange, RPC, or application communication remains unselected unless source analysis proves that it shares a selected route and cannot otherwise be separated safely.
- **CMT-SELECT-005:** Every discovered send mode for selected traffic MUST be intercepted. A lower-level path MUST NOT remain available to selected callers as an accidental bypass.
- **CMT-SELECT-006:** In instrumentation mode, selected traffic arriving through the ordinary direct P2P receive path MUST be rejected before native dispatch. The same path MUST remain unchanged when instrumentation is disabled.

## 5. Target Binding Contract

### 5.1 Outer controller contract

- **CMT-ADAPTER-001:** Use the registration and outer message HTTP contract from `core.md` without changing member names or their meaning.
- **CMT-ADAPTER-002:** Use the node's stable authenticated P2P identity as `alias` and `From`; use the authenticated destination identity as `To`. Do not use socket addresses, monikers, validator addresses, or transient peer indices as routing identities.
- **CMT-ADAPTER-003:** Registration MUST contain only the core `id`, `alias`, and callback `addr` fields unless source analysis proves that additional non-secret metadata is required. Private keys and signing material MUST NOT be sent.

### 5.2 Opaque target payload

- **CMT-ADAPTER-004:** `Data` MUST contain an adapter payload that is sufficient to reconstruct the original CometBFT route and native serialized message without controller interpretation.
- **CMT-ADAPTER-005:** The adapter payload schema MAY be chosen to fit the current revision, but it MUST be deterministic, version-local, bounded, and documented in focused tests and the final report. It MUST include a native route identifier and the exact native message bytes.
- **CMT-ADAPTER-006:** Serialize with the same Protobuf implementation and wrapper behavior used by the ordinary outbound P2P path. Do not invent a second semantic message representation.
- **CMT-ADAPTER-007:** On callback delivery, resolve the route through the existing Channel/message registry, instantiate the registered native message type, decode with the ordinary codec, and apply the ordinary unwrap operation before dispatch.
- **CMT-ADAPTER-008:** `Type` MUST be a deterministic human-readable label derived from the concrete native message. Source analysis MUST build a complete mapping for the selected inventory. Encountering a valid selected outbound type without a label is a local invariant failure under `CORE-ERR-004`.
- **CMT-ADAPTER-009:** After callback decoding, recompute the label from the concrete message and require it to match outer `Type`. A mismatch is a message-scoped rejection.

### 5.3 Native delivery semantics

- **CMT-ADAPTER-010:** Resolve outer `From` to the current authenticated peer object before accepting callback delivery. An unknown, disconnected, or stale source is a message-scoped rejection.
- **CMT-ADAPTER-011:** Deliver through the same owning Reactor or subsystem receive entry point used after ordinary P2P decoding, with the same route identifier, concrete message, and logical source peer.
- **CMT-ADAPTER-012:** Reuse ordinary semantic validation and peer-error reporting. Invalid controller input MUST NOT panic the process.
- **CMT-ADAPTER-013:** Do not locally restore send order, deduplicate, delay, or infer controller delivery. Callback acceptance order is the delivery order.
- **CMT-ADAPTER-014:** Preserve the outer `ID` unchanged in structured diagnostics from submission through callback handling. Repeated controller callbacks for the same Proposal, BlockPart, Vote, or other selected instance retain the same `ID` and MUST each reach the native reinjection boundary unless that individual callback is rejected under Section 8.
- **CMT-ADAPTER-015:** A callback succeeds when the selected message has been validated, decoded, associated with the authenticated source Peer, and safely handed to the ordinary Reactor or subsystem receive entry point or its bounded input queue. Success MUST be described as reinjection acceptance and MUST NOT assert that the Consensus Reactor processed the message, advanced Height/Round/Step, changed a VoteSet, accepted a Proposal, or persisted State Sync data.
- **CMT-ADAPTER-016:** `Type` MUST preserve every protocol-role distinction that the controller needs for scheduling even when multiple roles share one concrete native message type. In particular, a Prevote and a Precommit carried by the native `Vote` message MUST have distinct labels derived from the signed vote type; `nil` votes MUST retain that distinction. The adapter MUST NOT collapse both roles into a generic `Vote` label or require the controller to inspect `Data` to distinguish them.

## 6. Integration Constraints

- **CMT-INTEGRATION-001:** Place outbound interception at the narrowest shared boundary found by source analysis where the destination identity and native message are both available and every selected send mode can be covered.
- **CMT-INTEGRATION-002:** Preserve the ordinary P2P transport for connection authentication, peer lifecycle, and unselected traffic. Instrumentation MUST compose with the existing transport rather than replace unrelated networking behavior.
- **CMT-INTEGRATION-003:** Peers created by every connection direction MUST participate in interception and source lookup. Connection cleanup and replacement MUST remove stale mappings.
- **CMT-INTEGRATION-004:** Reuse the ordinary node-construction path. Prefer a transport factory, dependency option, or small conditional over copying the node constructor or Reactor setup.
- **CMT-INTEGRATION-005:** Preserve existing connection filters, peer limits, address-book behavior, persistent-peer behavior, logging setup, metrics setup, and service ordering unless a clause explicitly requires a localized change.
- **CMT-INTEGRATION-006:** The callback handler MUST validate and enqueue an immutable message without calling a Reactor while holding HTTP, queue, or peer-map locks.
- **CMT-INTEGRATION-007:** Use the standard library or an existing HTTP facility. Do not add a production web-framework dependency solely for controller communication.
- **CMT-INTEGRATION-008:** Do not add a protocol-mutating callback endpoint or modify a consensus transition to signal readiness. Readiness belongs to the instrumentation transport or node lifecycle.
- **CMT-INTEGRATION-009:** Structured instrumentation diagnostics MUST expose submission, callback rejection, reinjection acceptance, and reinjection failure using the core message fields. They MUST remain outside Consensus State and MUST NOT require changes to Reactor state transitions.
- **CMT-INTEGRATION-010:** After all construction-time options and custom Reactor replacements have been applied, but before instrumentation readiness, verify that every selected Channel is still owned by the expected native Consensus or State Sync Reactor and uses a compatible registered prototype, wrapper, and codec. Merely finding a non-empty Reactor and prototype for the numeric Channel is insufficient. An override of a selected route that cannot be proved compatible MUST fail startup as a binding failure under `CORE-LIFE-002`; unrelated custom Reactors MUST remain usable.

## 7. Configuration and Lifecycle

- **CMT-LIFE-001:** Add an explicit, default-off instrumentation configuration using naming and validation conventions already present in the checked-out revision.
- **CMT-LIFE-002:** Configuration MUST provide a callback listen/advertise address, controller address, and experiment-assigned integer node ID. Invalid or incomplete enabled configuration MUST fail before the node joins the experiment.
- **CMT-LIFE-003:** Integrate enablement into the ordinary startup path or a thin command wrapper that delegates to it. Do not create a second independently maintained node implementation.
- **CMT-LIFE-004:** If the repository contains testnet-generation tooling, extend it minimally so generated nodes receive unique callback addresses and IDs plus the common controller address. Do not duplicate the generator.
- **CMT-LIFE-005:** Start the callback listener, delivery worker, and successful registration before allowing selected sends. This MUST work for nodes that begin immediately and nodes that first synchronize.
- **CMT-LIFE-006:** Shutdown MUST cancel controller requests and workers, stop callback acceptance, close the existing transport, and remain safe after partial startup.
- **CMT-LIFE-007:** Empty queues MUST block on an event or use a reasonable timer; no instrumentation worker may busy-spin.

## 8. Failure Mapping

- **CMT-ERROR-001:** Invalid JSON, invalid adapter payload, unknown route supplied by the callback, wrong destination, stale source, oversized payload, native decode failure caused by supplied bytes, or `Type` mismatch is a message-scoped rejection under `CORE-ERR-002`. Reject that instance, record it, and continue.
- **CMT-ERROR-002:** A route that is registered as selected but has no local decoder or owning handler, a valid selected outbound message that cannot be encoded or labeled, or corruption of internal adapter state is a local invariant failure under `CORE-ERR-004` and MUST stop the experiment through orderly fatal shutdown.
- **CMT-ERROR-003:** Exhausting the bounded controller-registration retry budget before readiness is a startup failure under `CORE-ERR-006` and `CORE-REG-007`. After readiness, controller connection failure, submission timeout, a non-`2xx` submission response, malformed submission response behavior, or unexpected callback-listener failure is a fatal controller/control-channel failure under `CORE-ERR-005`.
- **CMT-ERROR-004:** A full non-blocking local submission queue MAY return the ordinary send failure and leave the node running only when failure is reported before acceptance. Any loss after a successful return is fatal.
- **CMT-ERROR-005:** Fatal errors MUST reach the node owner or command runner so the process exits non-zero after orderly cleanup. Deep transport or HTTP code MUST NOT hide the failure, panic uncontrollably, or call an abrupt process exit.

## 9. Conformance Verification

The conformance plan is derived from the integration map rather than from assumed symbol names.

- **CMT-TEST-001:** Derive the test plan from the integration map and add a selected-route inventory test that compares the adapter's selection with the current Consensus and State Sync registrations.
- **CMT-TEST-002:** For every discovered outbound send mode, prove that selected messages reach a fake controller and never invoke direct transport, while representative unselected messages retain the ordinary path.
- **CMT-TEST-003:** Prove that the adapter payload round-trips every selected concrete message family through the current native wrapper, codec, registry, and unwrap behavior.
- **CMT-TEST-004:** Prove that callback delivery reaches the correct existing handler with the original logical peer and route, including deliberate reorder and duplication.
- **CMT-TEST-005:** Prove that selected direct-P2P inbound traffic is rejected only in instrumentation mode.
- **CMT-TEST-006:** Prove the error split: malformed individual callbacks are dropped while the node remains usable; local invariant failures and controller/control-channel failures cause orderly non-zero termination without direct fallback.
- **CMT-TEST-007:** Test accepted and dialed peers, reconnection, stale-message rejection, concurrent IDs, bounded queues, partial startup, and repeated shutdown under the race detector where supported.
- **CMT-TEST-008:** Run the repository's existing focused formatting, unit, static-analysis, and race commands for every changed package. Discover commands from the current repository rather than assuming historical package paths, and record exact commands and results.
- **CMT-TEST-009:** The conformance suite MUST exercise at least one round-state message, Proposal plus BlockPart flow or its revision-local equivalent, Prevote, Precommit, vote-set reconciliation message, and each selected State Sync family. Each case MUST identify the native type and Channel discovered by `CMT-ANALYSIS-009`.
- **CMT-TEST-010:** A lightweight fake controller MAY be used only by tests. The production patch MUST NOT add a controller executable, scheduler, persistence layer, or policy engine.
- **CMT-TEST-011:** With a fake controller that immediately returns every submitted message unchanged, prove pass-through equivalence at the instrumentation boundary: the native route, message bytes, concrete decoded type, logical source Peer, reinjection order, and multiplicity MUST match the corresponding ordinary P2P receive behavior.
- **CMT-TEST-012:** Prove that the same outer `ID` correlates submission and callback diagnostics, that duplicate callbacks retain that `ID` without local deduplication, and that successful callback responses are reported only as reinjection acceptance rather than Consensus execution or state change.
- **CMT-TEST-013:** Validate the instrumentation manifest against the selected Channel and message inventory, discovered send and receive hooks, changed files and symbols, configuration bindings, and recorded command results. A stale, incomplete, or contradictory manifest fails conformance.
- **CMT-TEST-014:** Prove that controller-visible labels distinguish Prevote from Precommit for both non-`nil` and `nil` votes. Callback validation MUST reject a message whose recomputed vote-role label does not match outer `Type`, without requiring the controller to decode `Data`.
- **CMT-TEST-015:** Exercise construction-time extension compatibility. Replacing the Consensus Reactor or State Sync Reactor that owns a selected route with an unproved custom Reactor MUST fail before readiness, while adding a custom Reactor that does not own or alter a selected route MUST preserve enabled startup and ordinary behavior.

## 10. Acceptance Traces

### Scenario A: controlled drop

The sender reports successful submission of a selected Prevote to the controller, the controller does not call the destination, and the destination never invokes the native Consensus Reactor receive entry point for that vote.

### Scenario B: controlled reorder and duplication

The controller forwards selected Proposal, BlockPart, and Precommit instances in a chosen order and repeats one Precommit. The destination invokes or enqueues work for the existing Consensus Reactor receive entry point in exactly the callback order and multiplicity, with the authenticated original Peer as source. This scenario does not assert that every instance changes Consensus State.

### Scenario C: malformed forwarded message

The destination rejects and records one malformed callback, remains running, and subsequently accepts a valid callback.

### Scenario D: controller failure

After readiness, controller submission times out or returns non-success. No direct send occurs; instrumentation becomes unhealthy, the node shuts down cleanly, and the experiment process exits non-zero.

### Scenario E: ordinary traffic

An unselected transaction, Evidence, Block Sync, or peer-exchange message uses the existing P2P path and retains its original return, error, and receive behavior.

### Scenario F: immediate pass-through

The controller immediately forwards each selected NewRoundStep, Proposal, BlockPart, Prevote, Precommit, vote-set, or State Sync message without changing the outer object or adapter payload. The destination reaches the same native receive boundary with the same route, decoded message, logical Peer, order, and multiplicity that the ordinary P2P path would provide. Callback success establishes reinjection acceptance only.
