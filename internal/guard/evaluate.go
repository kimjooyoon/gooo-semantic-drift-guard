package guard

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RuntimeOptions struct {
	BuildMS     int
	TestMS      int
	TestMetrics TestMetrics
}

type bundleState struct {
	bundle  Bundle
	source  SemanticGraph
	ir      SemanticIR
	binding GeneratedBinding
	valid   bool
}

type evaluationState struct {
	meta         Meta
	report       Report
	unknowns     []Unknown
	changes      []SemanticChange
	refuted      bool
	refuteReason string
	cellStates   map[string]string
	cellReasons  map[string]string
	testMetrics  TestMetrics
	buildMS      int
	testMS       int
}

func LoadMeta(root string) (Meta, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Meta{}, err
	}
	paths := struct{ source, ir, generated, evaluator, contract string }{
		source:    filepath.Join(absoluteRoot, "examples/semantic-drift-guard/main.gooo"),
		ir:        filepath.Join(absoluteRoot, "internal/generated/semantic-ir.json"),
		generated: filepath.Join(absoluteRoot, "internal/generated/semantic.gooo.go"),
		evaluator: filepath.Join(absoluteRoot, "internal/guard/evaluate.go"),
		contract:  filepath.Join(absoluteRoot, "contracts/semantic-drift-guard-denominator-v1.json"),
	}
	sourceRaw, err := os.ReadFile(paths.source)
	if err != nil {
		return Meta{}, err
	}
	irRaw, err := os.ReadFile(paths.ir)
	if err != nil {
		return Meta{}, err
	}
	generatedRaw, err := os.ReadFile(paths.generated)
	if err != nil {
		return Meta{}, err
	}
	evaluatorRaw, err := os.ReadFile(paths.evaluator)
	if err != nil {
		return Meta{}, err
	}
	contract, contractRaw, err := LoadContract(paths.contract)
	if err != nil {
		return Meta{}, err
	}
	var ir SemanticIR
	if err := decodeStrict(irRaw, &ir); err != nil {
		return Meta{}, err
	}
	graph, err := ParseSource(sourceRaw)
	if err != nil {
		return Meta{}, err
	}
	if err := validateIR(ir, graph, "examples/semantic-drift-guard/main.gooo", "contracts/semantic-drift-guard-denominator-v1.json", contract, DigestBytes(contractRaw), true); err != nil {
		return Meta{}, err
	}
	binding, err := parseGeneratedBinding(generatedRaw)
	if err != nil {
		return Meta{}, err
	}
	meta := Meta{
		Root:       absoluteRoot,
		SourcePath: "examples/semantic-drift-guard/main.gooo", SourceDigest: DigestBytes(sourceRaw),
		SemanticIRPath: "internal/generated/semantic-ir.json", SemanticIRDigest: DigestBytes(irRaw),
		GeneratedGoPath: "internal/generated/semantic.gooo.go", GeneratedGoDigest: DigestBytes(generatedRaw),
		EvaluatorPath: "internal/guard/evaluate.go", EvaluatorDigest: DigestBytes(evaluatorRaw),
		ContractPath: "contracts/semantic-drift-guard-denominator-v1.json", ContractDigest: DigestBytes(contractRaw), Contract: contract,
	}
	meta.AuthorityChain = AuthorityChain{
		Source:      ArtifactRef{Path: meta.SourcePath, Digest: meta.SourceDigest},
		SemanticIR:  ArtifactRef{Path: meta.SemanticIRPath, Digest: meta.SemanticIRDigest},
		GeneratedGo: ArtifactRef{Path: meta.GeneratedGoPath, Digest: meta.GeneratedGoDigest},
		Evaluator:   ArtifactRef{Path: meta.EvaluatorPath, Digest: meta.EvaluatorDigest},
		Contract:    ArtifactRef{Path: meta.ContractPath, Digest: meta.ContractDigest},
	}
	if err := validateGeneratedBinding(binding, meta.SourcePath, ir, meta.SemanticIRPath, meta.SemanticIRDigest); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

