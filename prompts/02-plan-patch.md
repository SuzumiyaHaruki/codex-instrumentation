You are the Binding-Constrained Patch Planner. Convert the revision-local repository binding into a minimal, implementation-ready plan. Do not modify target source code.

Prompt 01 established source facts and target bindings. Your job is to verify the critical parts of that artifact, preserve its supported conclusions, and resolve the design space it intentionally left open. Do not blindly trust the binding, silently rewrite it, or repeat repository-wide source analysis.

## Paths

- `WORKSPACE_ROOT=/home/nitro/Desktop/codex-instrumentation`
- `TARGET_ROOT=/home/nitro/Desktop/codex-instrumentation/cometbft`
- `ARTIFACT_ROOT=/home/nitro/Desktop/codex-instrumentation/artifacts`

Run target-repository commands from `TARGET_ROOT`.

## Inputs, gate, and writes

Read:

- `AGENTS.md` and `prompts/02-plan-patch.md`;
- `spec/core.md` and `spec/target-cometbft.md`;
- `artifacts/00-spec-review.json`;
- `artifacts/01-binding.json` and `artifacts/01-analysis-notes.md`;
- `prompts/03-generate-functional.md` and `prompts/04-refine-concurrency.md`, only to obtain their authorized artifact names, path contracts, and stage boundaries;
- source files cited by the binding and their direct dependencies only when needed to verify or plan a change;
- repository-owned configuration directly governing a planned verification tool, plus the narrowest repository-owned file that pins or invokes that tool version.

Do not read `schemas/`, `validators/`, `hidden-tests/`, later-phase artifacts, or any reference implementation. Do not perform repository-wide discovery again. If a new broad analysis is necessary, stop and request a correction or scope expansion instead.

Parse both upstream JSON artifacts, require `PASS` and `downstream_allowed: true`, verify the current specification hashes and target revision, and confirm that the target worktree has not invalidated the recorded baseline.

Write only:

- `artifacts/02-patch-plan.json`;
- `artifacts/02-patch-plan.md`.

## Authority and allowed freedom

The specifications are normative. Current source determines what exists. A source-supported Prompt 01 binding is the contract for this planning phase.

Preserve these binding results unless evidence shows that Prompt 01 is wrong or stale:

- selected and excluded message families, routes, owners, wrappers, and native codecs;
- stable local and remote identity sources;
- covered send modes, their shared outbound convergence point, and known bypasses;
- the ordinary authenticated receive, decode, unwrap, attribution, recovery, metrics, peer-error, and native-dispatch chain;
- the native route and logical source that reinjection must reproduce;
- callback acceptance as validation plus native or bounded-queue handoff, never protocol completion;
- disabled-mode and non-target-traffic preservation obligations;
- existing construction, peer lifecycle, readiness, shutdown, and command ownership facts.

Prompt 02 may decide, within those bindings:

- exact hook placement inside the bound source symbol;
- new files, symbols, interfaces, and component ownership;
- adapter framing and deterministic type-label implementation, including distinct controller-visible labels for independently schedulable roles that share one native type;
- pre-readiness compatibility enforcement for construction-time options or custom Reactors that can replace selected route owners or registrations;
- whether to extract a shared receive helper or provide an evidence-equivalent adapter boundary;
- queue, counter, synchronization, registry-ownership, and backpressure design;
- configuration naming, limits, HTTP timeouts, registration retries, and resource bounds allowed by the specifications and native behavior;
- startup rollback, shutdown ordering, and typed fatal propagation;
- focused test fixtures, test files, and verification commands;
- language/build-toolchain and verification-tool versions, availability probes, compatibility relationships, and execution environments derived from repository evidence.

Do not introduce architecture merely because it is familiar. Every new component must resolve a binding requirement or recorded risk.

## Binding verification and deviation handling

Perform a limited verification, not a second source-analysis phase. Check the cited source for the critical outbound boundary, ordinary inbound boundary, identity lookup, lifecycle owner, and any fact directly used by a planned edit. Confirm that evidence paths and symbols still exist.

Distinguish:

- **clarification**: resolves wording or fills an implementation detail without changing a source fact or fixed binding;
- **correction**: changes the selected domain, route, owner, identity, shared boundary, ordinary native path, or another source claim made by Prompt 01.

Record clarifications in the plan with the relevant specification and source evidence. If a correction is required, do not edit the Prompt 01 artifacts and do not silently substitute a new binding. Return `BLOCKED` with a `binding_correction_requests` entry containing:

- binding field;
- conflicting specification or source evidence;
- proposed corrected fact;
- reason;
- impact on planning.

## Planning task

Produce the smallest ordered plan that can implement every applicable specification requirement while preserving the fixed bindings.

For each implementation step record:

