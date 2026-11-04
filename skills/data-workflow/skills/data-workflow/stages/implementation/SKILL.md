# Implementation Guardrails

Apply this skill before making code changes or performing code review.

## 1. Think Before Coding

Before implementing:

- State assumptions explicitly
- If something is uncertain, ask instead of guessing
- If there are multiple valid interpretations, surface them
- If a simpler approach exists, prefer it and say so
- If a requirement is unclear, stop and name the ambiguity

## 2. Simplicity First

Choose the minimum solution that satisfies the request.

- Do not add features that were not requested
- Do not introduce abstractions for one-off use
- Do not add speculative configurability
- Do not write defensive code for impossible cases
- If the solution feels larger than necessary, simplify it
- If the ask is essentially “add a list filter”, implement it as a filter (proto → pass-through → WHERE). Do not invent intermediate flags, one-off helpers, or cross-module signature changes for unrequested edge cases. See project rule **R18**.

Checkpoint:

`Would a senior engineer consider this overcomplicated for the problem?`

If yes, reduce scope.

## 3. Surgical Changes

Touch only the code required for the task.

- Do not clean up unrelated code
- Do not refactor adjacent paths unless required
- Match existing local style and patterns
- Remove only the unused code created by your own change
- If you notice unrelated issues, mention them instead of folding them into the diff

Validation rule:

`Every changed line should trace directly to the request.`

## 4. Goal-Driven Execution

Convert the request into verifiable outcomes before coding.

Use a short plan when the task has multiple steps:

```text
1. [Step] -> verify: [check]
2. [Step] -> verify: [check]
3. [Step] -> verify: [check]
```

Prefer concrete checks such as:

- reproduce the bug with a test, then make it pass
- add invalid-input coverage, then make it pass
- verify behavior before and after a refactor

Weak success criteria like `make it work` are not enough when a stronger check is available.
