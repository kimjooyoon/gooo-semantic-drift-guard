# Semantic drift guard protocol v1

## Immutable inputs

A comparison input names two `gooo/semantic-drift-guard/bundle/v1` bundles.
Every source, semantic IR, and generated Go artifact is read from the named
path and checked against its declared `sha256:` digest. A release digest binds
the release id, tag, and all three artifact digests; the observed release
digest must equal the declared release digest.

## Canonical graph

The `.gooo` metacode contains one graph declaration and twelve activity
declarations. The compiler ignores blank lines, comments, whitespace, and
declaration order. It emits a sorted graph with activity nodes and one
`MEASURES` relation per activity. The graph digest is computed from that
canonical JSON graph.

The IR binds the source path, source semantic digest, contract digest, graph
digest, and complete graph. Generated Go carries detached JSON binding metadata
with the source path, IR path and digest, graph digest, and one-to-one node and
relation ID lists. A bundle is usable only when source, IR, and generated
metadata agree.

## Decision lattice

`CLOSED` means both canonical graphs and all bound replay values are equal.
Raw artifact bytes may differ when the canonical graph remains equal; this is
the formatting/comment/order equivalence case.

`REFUTED` means a canonical activity, relation, or authority changed, a replay
value digest mismatched, or the read-only authority boundary was escalated.

`UNKNOWN` means evidence is not sufficient to decide, including a missing,
stale, or ambiguous source/IR/generated binding or replay binding. Every
UNKNOWN record carries all six required fields. Final precedence is
`REFUTED > UNKNOWN > CLOSED`.

## Outputs and authority

The evaluator writes only `comparison-report.json`, `decision-receipt.json`,
`replay-receipt.json`, `metrics.json`, and `human-report.md` beneath the
caller-owned output directory. It records `repository_writes=0`,
`local_test_executions=0`, and `cross_project_required_gates=0` as the runtime
authority boundary. Build and test timings are observations supplied by CI;
the evaluator does not run tests.
