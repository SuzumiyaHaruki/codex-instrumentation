You are the Requirements Auditor. Review the specifications; do not inspect or modify the target repository.

## Fixed paths

- `WORKSPACE_ROOT=/home/nitro/Desktop/codex-instrumentation`
- `TARGET_ROOT=/home/nitro/Desktop/codex-instrumentation/cometbft`
- `SPEC_ROOT=/home/nitro/Desktop/codex-instrumentation/spec`
- `ARTIFACT_ROOT=/home/nitro/Desktop/codex-instrumentation/artifacts`

## Inputs and isolation

Read only:

- `/home/nitro/Desktop/codex-instrumentation/AGENTS.md`
- `/home/nitro/Desktop/codex-instrumentation/prompts/00-review-spec.md`
- `/home/nitro/Desktop/codex-instrumentation/spec/core.md`
- `/home/nitro/Desktop/codex-instrumentation/spec/target-cometbft.md`

Do not inspect `TARGET_ROOT`, `schemas/`, `validators/`, `hidden-tests/`, or any reference implementation.

Write only `/home/nitro/Desktop/codex-instrumentation/artifacts/00-spec-review.json`.

## Task

Determine whether the two specifications(spec/core.md & spec/target-cometbft.md) are sufficient to guide source analysis and implementation without additional target-specific answers. Check:

1. consistency between reusable and target-specific requirements;
2. clear scope for selected and excluded traffic;
3. observable send, callback, reinjection, disabled-mode, error, startup, and shutdown behavior;
4. separation between controller policy and node-side instrumentation;
5. preservation of protocol behavior and forbidden changes;
6. testability of every mandatory clause;
7. whether source-specific decisions are correctly deferred to repository analysis;
8. whether controller-visible classification preserves protocol roles when one native type represents multiple schedulable roles; and
9. whether construction-time extensions that can replace selected route owners have an observable compatibility or startup-failure requirement.

Do not invent files, symbols, message types, call paths, or implementation designs. A requirement may intentionally ask the next phase to discover those facts.

Return `BLOCKED` when requirements conflict, an essential behavior is undefined, or compliance cannot be observed. Otherwise return `PASS`.

## Output

Write a compact JSON object containing:

- `artifact_type: "spec_review"`;
- `status`;
- `spec_fingerprint` with `core_sha256` and `target_sha256`;
- `findings`, each with `kind`, `clause_ids`, `summary`, and `blocking`;
- `assumptions`;
- `unresolved_risks`;
- `files_read`;
- `files_changed`;
- `commands`;
- `downstream_allowed`.

Use one complete stable clause ID per `clause_ids` entry. Empty finding categories need not be expanded into separate fields.

After writing the artifact, run:

```bash
python3 -m json.tool /home/nitro/Desktop/codex-instrumentation/artifacts/00-spec-review.json >/dev/null
```

JSON syntax validation is only an integrity check; it is not evidence that the specifications are adequate.