func Compare(raw []byte, meta Meta, options RuntimeOptions) Report {
	started := time.Now()
	state := &evaluationState{
		meta:     meta,
		unknowns: []Unknown{}, changes: []SemanticChange{},
		cellStates: map[string]string{}, cellReasons: map[string]string{},
	}
	for _, cell := range meta.Contract.Cells {
		state.cellStates[cell.ID] = DecisionClosed
		state.cellReasons[cell.ID] = "NO_SEMANTIC_CHANGE"
	}
	state.report = Report{
		Schema: ReportSchema, Precedence: append([]string(nil), Precedence...),
		Changes: []SemanticChange{}, Unknowns: []Unknown{},
		Replay:         ReplayMetrics{Unknowns: []Unknown{}},
		Authority:      AuthorityReport{RepositoryWrites: 0, LocalTestExecutions: 0, CrossProjectRequiredGates: 0, ReadOnly: true},
		AuthorityChain: meta.AuthorityChain,
		Inventory:      inventory(meta.Root),
	}
	var input Comparison
	if err := decodeStrict(raw, &input); err != nil {
		state.refute("INPUT", "MALFORMED_INPUT", "comparison input is not valid protocol JSON", nil)
		return finish(state, started, options)
	}
	state.report.CaseID = input.CaseID
	if input.Schema != InputSchema || input.CaseID == "" {
		state.refute("INPUT", "INVALID_INPUT_CONTRACT", "comparison schema or case identifier is invalid", nil)
	}
	state.report.Authority = authorityReport(input.Authority)
	state.testMetrics = input.TestMetrics
	if state.testMetrics == (TestMetrics{}) {
		state.testMetrics.NotObserved = 1
	}
	if options.TestMetrics != (TestMetrics{}) {
		state.testMetrics = options.TestMetrics
	}
	state.buildMS = options.BuildMS
	state.testMS = options.TestMS
	if input.Authority.RepositoryWrites != 0 || input.Authority.LocalTestExecutions != 0 || input.Authority.CrossProjectRequiredGates != 0 {
		state.refute("AUTHORITY", "AUTHORITY_ESCALATION", "read-only authority fields must all be zero", nil)
		state.setCellByActivity("DetectAuthorityDrift", DecisionRefuted, "AUTHORITY_ESCALATION")
	}
	base := inspectBundle(meta, input.Base, "base", state)
	candidate := inspectBundle(meta, input.Candidate, "candidate", state)
	state.report.Metrics.ReleasesCompared = countNonEmptyBundles(input.Base, input.Candidate)
	if base.valid {
		state.report.Metrics.SourceFiles++
		state.report.Metrics.IRNodes += len(base.ir.Graph.Nodes)
		state.report.Metrics.GeneratedFiles++
		state.report.Metrics.SemanticRelationsBefore = len(base.ir.Graph.Relations)
	}
	if candidate.valid {
		state.report.Metrics.SourceFiles++
		state.report.Metrics.IRNodes += len(candidate.ir.Graph.Nodes)
		state.report.Metrics.GeneratedFiles++
		state.report.Metrics.SemanticRelationsAfter = len(candidate.ir.Graph.Relations)
	}
	if base.valid && candidate.valid {
		state.changes = graphDiff(base.ir.Graph, candidate.ir.Graph)
		state.report.Changes = append([]SemanticChange{}, state.changes...)
		if len(state.changes) > 0 {
			state.refute("GRAPH", "SEMANTIC_GRAPH_DRIFT", "canonical semantic graph changed", nil)
			state.report.Metrics.SemanticDriftChanges = 1
			for _, change := range state.changes {
				state.setCellByActivity(activityForChange(change.Kind), DecisionRefuted, "SEMANTIC_GRAPH_DRIFT")
			}
		} else if bundleBytesChanged(base.bundle, candidate.bundle) {
			state.report.Metrics.EquivalentChanges = 1
			for _, activity := range []string{"ClassifyFormattingEquivalence", "ClassifyCommentEquivalence", "ClassifyOrderEquivalence"} {
				state.setCellByActivity(activity, DecisionClosed, "CANONICAL_GRAPH_EQUAL")
			}
		}
	}
	state.report.Replay = evaluateReplay(input, base, candidate, state)
	state.report.Metrics.ReplayComparisons = state.report.Replay.Comparisons
	state.report.Metrics.ReplayMismatches = state.report.Replay.Mismatches
	state.report.Unknowns = append([]Unknown(nil), state.unknowns...)
	state.report.Changes = append([]SemanticChange{}, state.changes...)
	return finish(state, started, options)
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func authorityReport(input AuthorityInput) AuthorityReport {
	return AuthorityReport{
		RepositoryWrites: 0, LocalTestExecutions: 0, CrossProjectRequiredGates: 0,
		ReadOnly: input.RepositoryWrites == 0 && input.LocalTestExecutions == 0 && input.CrossProjectRequiredGates == 0,
	}
}

func countNonEmptyBundles(base, candidate Bundle) int {
	count := 0
	if base.ReleaseID != "" || base.ReleaseTag != "" || base.Source.Path != "" {
		count++
	}
	if candidate.ReleaseID != "" || candidate.ReleaseTag != "" || candidate.Source.Path != "" {
		count++
	}
	return count
}

func inspectBundle(meta Meta, bundle Bundle, label string, state *evaluationState) bundleState {
	result := bundleState{bundle: bundle}
	if bundle.Schema != BundleSchema || bundle.ReleaseID == "" || bundle.ReleaseTag == "" || !validDigest(bundle.ReleaseDigest) || !validDigest(bundle.ObservedReleaseDigest) {
		state.addUnknown(Unknown{Stage: "BUNDLE", Step: "READ_IMMUTABLE_BUNDLE", Reason: "BUNDLE_BINDING_MISSING", UnknownClass: UnknownDirect, NextOperation: "PROVIDE_IMMUTABLE_RELEASE_BUNDLE", BlockedBy: []string{label + ".bundle"}})
		return result
	}
	computedRelease, err := bundleDigest(bundle)
	if err != nil || computedRelease != bundle.ReleaseDigest || bundle.ObservedReleaseDigest != bundle.ReleaseDigest {
		state.addUnknown(Unknown{Stage: "BUNDLE", Step: "VERIFY_RELEASE_DIGEST", Reason: "STALE_RELEASE_DIGEST", UnknownClass: UnknownStale, NextOperation: "PIN_OBSERVED_RELEASE_DIGEST", BlockedBy: []string{label + ".release_digest"}})
		return result
	}
	sourceRaw, sourceErr := readArtifact(meta.Root, bundle.Source)
	if sourceErr != nil {
		state.addArtifactUnknown(label, "source", sourceErr)
		return result
	}
	irRaw, irErr := readArtifact(meta.Root, bundle.SemanticIR)
	if irErr != nil {
		state.addArtifactUnknown(label, "semantic_ir", irErr)
		return result
	}
	generatedRaw, generatedErr := readArtifact(meta.Root, bundle.Generated)
	if generatedErr != nil {
		state.addArtifactUnknown(label, "generated", generatedErr)
		return result
	}
	sourceGraph, sourceParseErr := ParseSource(sourceRaw)
	if sourceParseErr != nil {
		state.addUnknown(Unknown{Stage: "SOURCE", Step: "PARSE_CANONICAL_GRAPH", Reason: "AMBIGUOUS_SOURCE_GRAPH", UnknownClass: UnknownAmbiguous, NextOperation: "PROVIDE_UNAMBIGUOUS_GOOO_SOURCE", BlockedBy: []string{label + ".source"}})
		return result
	}
	var ir SemanticIR
	if err := decodeStrict(irRaw, &ir); err != nil {
		state.addUnknown(Unknown{Stage: "IR", Step: "DECODE_SEMANTIC_IR", Reason: "AMBIGUOUS_SEMANTIC_IR", UnknownClass: UnknownAmbiguous, NextOperation: "REGENERATE_CANONICAL_SEMANTIC_IR", BlockedBy: []string{label + ".semantic_ir"}})
		return result
	}
	if err := validateIR(ir, sourceGraph, bundle.Source.Path, "contracts/semantic-drift-guard-denominator-v1.json", meta.Contract, meta.ContractDigest, false); err != nil {
		state.addUnknown(Unknown{Stage: "IR", Step: "VERIFY_SOURCE_IR_BINDING", Reason: "STALE_OR_AMBIGUOUS_SOURCE_IR_BINDING", UnknownClass: classifyBindingError(err), NextOperation: "REGENERATE_AND_REPIN_SOURCE_IR_BINDING", BlockedBy: []string{label + ".source", label + ".semantic_ir"}})
		return result
	}
	binding, err := parseGeneratedBinding(generatedRaw)
	if err != nil {
		state.addUnknown(Unknown{Stage: "GENERATED", Step: "READ_GENERATED_BINDING", Reason: "GENERATED_BINDING_MISSING", UnknownClass: UnknownDirect, NextOperation: "PROVIDE_GENERATED_BINDING_METADATA", BlockedBy: []string{label + ".generated"}})
		return result
	}
	if err := validateGeneratedBinding(binding, bundle.Source.Path, ir, bundle.SemanticIR.Path, bundle.SemanticIR.Digest); err != nil {
		state.addUnknown(Unknown{Stage: "GENERATED", Step: "VERIFY_IR_GENERATED_BINDING", Reason: "STALE_OR_AMBIGUOUS_IR_GENERATED_BINDING", UnknownClass: classifyBindingError(err), NextOperation: "REGENERATE_AND_REPIN_GENERATED_BINDING", BlockedBy: []string{label + ".semantic_ir", label + ".generated"}})
		return result
	}
	result.source, result.ir, result.binding, result.valid = sourceGraph, ir, binding, true
	return result
}

func (state *evaluationState) addArtifactUnknown(label, artifact string, err error) {
	class := UnknownDirect
	reason := "ARTIFACT_MISSING"
	if strings.Contains(err.Error(), "stale") {
		class = UnknownStale
		reason = "STALE_ARTIFACT_DIGEST"
	}
	state.addUnknown(Unknown{Stage: "BINDING", Step: "READ_" + strings.ToUpper(artifact), Reason: reason, UnknownClass: class, NextOperation: "PROVIDE_CURRENT_IMMUTABLE_ARTIFACT", BlockedBy: []string{label + "." + artifact}})
}

func classifyBindingError(err error) string {
	if strings.Contains(err.Error(), "ambiguous") || strings.Contains(err.Error(), "duplicate") {
		return UnknownAmbiguous
	}
	return UnknownStale
}

func validateIR(ir SemanticIR, sourceGraph SemanticGraph, sourcePath, contractPath string, contract Contract, contractDigest string, requireContract bool) error {
	if ir.Schema != IRSchema || ir.SourcePath != sourcePath || ir.ContractPath != contractPath || ir.ContractDigest != contractDigest || ir.SourceSemanticDigest == "" || !validDigest(ir.SourceSemanticDigest) || !validDigest(ir.GraphDigest) {
		return errors.New("stale semantic IR binding")
	}
	parsedDigest, err := graphDigest(sourceGraph)
	if err != nil || parsedDigest != ir.SourceSemanticDigest {
		return errors.New("stale source semantic digest")
	}
	actualGraphDigest, err := graphDigest(ir.Graph)
	if err != nil || actualGraphDigest != ir.GraphDigest || ir.GraphDigest != parsedDigest {
		return errors.New("stale IR graph digest")
	}
	if stringMustEqual(ir.Graph, sourceGraph) != true {
		return errors.New("ambiguous source IR graph")
	}
	if requireContract {
		return validateGraphAgainstContract(ir.Graph, contract)
	}
	return validateGraphShape(ir.Graph, 12)
}

func validateGraphShape(graph SemanticGraph, total int) error {
	if graph.Schema != graphSchema || len(graph.Nodes) != total || len(graph.Relations) != total {
		return errors.New("graph cardinality is not bound")
	}
	nodes := map[string]ActivityNode{}
	for _, node := range graph.Nodes {
		if node.ID == "" || node.Activity == "" || node.MetricID == "" || node.Authority == "" {
			return errors.New("graph node is incomplete")
		}
		if _, exists := nodes[node.ID]; exists {
			return errors.New("ambiguous graph node")
		}
		nodes[node.ID] = node
	}
	relations := map[string]SemanticRelation{}
	for _, relation := range graph.Relations {
		if relation.ID == "" || relation.Kind == "" {
			return errors.New("graph relation is incomplete")
		}
		if _, exists := relations[relation.ID]; exists {
			return errors.New("ambiguous graph relation")
		}
		relations[relation.ID] = relation
	}
	for _, node := range graph.Nodes {
		relation, ok := relations[relationID(node.ID, node.MetricID)]
		if !ok || relation.From != node.ID || relation.To != node.MetricID || relation.Kind != "MEASURES" {
			return errors.New("graph relation does not bind node")
		}
	}
	return nil
}

func stringMustEqual(left, right SemanticGraph) bool {
	leftRaw, leftErr := json.Marshal(canonicalGraph(left))
	rightRaw, rightErr := json.Marshal(canonicalGraph(right))
	return leftErr == nil && rightErr == nil && bytes.Equal(leftRaw, rightRaw)
}

func parseGeneratedBinding(raw []byte) (GeneratedBinding, error) {
	const marker = "// semantic-binding:"
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), marker) {
			value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), marker))
			var binding GeneratedBinding
			if err := decodeStrict([]byte(value), &binding); err != nil {
				return GeneratedBinding{}, err
			}
			return binding, nil
		}
	}
	return GeneratedBinding{}, errors.New("generated binding marker missing")
}

