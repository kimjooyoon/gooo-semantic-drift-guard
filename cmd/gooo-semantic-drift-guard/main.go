package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kimjooyoon/gooo-semantic-drift-guard/internal/guard"
)

func main() {
	if len(os.Args) < 2 {
		fatal("command is required: compile, compare, or conformance")
	}
	switch os.Args[1] {
	case "compile":
		compileCommand(os.Args[2:])
	case "compare":
		compareCommand(os.Args[2:])
	case "conformance":
		conformanceCommand(os.Args[2:])
	default:
		fatal("unsupported command %q", os.Args[1])
	}
}

func compileCommand(args []string) {
	flags := flag.NewFlagSet("compile", flag.ExitOnError)
	source := flags.String("source", "examples/semantic-drift-guard/main.gooo", "source metacode path")
	contract := flags.String("contract", "contracts/semantic-drift-guard-denominator-v1.json", "denominator contract path")
	outputIR := flags.String("output-ir", "", "absolute output path for semantic IR")
	outputGo := flags.String("output-go", "", "absolute output path for generated Go")
	irBindingPath := flags.String("ir-binding-path", "internal/generated/semantic-ir.json", "semantic IR path recorded in generated binding")
	_ = flags.Parse(args)
	if *outputIR == "" || *outputGo == "" {
		fatal("compile requires absolute -output-ir and -output-go paths")
	}
	workingRoot, err := os.Getwd()
	if err != nil {
		fatal("compile working directory: %v", err)
	}
	if err := ensureCallerOutput(workingRoot, *outputIR); err != nil {
		fatal("compile output: %v", err)
	}
	if err := ensureCallerOutput(workingRoot, *outputGo); err != nil {
		fatal("compile output: %v", err)
	}
	if err := guard.WriteCompileOutputs(*source, *contract, *outputIR, *outputGo, *irBindingPath); err != nil {
		fatal("compile: %v", err)
	}
}

func compareCommand(args []string) {
	flags := flag.NewFlagSet("compare", flag.ExitOnError)
	root := flags.String("root", ".", "repository root containing immutable inputs")
	inputPath := flags.String("input", "", "comparison input JSON")
	outputDir := flags.String("output-dir", "", "absolute caller-owned output directory")
	options := runtimeOptions(flags)
	_ = flags.Parse(args)
	if *inputPath == "" || *outputDir == "" {
		fatal("compare requires -input and -output-dir")
	}
	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		fatal("compare root: %v", err)
	}
	if err := ensureCallerOutput(absoluteRoot, *outputDir); err != nil {
		fatal("compare output: %v", err)
	}
	meta, err := guard.LoadMeta(absoluteRoot)
	if err != nil {
		fatal("compare metadata: %v", err)
	}
	raw, err := os.ReadFile(*inputPath)
	if err != nil {
		fatal("compare input: %v", err)
	}
	report := guard.Compare(raw, meta, *options)
	if err := guard.WriteOutputs(*outputDir, report); err != nil {
		fatal("compare output: %v", err)
	}
	if report.Decision == guard.DecisionRefuted {
		os.Exit(2)
	}
}

