You are the Repository Binder. Translate the reusable specifications into a concise, revision-local map of the target repository. Do not modify source code and do not produce a detailed patch plan.

A `PASS` from this phase means that the next agent has enough source-grounded information to plan a plausible compliant implementation. It does not mean that every implementation detail, race, or test outcome has already been proved.

## Paths

- `WORKSPACE_ROOT=/home/nitro/Desktop/codex-instrumentation`
- `TARGET_ROOT=/home/nitro/Desktop/codex-instrumentation/cometbft`
- `SPEC_ROOT=/home/nitro/Desktop/codex-instrumentation/spec`
- `ARTIFACT_ROOT=/home/nitro/Desktop/codex-instrumentation/artifacts`

Run repository commands from `TARGET_ROOT`. Read-only repository-wide discovery is allowed only under that directory.

## Inputs, gate, and writes

Read:

- `AGENTS.md` and `prompts/01-analyze-source.md`;
- `spec/core.md` and `spec/target-cometbft.md`;
- `artifacts/00-spec-review.json`;
- source and tests under `TARGET_ROOT`.

Do not read `schemas/`, `validators/`, `hidden-tests/`, later-phase artifacts, or any reference implementation outside `TARGET_ROOT`.

Before inspecting target source, parse the upstream artifact, require `status: "PASS"` and `downstream_allowed: true`, and verify both specification hashes. Otherwise return `BLOCKED` without inspecting target source.

Write only:

- `artifacts/01-binding.json`;
- `artifacts/01-analysis-notes.md`.

## Objective

Bind the following specification concepts to the checked-out revision:

1. selected and explicitly excluded native message families, using the actual route or channel registrations and wrapper registry;
2. all blocking, non-blocking, broadcast, reply, gossip, and retry paths for selected traffic, plus the narrowest shared outbound interception boundary and any lower bypass;
3. the ordinary inbound path from authenticated connection and native bytes through route lookup, decode, unwrap, source attribution, recovery, metrics, peer-error behavior, and native subsystem dispatch;
4. a plausible controller-callback reinjection boundary that preserves native route, bytes, concrete message type, and current logical source peer without inventing a second protocol representation;
5. stable local and remote identities and their behavior across accepted peers, dialed peers, disconnect, removal, and reconnection;
6. existing configuration, node construction, readiness, startup, shutdown, testnet generation, and fatal-error ownership to which instrumentation can be attached;
7. implementation constraints imposed by existing send return values, timeout bounds, message-size bounds, peer lifecycle, and native handler behavior, plus repository-declared language/tool versions, verification configuration, and native CI or wrapper commands;
8. controller-visible classification for every schedulable protocol role, including roles that share one concrete native type, and the exact native discriminator from which each label can be recomputed after callback decoding;
9. construction-time options, custom Reactor facilities, and registration hooks that can replace or alter selected route owners, prototypes, wrappers, or codecs after ordinary assembly, including evidence for distinguishing unrelated extensions from incompatible overrides; and
10. the smallest likely source surface and focused tests that the planning phase should examine.

Use the checked-out source as evidence. Historical names in the target specification are orientation; current registrations and call paths are authoritative.

## Analysis discipline

Separate three kinds of statements:

- **existing fact**: a current file, symbol, call path, registration, or behavior supported by source evidence;
- **binding decision**: the selected revision-local boundary or mapping, with a short reason and credible alternatives when relevant;
- **planning constraint**: a risk or obligation that the patch planner must resolve, without prescribing an unproved implementation.

Do not present planned files, symbols, locks, queues, or services as existing facts. Prompt 01 may identify a likely integration surface, but Prompt 02 owns the concrete patch structure and Prompt 04 owns detailed concurrency refinement.

Trace enough callers and callees to establish completeness. In particular, do not stop after finding only one send mode, one connection direction, or the final native handler. At the same time, avoid exhaustive narration once a shared boundary has been proved.

For callback reinjection, identify what must be validated before acceptance, what stable data may cross an asynchronous handoff, and what connection-sensitive state must be resolved again at dispatch. Record relevant hazards such as a native handler synchronously stopping its source peer, shutdown racing accepted work, or a registry changing. These are planning constraints unless the source shows that no plausible compliant implementation exists.

Callback `2xx` means only that validation and the specification-defined native handoff or bounded queue handoff succeeded. It must not wait for, or claim, protocol-handler completion, application work, persistence, or protocol-state change. Any possible loss after acceptance must be visible to later planning as a post-acceptance integrity failure rather than a silent drop.