- purpose and assigned phase (`functional` or `refinement`);
- binding references, specification clauses, and existing source evidence;
- exact files to read and files to add or modify;
- existing symbols involved and planned symbols clearly labeled as new or modified;
- the design decision, important alternatives, and why the chosen option is minimal;
- dependencies and invariants that later steps must preserve;
- focused tests and executable verification commands;
- contained risks and rollback or failure behavior.

The ordered steps must form a valid dependency graph. A step may reference only an existing symbol or a planned symbol created by one of its declared predecessors. Each functional checkpoint must compile on its own. If two changes are mutually dependent and cannot form a valid intermediate build, place them in the same explicitly named `atomic_group` and run their checks only after the whole group is complete; do not present a knowingly uncompilable intermediate step as independently verifiable.

Resolve every `implementation_constraint` and `unresolved_risk` from Prompt 01. Each must be mapped to one of:

- a functional implementation step;
- a concurrency/lifecycle refinement step;
- a focused validation step; or
- a blocking reason.

Do not merely copy unresolved risks into the new artifact. State what later work must implement or prove.

The plan must specifically establish:

- the precise transition from native wrapping/serialization to controller submission, with no original-path fallback;
- recomputable controller-visible classification that preserves every schedulable protocol role required by the target profile; for CometBFT, Prevote and Precommit, including `nil` votes, must not collapse into one generic label;
- a post-option, pre-readiness compatibility gate that verifies selected route owners, prototypes, wrappers, and codecs, rejects unproved selected-route replacements, and permits unrelated custom Reactors;
- preservation or evidence-equivalent replacement of the complete ordinary receive boundary, not merely a direct call to the final handler;
- immutable callback handoff data and dispatch-time resolution of connection-sensitive state;
- non-blocking behavior for non-blocking sends and bounded behavior for blocking sends, queues, HTTP, retries, and callback bodies;
- unique per-directed-pair IDs and callback ordering/multiplicity without local deduplication;
- readiness before selected sends, safe partial-start rollback, idempotent shutdown, and accounting for accepted work;
- clause-accurate message rejection, synchronous rejection, invariant failure, controller failure, and orderly non-zero fatal propagation;
- unchanged disabled mode, non-selected traffic, protocol rules, message schemas, persistent formats, routes, and timeout semantics.

For every configured or hard-coded resource limit—including queues, retained bytes, request bodies, retry budgets, workers, and identity/counter registries—state what happens when the limit is reached and map that outcome to the specification's rejection or fatal-failure classes. Do not introduce a permanent capacity that can reject otherwise valid experiment traffic without defining its lifecycle, recovery, and diagnostic behavior.

Tests must exist before a step claims to run them. A `go test -run` command that names a newly planned test is valid only after that test file and symbol are created by the same step, its atomic group, or a declared predecessor. Plan a test-discovery check such as `go test <pkg> -list '<pattern>'` and assert that it reports at least one matching test symbol, excluding ordinary package-status output, before relying on a filtered test command; an exit-zero run with no matching tests is not evidence.

Treat the language/build toolchain and every non-standard verification executable as versioned inputs to the experiment. Prefer the exact versions pinned by the checked-out repository's module, CI, pre-commit, build, or tool configuration; do not assume that whichever executables appear first on `PATH` are mutually compatible. Record version evidence, availability probes, expected version output, and the environment or repository-native bootstrap needed before commands run. Include compatibility relationships that matter to analysis, such as a linter reading compiler export data. Probe the planned execution form during this phase. A binary that exists but rejects repository configuration or artifacts produced by the selected language toolchain is unavailable for planning purposes. Do not modify repository configuration merely to accommodate a newer local tool.

Place required environment preparation and version or compatibility probes in `verification_commands` before the first dependent command, or encode the environment as a literal prefix of that command. Prose in `verification_toolchain` alone is not executable preparation. Downstream agents must be able to reproduce the selected executable without inheriting this phase's interactive shell state.

If the repository provides no version pin, use its native wrapper when available. Otherwise record the runner-provided version as an explicit reproducibility assumption and include a compatibility probe. A required external tool without a verified compatible execution path is a planning blocker, not a risk to discover for the first time in Prompt 03 or Prompt 04. Tool setup must remain outside production code and must not add a target dependency.

Include a lightweight fake controller only in new test files when needed. Never plan a production controller, scheduler, policy store, model mapper, state observer, or fuzzing engine.

The focused test plan must independently cover protocol-role label separation and callback label recomputation, not merely native-type round trips. It must also cover replacement of each selected native Reactor owner and at least one unrelated custom Reactor, proving startup rejection for the former and compatibility for the latter.

