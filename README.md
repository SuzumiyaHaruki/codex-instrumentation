# Specification-Guided LLM Instrumentation

This repository explores whether an LLM can generate reliable, revision-specific instrumentation for a distributed system from a reusable specification and a target profile.

The current target is CometBFT. In instrumentation mode, selected node-to-node messages no longer travel directly over their original P2P path. The sending node submits them to an external controller, which may deliver, delay, reorder, duplicate, or drop them. Messages chosen for delivery are sent back to the destination node and re-enter the native receive path. The controller itself, protocol-state modeling, and fuzzing policy are outside the scope of this repository.

## Research goal

The experiment separates stable behavioral requirements from revision-local source facts:

- `spec/core.md` defines the reusable controller-mediated delivery contract without depending on a particular consensus implementation.
- `spec/target-cometbft.md` specializes that contract for CometBFT and expresses the required consensus and State Sync coverage.
- `prompts/` divides requirements review, source binding, planning, implementation, hardening, independent validation, and local repair into constrained phases.
- `artifacts/` records the evidence and decisions passed between phases.

This separation is intended to make the process auditable while keeping source analysis and implementation decisions the responsibility of the LLM rather than encoding a reference patch in the specification.

## Message flow

```text
source node
    │ selected outbound message
    ▼
external controller ── drop / delay / reorder / duplicate
    │ forwarded message
    ▼
destination callback ── validate ── native decode and receive path
```

When instrumentation is disabled, the original P2P behavior must remain unchanged. When it is enabled, selected messages must not fall back to direct P2P delivery after controller submission fails.

## Repository layout

```text
codex-instrumentation/
├── AGENTS.md                  # Global experiment boundaries and reporting rules
├── spec/                      # Reusable core and CometBFT target specifications
├── prompts/                   # Seven phase prompts and Chinese translations
├── artifacts/                 # Machine-readable reports and experiment evidence
└── cometbft/                  # CometBFT source snapshot and generated instrumentation
```

The main specifications are available in both English and Chinese:

- [Core specification](spec/core.md) · [中文版](spec/core-zh.md)
- [CometBFT target specification](spec/target-cometbft.md) · [中文版](spec/target-cometbft-zh.md)

## Workflow

Run each phase in order, preferably in a fresh agent context. Tell the agent to read the corresponding prompt first and follow its read/write boundaries exactly.

1. `00-review-spec.md` audits the specification suite without inspecting target source.
2. `01-analyze-source.md` binds the requirements to the checked-out target revision.
3. `02-plan-patch.md` converts the source binding into a minimal implementation plan.
4. `03-generate-functional.md` implements the functional instrumentation once from a clean target revision.
5. `04-refine-concurrency.md` hardens concurrency, lifecycle, reconnection, shutdown, and failure behavior.
6. `05-validate.md` independently checks every stable specification clause and does not repair failures.
7. `06-repair.md` repairs only explicitly selected validation IDs. Run Phase 05 again after every repair round.

`03-generate-functional-resume.md` is a recovery prompt for the recorded interrupted Phase 03 run. It is not the canonical prompt for a fresh experiment.

Example invocation:

```text
Read prompts/00-review-spec.md first and execute that phase exactly as written. Respect AGENTS.md, including all read/write boundaries and reporting requirements.
```

For Phase 06, provide the selected IDs explicitly:

```text
Execute prompts/06-repair.md with selected_violation_ids: VAL-EXAMPLE-001, VAL-EXAMPLE-002.
```

## Reproducing a fresh CometBFT run

The prompts currently use the fixed workspace path `/home/nitro/Desktop/codex-instrumentation`. Clone the repository there or update every fixed path consistently before starting.

The committed `cometbft/` directory is the generated result, not a Git submodule. A fresh run needs a nested target repository at the recorded clean baseline revision:

```bash
cd /home/nitro/Desktop/codex-instrumentation/cometbft
git init
git remote add origin https://github.com/cometbft/cometbft.git
git fetch --depth=1 origin d22299509a50140b74d81b113c4d78e4cf501994
git reset --hard FETCH_HEAD
```

Run these destructive reset commands only in a disposable clone: they replace the committed generated result with the clean CometBFT v1.0.1 baseline. Before Phase 03, independently confirm the baseline commands and known-failure signatures recorded in `artifacts/baseline-report.json`; regenerate that evidence when the revision, toolchain, or environment changes.

The workflow expects a compatible Go toolchain, Python 3 for artifact audits, and the repository's lint tooling. Environment preparation and literal verification commands must be recorded in phase artifacts rather than assumed implicitly.

## Current experiment status

The committed implementation was independently validated as `PASS` for 158 clauses under:

- CometBFT revision `d22299509a50140b74d81b113c4d78e4cf501994` (`v1.0.1`)
- Core specification SHA-256 `c55814f46414d2dbb9a90972fc3b7ca2c3a430f79ae807aaf486c3e04ed6dff8`
- Target specification SHA-256 `88757c2ff6cdd182f846324ab6114ba575b5f714e3930201abeaeb90834413b9`
- Final target-change fingerprint `4725431b94a794616f6ad0ad2ed944eb4c7d4c26441701554a2d4ec60573b716`

The current target specification has since changed to SHA-256 `a4b876a8283e42cf6788321dfd87308fba3aaaef1908a7e307372317f3337dff`. It now strengthens controller-visible Prevote/Precommit labeling and compatibility checks for custom reactors on selected routes. Consequently, the existing artifacts are historical evidence for the earlier specification revision, not a conformance claim for the current files. A new evaluation should restart from Phase 00.

The recorded baseline also contains an exact, isolated known failure in the unmodified upstream `TestStateFullRound1`. The validation policy quarantines only that anchored test and still requires the remaining Consensus suite to pass; unrelated failures cannot reuse the disposition.

## Scope and limitations

- This repository implements node-side submission and callback reinjection, not the central controller.
- It does not add consensus-state-to-TLA+ conversion, model checking, or fuzzing policy.
- The current evidence is revision-specific and should not be generalized to another CometBFT release without rerunning the workflow.
- Adapting the experiment to another system requires a new target specification and target-aware prompt paths; the reusable core contract should remain system-independent.
- The existing tests are extensive but do not replace a real multi-node controller-mediated end-to-end campaign.

## Licensing

The `cometbft/` snapshot retains the upstream CometBFT license in [cometbft/LICENSE](cometbft/LICENSE). No separate license has yet been declared for the experiment-specific specifications, prompts, and artifacts.