func validateGeneratedBinding(binding GeneratedBinding, sourcePath string, ir SemanticIR, irPath, irDigest string) error {
	if binding.Schema != GeneratedSchema || binding.SourcePath != sourcePath || binding.SourceSemanticDigest != ir.SourceSemanticDigest || binding.IRPath != irPath || binding.IRDigest != irDigest || binding.GraphDigest != ir.GraphDigest {
		return errors.New("stale generated binding")
	}
	nodeIDs := make([]string, 0, len(ir.Graph.Nodes))
	for _, node := range canonicalGraph(ir.Graph).Nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	relationIDs := make([]string, 0, len(ir.Graph.Relations))
	for _, relation := range canonicalGraph(ir.Graph).Relations {
		relationIDs = append(relationIDs, relation.ID)
	}
	if !equalStrings(binding.NodeIDs, nodeIDs) || !equalStrings(binding.RelationIDs, relationIDs) {
		return errors.New("ambiguous generated binding")
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func graphDiff(before, after SemanticGraph) []SemanticChange {
	changes := []SemanticChange{}
	beforeNodes, afterNodes := map[string]ActivityNode{}, map[string]ActivityNode{}
	for _, node := range before.Nodes {
		beforeNodes[node.ID] = node
	}
	for _, node := range after.Nodes {
		afterNodes[node.ID] = node
	}
	ids := map[string]bool{}
	for id := range beforeNodes {
		ids[id] = true
	}
	for id := range afterNodes {
		ids[id] = true
	}
	sortedIDs := make([]string, 0, len(ids))
	for id := range ids {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)
	for _, id := range sortedIDs {
		oldNode, oldOK := beforeNodes[id]
		newNode, newOK := afterNodes[id]
		if !oldOK || !newOK {
			changes = append(changes, SemanticChange{Kind: "activity", ID: id, Before: nodeText(oldNode, oldOK), After: nodeText(newNode, newOK)})
			continue
		}
		if oldNode.Activity != newNode.Activity {
			changes = append(changes, SemanticChange{Kind: "activity", ID: id, Before: oldNode.Activity, After: newNode.Activity})
		}
		if oldNode.Authority != newNode.Authority {
			changes = append(changes, SemanticChange{Kind: "authority", ID: id, Before: oldNode.Authority, After: newNode.Authority})
		}
		if oldNode.ProofChoice != newNode.ProofChoice || oldNode.IndicatorClass != newNode.IndicatorClass || oldNode.MetricID != newNode.MetricID || oldNode.Artifact != newNode.Artifact {
			changes = append(changes, SemanticChange{Kind: "activity", ID: id, Before: nodeSemanticText(oldNode), After: nodeSemanticText(newNode)})
		}
	}
	beforeRelations, afterRelations := map[string]SemanticRelation{}, map[string]SemanticRelation{}
	for _, relation := range before.Relations {
		beforeRelations[relation.ID] = relation
	}
	for _, relation := range after.Relations {
		afterRelations[relation.ID] = relation
	}
	relationIDs := map[string]bool{}
	for id := range beforeRelations {
		relationIDs[id] = true
	}
	for id := range afterRelations {
		relationIDs[id] = true
	}
	sortedRelations := make([]string, 0, len(relationIDs))
	for id := range relationIDs {
		sortedRelations = append(sortedRelations, id)
	}
	sort.Strings(sortedRelations)
	for _, id := range sortedRelations {
		oldRelation, oldOK := beforeRelations[id]
		newRelation, newOK := afterRelations[id]
		if !oldOK || !newOK || oldRelation.From != newRelation.From || oldRelation.To != newRelation.To || oldRelation.Kind != newRelation.Kind {
			changes = append(changes, SemanticChange{Kind: "relation", ID: id, Before: relationText(oldRelation, oldOK), After: relationText(newRelation, newOK)})
		}
	}
	return changes
}

func nodeText(node ActivityNode, ok bool) string {
	if !ok {
		return "<missing>"
	}
	return nodeSemanticText(node)
}

func nodeSemanticText(node ActivityNode) string {
	return strings.Join([]string{node.ID, node.Activity, node.ProofChoice, node.IndicatorClass, node.MetricID, node.Artifact, node.Authority}, "|")
}

func relationText(relation SemanticRelation, ok bool) string {
	if !ok {
		return "<missing>"
	}
	return strings.Join([]string{relation.From, relation.To, relation.Kind}, "|")
}

func bundleBytesChanged(before, after Bundle) bool {
	return before.Source.Digest != after.Source.Digest || before.SemanticIR.Digest != after.SemanticIR.Digest || before.Generated.Digest != after.Generated.Digest || before.ReleaseDigest != after.ReleaseDigest
}

func evaluateReplay(input Comparison, base, candidate bundleState, state *evaluationState) ReplayMetrics {
	result := ReplayMetrics{Unknowns: []Unknown{}}
	if len(input.Replay) == 0 {
		unknown := Unknown{Stage: "REPLAY", Step: "REQUIRE_REPLAY_COMPARISON", Reason: "REPLAY_BINDING_MISSING", UnknownClass: UnknownDirect, NextOperation: "PROVIDE_BOUND_REPLAY_OBSERVATIONS", BlockedBy: []string{"replay"}}
		state.addUnknown(unknown)
		result.Unknowns = append(result.Unknowns, unknown)
		return result
	}
	for _, observation := range input.Replay {
		result.Comparisons++
		if observation.InputID == "" || observation.Status != "REPLAYED" || !validDigest(observation.BaseValueDigest) || !validDigest(observation.CandidateValueDigest) || !validDigest(observation.BaseReleaseDigest) || !validDigest(observation.CandidateReleaseDigest) || observation.BaseReleaseDigest != input.Base.ReleaseDigest || observation.CandidateReleaseDigest != input.Candidate.ReleaseDigest || !base.valid || !candidate.valid {
			unknown := Unknown{Stage: "REPLAY", Step: "VERIFY_REPLAY_BINDING", Reason: "REPLAY_BINDING_STALE_OR_AMBIGUOUS", UnknownClass: UnknownStale, NextOperation: "REPLAY_WITH_PINNED_RELEASE_DIGESTS", BlockedBy: []string{"replay." + observation.InputID}}
			state.addUnknown(unknown)
			result.Unknowns = append(result.Unknowns, unknown)
			continue
		}
		if observation.BaseValueDigest != observation.CandidateValueDigest {
			result.Mismatches++
			state.refute("REPLAY", "REPLAY_MISMATCH", "observable replay value digest changed", []string{observation.InputID})
			state.setCellByActivity("BindReplayObservation", DecisionRefuted, "REPLAY_MISMATCH")
		}
	}
	return result
}

func activityForChange(kind string) string {
	switch kind {
	case "relation":
		return "DetectRelationDrift"
	case "authority":
		return "DetectAuthorityDrift"
	default:
		return "DetectActivityDrift"
	}
}

func (state *evaluationState) addUnknown(value Unknown) {
	if value.Stage == "" || value.Step == "" || value.Reason == "" || value.UnknownClass == "" || value.NextOperation == "" || len(value.BlockedBy) == 0 {
		value = Unknown{Stage: "BINDING", Step: "PRESERVE_UNKNOWN", Reason: "INCOMPLETE_UNKNOWN_RECORD", UnknownClass: UnknownAmbiguous, NextOperation: "REPAIR_UNKNOWN_RECORD", BlockedBy: []string{"unknown-record"}}
	}
	state.unknowns = append(state.unknowns, value)
	if value.Stage != "REPLAY" {
		state.report.Metrics.UnknownBindings++
	}
	state.report.Unknowns = append(state.report.Unknowns, value)
	for _, blocked := range value.BlockedBy {
		if strings.Contains(blocked, "source") {
			state.setCellByActivity("BindSource", DecisionUnknown, value.Reason)
		}
		if strings.Contains(blocked, "semantic_ir") {
			state.setCellByActivity("BindSemanticIR", DecisionUnknown, value.Reason)
		}
		if strings.Contains(blocked, "generated") {
			state.setCellByActivity("BindGeneratedGo", DecisionUnknown, value.Reason)
		}
	}
}

func (state *evaluationState) refute(stage, reason, _ string, blocked []string) {
	if !state.refuted {
		state.refuteReason = reason
	}
	state.refuted = true
	if len(blocked) > 0 {
		state.changes = append(state.changes, SemanticChange{Kind: strings.ToLower(stage), ID: reason, Before: strings.Join(blocked, ","), After: "REFUTED"})
	}
}

func (state *evaluationState) setCellByActivity(activity, decision, reason string) {
	for _, cell := range state.meta.Contract.Cells {
		if cell.Activity == activity {
			state.cellStates[cell.ID] = decision
			state.cellReasons[cell.ID] = reason
		}
	}
}

func finish(state *evaluationState, started time.Time, options RuntimeOptions) Report {
	decision := DecisionClosed
	reason := "CANONICAL_GRAPH_EQUAL_AND_REPLAY_EQUAL"
	if state.refuted {
		decision, reason = DecisionRefuted, state.refuteReason
	} else if len(state.unknowns) > 0 {
		decision, reason = DecisionUnknown, state.unknowns[0].Reason
	}
	state.report.Decision, state.report.Reason = decision, reason
	state.report.Unknowns = append([]Unknown(nil), state.unknowns...)
	state.report.Changes = append([]SemanticChange{}, state.changes...)
	state.report.Metrics.WallMS = int(time.Since(started).Milliseconds())
	if state.report.Metrics.WallMS < 1 {
		state.report.Metrics.WallMS = 1
	}
	state.report.Metrics.PeakRSSKiB = peakRSSKiB()
	state.report.Metrics.BuildMS = state.buildMS
	state.report.Metrics.TestMS = state.testMS
	state.report.TestMetrics = state.testMetrics
	state.report.Metrics.TestsTotal = state.testMetrics.Total
	state.report.Metrics.TestsExecuted = state.testMetrics.Executed
	state.report.Metrics.TestsReused = state.testMetrics.Reused
	state.report.Metrics.TestsSkipped = state.testMetrics.Skipped
	state.report.Metrics.TestsNotObserved = state.testMetrics.NotObserved
	state.report.Metrics.GoPhysicalLines = state.report.Inventory.GoPhysicalLines
	state.report.Metrics.GoFiles = state.report.Inventory.GoFiles
	state.report.Metrics.GoooPhysicalLines = state.report.Inventory.GoooPhysicalLines
	state.report.Metrics.GoooFiles = state.report.Inventory.GoooFiles
	state.report.Metrics.DescendantDirs = state.report.Inventory.DescendantDirs
	state.report.Metrics.DescendantFiles = state.report.Inventory.DescendantFiles
	state.report.CellDecisions = make([]CellDecision, 0, len(state.meta.Contract.Cells))
	for _, cell := range state.meta.Contract.Cells {
		state.report.CellDecisions = append(state.report.CellDecisions, CellDecision{Ordinal: cell.Ordinal, ID: cell.ID, Activity: cell.Activity, Decision: state.cellStates[cell.ID], Reason: state.cellReasons[cell.ID]})
	}
	return state.report
}