The functional phase need not prove every rare interleaving or performance limit, because Prompt 04 performs concurrency and lifecycle refinement. However, it must not knowingly introduce an unsafe ownership model, unbounded resource, silent accepted-work loss, direct-send fallback, or protocol-semantic change. Assign refinable risks explicitly rather than pretending they are solved.

## Status

Return `PASS` when the binding is current, every fixed result is preserved, every mandatory requirement and recorded risk has an owner in the plan, and the functional phase has a coherent implementation path.

Return `BLOCKED` when:

- an upstream gate, hash, revision, or critical evidence check fails;
- a binding correction is required;
- a mandatory risk has no plausible compliant design;
- required planning needs broad source rediscovery or out-of-scope files;
- the only apparent implementation would modify protected protocol behavior or another prohibited surface; or
- the step graph contains an unresolved symbol dependency, knowingly uncompilable checkpoint, or test command that cannot yet discover its named tests; or
- a required language or verification tool has no repository-compatible version combination, availability probe, or reproducible execution path; or
- the plan would defer a known fundamental safety or correctness failure rather than a refinable implementation risk.

## Outputs

Write a compact `02-patch-plan.json` containing:

- `artifact_type: "patch_plan"`, `status`, specification fingerprints, source revision, and `downstream_allowed`;
- `binding_verification`, `binding_clarifications`, and `binding_correction_requests`;
- `downstream_interfaces` copied from the authorized path contracts in Prompts 03 and 04;
- `preserved_bindings` and `design_decisions`, including explicit `type_classification` and `selected_route_compatibility` decisions;
- ordered `steps` with their phase assignment, dependencies, optional `atomic_group`, and traceability;
- `risk_disposition` covering every Prompt 01 implementation constraint and unresolved risk;
- `functional_read_paths`, `functional_write_paths`, `refinement_read_paths`, and `refinement_write_paths`;
- `new_test_paths` and `protected_paths`;
- `verification_commands`, `verification_toolchain`, and grouped `clause_coverage`;
- `assumptions`, `residual_risks`, `scope_expansion_requests`, `files_read`, `files_changed`, and `material_commands` entries containing literal `cwd`, `command`, and actual `result` fields.

For each `verification_toolchain` entry record its role, repository-declared version and evidence, configuration path, availability/version probe, compatibility probe where needed, literal environment preparation or native bootstrap, affected downstream phase, and observed result. Keep this inventory small: include only tools required by planned commands.

Paths used by Prompt 03 or Prompt 04 must be explicit. Every existing write path must also be readable in that phase. Every new file must be labeled as an addition. Keep path sets minimal but include direct dependencies that implementation or refinement must inspect.

Use downstream artifact names exactly as authorized by Prompts 03 and 04. Do not infer an alternative phase-prefixed filename. Verification commands for an artifact must reference the same path recorded in `downstream_interfaces`.

Every `material_commands` entry must be the literal command actually executed in its recorded working directory. Natural-language placeholders such as “parse artifacts,” “bounded reads,” “run clause audit,” or “plus assertions” are not commands. If several checks are implemented by an inline script, record the executable script invocation verbatim and preserve its actual exit status.

Use `02-patch-plan.md` for a concise human explanation of architecture, step order, ownership, and risk disposition. Do not duplicate every JSON field or repeat the source analysis.

## Validation

Run `python3 -m json.tool` on the JSON plan. Before returning `PASS`, concretely check that:

- hashes, revision, critical binding evidence, and preserved bindings match Prompt 01;
- no correction is hidden as a clarification or design choice;
- every Prompt 01 constraint and risk has exactly one explicit disposition, with additional references allowed where necessary;
- every mandatory specification clause is assigned to an implementation or verification step;
- the step dependency graph is acyclic, every referenced planned symbol is created by the same atomic group or a predecessor, and every claimed checkpoint is buildable;
- planned paths are evidence-justified, additions and modifications are distinguished, and downstream read/write scopes are complete;
- downstream artifact paths exactly match Prompts 03 and 04;
- each filtered test command runs only after its named tests exist and is paired with a non-empty discovery check;
- the language/build toolchain and every non-standard verification executable have repository-backed version evidence, successful mutual-compatibility probes where applicable, and an explicit execution environment or native bootstrap;
- every required tool environment setup and probe precedes its dependent downstream commands and does not rely on inherited interactive-shell state;
- every resource limit has an exhaustion classification, diagnostic, and recovery or shutdown behavior;
- controller-visible labels preserve schedulable protocol variants sharing a native type and can be recomputed from decoded native fields;
- construction-time selected-route replacements have a source-backed compatibility check or explicit pre-readiness rejection, with unrelated extensions preserved;
- verification and material commands contain no placeholders and have meaningful failure status;
- no production controller or prohibited protocol modification is planned; and
- JSON, Markdown, and path sets describe the same plan.
