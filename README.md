# gooo-semantic-drift-guard

`gooo-semantic-drift-guard` is an independent, reusable evidence tool for
deterministically classifying meaning changes between two immutable Gooo
release bundles.

The authority chain is explicit:

```text
.gooo source → canonical semantic graph → semantic-ir.json → generated Go binding
```

The evaluator compares canonical graph nodes and relations. It never closes a
change from a string diff or generated Go text diff. Formatting, comments, and
declaration order can therefore produce `CLOSED` only when the source, IR, and
generated binding all resolve to the same graph. Observable activity, relation,
or authority changes are `REFUTED`. Missing, stale, or ambiguous source/IR/
generated bindings are `UNKNOWN` and preserve `stage`, `step`, `reason`,
`unknown_class`, `next_operation`, and `blocked_by`.

Decision precedence is fixed: `REFUTED > UNKNOWN > CLOSED`.

The denominator is exactly 12 cells with `FOUNDATION`, `COHERENCE`, and
`REGRESSION` at 4 each, and `DRIVER`, `OUTCOME`, and `GUARDRAIL` at 4 each.
The repository includes canonical normal, UNKNOWN, and REFUTED fixtures for
formatting/comment/order equivalence, missing/stale/ambiguous bindings,
relation/activity/authority drift, replay mismatch, and authority escalation.

## Commands

`compile` reads a `.gooo` source and contract and writes IR and generated Go
only to caller-provided absolute output paths.

`compare` reads one comparison input and writes five receipts to an absolute
caller-owned output directory.

`conformance` evaluates every JSON file below `fixtures/cases` and writes
per-case receipts plus `conformance-index.json` and `ci-summary.md` to the
caller-owned output directory.

Examples:

```text
go run ./cmd/gooo-semantic-drift-guard compile \
  -source examples/semantic-drift-guard/main.gooo \
  -contract contracts/semantic-drift-guard-denominator-v1.json \
  -output-ir /tmp/gooo-semantic-ir.json \
  -output-go /tmp/gooo-semantic.gooo.go

go run ./cmd/gooo-semantic-drift-guard conformance \
  -root . -fixtures fixtures/cases -output-dir /tmp/gooo-conformance
```

The CI report emits integer observations only: releases, source/IR/generated
counts, canonical relation counts, equivalent/drift/unknown counts, replay
counts, resource times, build/test observations, test reuse fields, and
Go/Gooo inventory. It emits no score, percentage, or qualitative improvement
claim. A project-root `README.md` is excluded from inventory counts.

This repository does not assert that any self-improvement ledger process has
been closed. It is an independent evidence mechanism; ledger state remains
outside its authority boundary.
