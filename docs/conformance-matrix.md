# Conformance matrix

The fixed denominator is `gooo://denominator/semantic-drift-guard/v1` with
exactly twelve cells.

| proof choice | cells | indicator classes represented |
|---|---:|---|
| FOUNDATION | 4 | DRIVER, OUTCOME, GUARDRAIL |
| COHERENCE | 4 | DRIVER, OUTCOME, GUARDRAIL |
| REGRESSION | 4 | DRIVER, OUTCOME, GUARDRAIL |

| fixture family | canonical cases |
|---|---|
| normal | formatting/comment/order-only graph equality |
| UNKNOWN | missing source, stale release digest, ambiguous IR, missing generated binding |
| REFUTED | relation drift, activity drift, authority drift, replay mismatch, authority escalation |

Every conformance report contains one decision row for each of the twelve
cells. Reports include exact integer metrics and six-field UNKNOWN records.
The conformance process does not modify repository inputs and does not claim a
self-improvement ledger transition.
