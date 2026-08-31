package guard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DecisionReceipt struct {
	Schema        string         `json:"schema"`
	CaseID        string         `json:"case_id"`
	Decision      string         `json:"decision"`
	Reason        string         `json:"reason"`
	Precedence    []string       `json:"precedence"`
	CellDecisions []CellDecision `json:"cell_decisions"`
	Unknowns      []Unknown      `json:"unknowns"`
	Metrics       Metrics        `json:"metrics"`
}

type ConformanceCase struct {
	CaseID   string `json:"case_id"`
	Decision string `json:"decision"`
	Expected string `json:"expected"`
	Report   string `json:"report"`
}

type ConformanceIndex struct {
	Schema string            `json:"schema"`
	Cases  []ConformanceCase `json:"cases"`
}

func WriteOutputs(outputDir string, report Report) error {
	if outputDir == "" || !filepath.IsAbs(outputDir) {
		return errors.New("output directory must be an absolute caller-owned path")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	decision := DecisionReceipt{Schema: ReportSchema, CaseID: report.CaseID, Decision: report.Decision, Reason: report.Reason, Precedence: report.Precedence, CellDecisions: report.CellDecisions, Unknowns: report.Unknowns, Metrics: report.Metrics}
	if err := writeJSON(filepath.Join(outputDir, "comparison-report.json"), report); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outputDir, "decision-receipt.json"), decision); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outputDir, "replay-receipt.json"), report.Replay); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outputDir, "metrics.json"), report.Metrics); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, "human-report.md"), []byte(HumanReport(report)), 0o644)
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func HumanReport(report Report) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Gooo semantic drift guard report\n\nDecision: `%s`\n\nReason: `%s`\n\n", report.Decision, report.Reason)
	builder.WriteString("Precedence: `REFUTED > UNKNOWN > CLOSED`\n\n")
	builder.WriteString("## Exact metrics\n\n")
	metrics := report.Metrics
	fmt.Fprintf(&builder, "- releases_compared: %d\n- source_files: %d\n- ir_nodes: %d\n- generated_files: %d\n- semantic_relations_before: %d\n- semantic_relations_after: %d\n- equivalent_changes: %d\n- semantic_drift_changes: %d\n- unknown_bindings: %d\n- replay_comparisons: %d\n- replay_mismatches: %d\n- peak_rss_kib: %d\n- wall_ms: %d\n- build_ms: %d\n- test_ms: %d\n- tests_total: %d\n- tests_executed: %d\n- tests_reused: %d\n- tests_skipped: %d\n- tests_not_observed: %d\n- go_physical_lines: %d\n- go_files: %d\n- gooo_physical_lines: %d\n- gooo_files: %d\n- descendant_dirs: %d\n- descendant_files: %d\n\n", metrics.ReleasesCompared, metrics.SourceFiles, metrics.IRNodes, metrics.GeneratedFiles, metrics.SemanticRelationsBefore, metrics.SemanticRelationsAfter, metrics.EquivalentChanges, metrics.SemanticDriftChanges, metrics.UnknownBindings, metrics.ReplayComparisons, metrics.ReplayMismatches, metrics.PeakRSSKiB, metrics.WallMS, metrics.BuildMS, metrics.TestMS, metrics.TestsTotal, metrics.TestsExecuted, metrics.TestsReused, metrics.TestsSkipped, metrics.TestsNotObserved, metrics.GoPhysicalLines, metrics.GoFiles, metrics.GoooPhysicalLines, metrics.GoooFiles, metrics.DescendantDirs, metrics.DescendantFiles)
	builder.WriteString("## Exact 12-cell judgment\n\n| ordinal | id | activity | decision | reason |\n|---:|---|---|---|---|\n")
	for _, cell := range report.CellDecisions {
		fmt.Fprintf(&builder, "| %d | `%s` | `%s` | `%s` | `%s` |\n", cell.Ordinal, cell.ID, cell.Activity, cell.Decision, cell.Reason)
	}
	builder.WriteString("\n## Unknown records\n\n")
	if len(report.Unknowns) == 0 {
		builder.WriteString("None.\n")
	} else {
		for _, unknown := range report.Unknowns {
			fmt.Fprintf(&builder, "- stage: `%s`; step: `%s`; reason: `%s`; unknown_class: `%s`; next_operation: `%s`; blocked_by: `%s`\n", unknown.Stage, unknown.Step, unknown.Reason, unknown.UnknownClass, unknown.NextOperation, strings.Join(unknown.BlockedBy, ","))
		}
	}
	return builder.String()
}