func conformanceCommand(args []string) {
	flags := flag.NewFlagSet("conformance", flag.ExitOnError)
	root := flags.String("root", ".", "repository root containing immutable inputs")
	fixturesDir := flags.String("fixtures", "fixtures/cases", "canonical fixture directory")
	outputDir := flags.String("output-dir", "", "absolute caller-owned output directory")
	options := runtimeOptions(flags)
	_ = flags.Parse(args)
	if *outputDir == "" {
		fatal("conformance requires -output-dir")
	}
	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		fatal("conformance root: %v", err)
	}
	if err := ensureCallerOutput(absoluteRoot, *outputDir); err != nil {
		fatal("conformance output: %v", err)
	}
	meta, err := guard.LoadMeta(absoluteRoot)
	if err != nil {
		fatal("conformance metadata: %v", err)
	}
	paths := []string{}
	walkErr := filepath.WalkDir(*fixturesDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			paths = append(paths, path)
		}
		return nil
	})
	if walkErr != nil {
		fatal("conformance fixtures: %v", walkErr)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		fatal("conformance fixtures: no JSON cases")
	}
	index := guard.ConformanceIndex{Schema: "gooo/semantic-drift-guard/conformance-index/v1", Cases: []guard.ConformanceCase{}}
	seenKinds := map[string]bool{}
	for _, path := range paths {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			fatal("fixture %s: %v", path, readErr)
		}
		var fixture guard.Comparison
		if err := decodeStrict(raw, &fixture); err != nil {
			fatal("fixture %s: %v", path, err)
		}
		if fixture.ExpectedDecision == "" {
			fatal("fixture %s: expected_decision is required", path)
		}
		report := guard.Compare(raw, meta, *options)
		rel, _ := filepath.Rel(*fixturesDir, path)
		caseName := strings.TrimSuffix(filepath.ToSlash(rel), ".json")
		caseName = strings.NewReplacer("/", "__", "\\", "__").Replace(caseName)
		caseDir := filepath.Join(*outputDir, caseName)
		if err := guard.WriteOutputs(caseDir, report); err != nil {
			fatal("fixture %s output: %v", path, err)
		}
		if report.Decision != fixture.ExpectedDecision {
			fatal("fixture %s: expected %s, got %s", path, fixture.ExpectedDecision, report.Decision)
		}
		if len(report.CellDecisions) != 12 {
			fatal("fixture %s: expected exact 12 cell decisions", path)
		}
		kind := filepath.ToSlash(rel)
		if index := strings.IndexByte(kind, '/'); index >= 0 {
			seenKinds[kind[:index]] = true
		}
		index.Cases = append(index.Cases, guard.ConformanceCase{CaseID: report.CaseID, Decision: report.Decision, Expected: fixture.ExpectedDecision, Report: filepath.ToSlash(filepath.Join(caseName, "comparison-report.json"))})
	}
	for _, required := range []string{"normal", "unknown", "refuted"} {
		if !seenKinds[required] {
			fatal("conformance fixtures: missing %s canonical fixture directory", required)
		}
	}
	if err := writeJSON(filepath.Join(*outputDir, "conformance-index.json"), index); err != nil {
		fatal("conformance index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(*outputDir, "ci-summary.md"), []byte(conformanceSummary(index)), 0o644); err != nil {
		fatal("conformance summary: %v", err)
	}
}

func runtimeOptions(flags *flag.FlagSet) *guard.RuntimeOptions {
	options := &guard.RuntimeOptions{}
	flags.IntVar(&options.BuildMS, "build-ms", 0, "observed CI build time in milliseconds")
	flags.IntVar(&options.TestMS, "test-ms", 0, "observed CI test time in milliseconds")
	flags.IntVar(&options.TestMetrics.Total, "tests-total", 0, "observed CI test total")
	flags.IntVar(&options.TestMetrics.Executed, "tests-executed", 0, "observed CI tests executed")
	flags.IntVar(&options.TestMetrics.Reused, "tests-reused", 0, "observed CI tests reused")
	flags.IntVar(&options.TestMetrics.Skipped, "tests-skipped", 0, "observed CI tests skipped")
	flags.IntVar(&options.TestMetrics.NotObserved, "tests-not-observed", 0, "tests not observed")
	return options
}

func ensureCallerOutput(root, output string) error {
	if !filepath.IsAbs(output) {
		return errors.New("output must be absolute")
	}
	absoluteOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, absoluteOutput)
	if err != nil {
		return err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return errors.New("output must be outside the immutable repository root")
	}
	return nil
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func conformanceSummary(index guard.ConformanceIndex) string {
	var builder strings.Builder
	builder.WriteString("# Gooo semantic drift guard conformance\n\n")
	fmt.Fprintf(&builder, "cases: %d\n\n| case_id | expected | decision | report |\n|---|---|---|---|\n", len(index.Cases))
	for _, item := range index.Cases {
		fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | `%s` |\n", item.CaseID, item.Expected, item.Decision, item.Report)
	}
	builder.WriteString("\nThe table contains exact decisions and integer observations.\n")
	return builder.String()
}

func fatal(format string, values ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
