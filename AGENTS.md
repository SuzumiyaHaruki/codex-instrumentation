# Specification-guided instrumentation experiment

## Sources of truth

1. Requirements under `spec/` are authoritative.
2. The current phase prompt defines the allowed read and write scope.
3. Do not weaken, reinterpret, or silently ignore a specification clause.
4. When a requirement is ambiguous or contradictory, report it and stop.

## File access

- Read only paths explicitly listed in the current phase prompt.
- Write only paths explicitly listed in the current phase prompt.
- Do not run repository-wide `find`, `grep`, `rg`, `git grep`, or similar
  commands unless the prompt explicitly authorizes repository-wide access.
- Every source-inspection command must name an allowed path.
- If an out-of-scope file appears necessary, do not read it.
  Return a `SCOPE_EXPANSION_REQUEST` with:
  - requested path;
  - reason;
  - hypothesis to verify;
  - expected impact.

## Protected artifacts

Never modify:

- `spec/`
- `schemas/`
- `validators/`
- `hidden-tests/`
- original regression tests, unless a prompt explicitly permits adding a
  separate new test file.

Do not delete, skip, weaken, or rewrite tests to make a patch pass.

## Protocol safety

Do not modify:

- consensus validity conditions;
- voting or quorum rules;
- protocol state transitions;
- protocol message schemas;
- persistent data formats;
- existing channel or message identifiers;
- existing timeout semantics.

## Patch discipline

- Keep changes minimal and localized.
- Do not perform unrelated refactoring.
- Do not add a production dependency without explicit permission.
- Preserve existing error and return-value behavior unless the spec says otherwise.
- Every changed function must be associated with at least one specification clause.

## Required reporting

At the end of each phase report:

1. status: PASS, BLOCKED, or FAILED;
2. files and symbols read;
3. files changed;
4. specification clauses addressed;
5. commands executed and results;
6. assumptions;
7. unresolved risks;
8. scope expansion requests.