For outbound calls, preserve the distinction between blocking and non-blocking APIs. Identify existing work and bounds on each path and flag any proposed state lookup, counter, lock, serialization, or queue operation that could change return behavior. The detailed synchronization algorithm belongs to later phases.

Do not assume that one concrete native type implies one controller-visible `Type`. When protocol roles sharing a native type are independently schedulable, bind the native discriminator and record distinct required labels. For CometBFT votes, establish from source how Prevote and Precommit, including their `nil` forms, are distinguished without interpreting the opaque adapter payload in the controller.

Analyze registrations after all ordinary construction options are applied. If a custom Reactor or extension hook can replace an owner of a selected route, record the original owner, the replacement mechanism, the compatibility evidence available at startup, and whether the safest compliant policy is validation or explicit rejection. A non-empty route registration alone is not compatibility evidence.

Inspect only enough repository-owned build, CI, pre-commit, and verification configuration to identify declared language versions, non-standard tool versions, configuration files, and native command entry points. Record missing pins as facts. Do not install tools, choose host-specific paths, or test local tool availability here; Prompt 02 owns environment compatibility and executable planning.

Do not implement or design a production controller. A later validation phase may add a test-only fake controller.

## Phase boundary and status

Return `PASS` when all mandatory bindings are supported by source evidence and at least one plausible implementation path remains. Record normal implementation risks for the planning, generation, refinement, and validation phases; do not turn every unresolved coding detail into a blocker.

Return `BLOCKED` only when:

- the upstream gate fails;
- the specifications are contradictory or too ambiguous to bind;
- the selected inventory, outbound coverage, stable identity, native reinjection entry, disabled-mode preservation, lifecycle owner, or fatal-error path cannot be established without guessing;
- available source evidence rules out a compliant implementation; or
- additional out-of-scope source is required.

When blocked by scope, return the smallest `SCOPE_EXPANSION_REQUEST` required by `AGENTS.md`.

## Outputs

Write a compact `01-binding.json` with:

- artifact metadata, specification fingerprints, source revision, initial worktree status, `status`, and `downstream_allowed`;
- grouped `selected_messages` and `excluded_messages` containing roles, native types, controller-visible labels and native discriminators, routes, owners, wrappers, and concise evidence references;
- `outbound_binding` containing covered send modes, selected boundary, and bypass risks;
- `inbound_binding` containing the ordinary receive chain and candidate reinjection entry;
- `callback_contract` containing the validation/acceptance boundary, queued stable data, dispatch-time lookups, and post-acceptance responsibility;
- `identity_and_lifecycle` containing identity, peer replacement, configuration, readiness, shutdown, and fatal propagation bindings;
- `extension_compatibility` containing construction-time replacement hooks, expected selected-route owners and registrations, unrelated-extension behavior, startup detection evidence, and unresolved compatibility limits;
- `repository_toolchain` containing repository-declared language and verification-tool versions, version/config evidence, native command entry points, and explicit unpinned tools;
- `implementation_constraints` containing only source-derived obligations and hazards for later phases;
- `candidate_surface` containing existing files and symbols likely to be involved, without inventing detailed additions;
- `test_anchors` and a small set of executable repository-native verification commands;
- deduplicated `evidence`, `files_read`, material `commands`, `assumptions`, `unresolved_risks`, and `scope_expansion_requests`.

Group messages that share a route, owner, wrapper, and evidence instead of repeating one large object per protocol role. Use short evidence identifiers mapped once to `path:line` locations. Record only commands that materially establish a binding or validate the artifact; each command must be literal and actually executable, not a prose pseudocommand.

Use `01-analysis-notes.md` only for brief rationale, rejected boundary alternatives, and risks that are awkward in JSON. Do not duplicate the JSON or restate the specifications. Prefer a small source map that downstream agents can consume over an exhaustive research log.

## Validation

Run `python3 -m json.tool` on `01-binding.json`. Perform lightweight, concrete checks that:

- specification hashes and source revision match;
- every cited existing path exists and was inspected;
- selected roles, registered routes, send modes, and connection directions are covered;
- independently schedulable roles sharing a native type have distinct controller-visible labels backed by a native discriminator;
- selected-route owner or registration replacement hooks and their pre-readiness compatibility evidence are covered;
- existing facts and candidate changes are not conflated;
- repository-declared tool versions and native verification entry points are recorded without claiming host availability;
- callback acceptance is not described as protocol execution;
- unresolved implementation risks are preserved for later phases; and
- the target source worktree was not modified.

Focused read-only tests may be run when they materially verify a binding, but a broad baseline test run is not required in this analysis phase. Record a timeout or interruption as incomplete evidence, never as a pass.
