package guard

import "encoding/json"

const (
	InputSchema      = "gooo/semantic-drift-guard/comparison/v1"
	BundleSchema     = "gooo/semantic-drift-guard/bundle/v1"
	IRSchema         = "gooo/semantic-drift-guard/semantic-ir/v1"
	GeneratedSchema  = "gooo/semantic-drift-guard/generated-binding/v1"
	ReportSchema     = "gooo/semantic-drift-guard/report/v1"
	DecisionClosed   = "CLOSED"
	DecisionUnknown  = "UNKNOWN"
	DecisionRefuted  = "REFUTED"
	UnknownDirect    = "DIRECT_MISSING"
	UnknownStale     = "STALE"
	UnknownAmbiguous = "AMBIGUOUS"
)

var Precedence = []string{DecisionRefuted, DecisionUnknown, DecisionClosed}

type Comparison struct {
	Schema           string              `json:"schema"`
	CaseID           string              `json:"case_id"`
	ExpectedDecision string              `json:"expected_decision,omitempty"`
	Base             Bundle              `json:"base"`
	Candidate        Bundle              `json:"candidate"`
	Replay           []ReplayObservation `json:"replay"`
	Authority        AuthorityInput      `json:"authority"`
	TestMetrics      TestMetrics         `json:"test_metrics"`
}

type Bundle struct {
	Schema                string      `json:"schema"`
	ReleaseID             string      `json:"release_id"`
	ReleaseTag            string      `json:"release_tag"`
	ReleaseDigest         string      `json:"release_digest"`
	ObservedReleaseDigest string      `json:"observed_release_digest"`
	Source                ArtifactRef `json:"source"`
	SemanticIR            ArtifactRef `json:"semantic_ir"`
	Generated             ArtifactRef `json:"generated"`
}

type ArtifactRef struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type ReplayObservation struct {
	InputID                string `json:"input_id"`
	Status                 string `json:"status"`
	BaseReleaseDigest      string `json:"base_release_digest"`
	CandidateReleaseDigest string `json:"candidate_release_digest"`
	BaseValueDigest        string `json:"base_value_digest"`
	CandidateValueDigest   string `json:"candidate_value_digest"`
}

type AuthorityInput struct {
	RepositoryWrites          int `json:"repository_writes"`
	LocalTestExecutions       int `json:"local_test_executions"`
	CrossProjectRequiredGates int `json:"cross_project_required_gates"`
}

type AuthorityReport struct {
	RepositoryWrites          int  `json:"repository_writes"`
	LocalTestExecutions       int  `json:"local_test_executions"`
	CrossProjectRequiredGates int  `json:"cross_project_required_gates"`
	ReadOnly                  bool `json:"read_only"`
}

type ActivityNode struct {
	ID             string `json:"id"`
	Activity       string `json:"activity"`
	ProofChoice    string `json:"proof_choice"`
	IndicatorClass string `json:"indicator_class"`
	MetricID       string `json:"metric_id"`
	Artifact       string `json:"artifact"`
	Authority      string `json:"authority"`
}

type SemanticRelation struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type SemanticGraph struct {
	Schema    string             `json:"schema"`
	Nodes     []ActivityNode     `json:"nodes"`
	Relations []SemanticRelation `json:"relations"`
}

type SemanticIR struct {
	Schema               string        `json:"schema"`
	SourcePath           string        `json:"source_path"`
	SourceSemanticDigest string        `json:"source_semantic_digest"`
	ContractPath         string        `json:"contract_path"`
	ContractDigest       string        `json:"contract_digest"`
	GraphDigest          string        `json:"graph_digest"`
	Graph                SemanticGraph `json:"graph"`
}

type GeneratedBinding struct {
	Schema               string   `json:"schema"`
	SourcePath           string   `json:"source_path"`
	SourceSemanticDigest string   `json:"source_semantic_digest"`
	IRPath               string   `json:"ir_path"`
	IRDigest             string   `json:"ir_digest"`
	GraphDigest          string   `json:"graph_digest"`
	NodeIDs              []string `json:"node_ids"`
	RelationIDs          []string `json:"relation_ids"`
}

type Contract struct {
	Schema           string         `json:"schema"`
	DenominatorID    string         `json:"denominator_id"`
	CandidateID      string         `json:"candidate_id"`
	Total            int            `json:"total"`
	Proofs           []Balance      `json:"proofs"`
	IndicatorClasses []Balance      `json:"indicator_classes"`
	Cells            []ContractCell `json:"cells"`
}

type Balance struct {
	Choice string `json:"choice,omitempty"`
	Class  string `json:"class,omitempty"`
	Total  int    `json:"total"`
}

type ContractCell struct {
	Ordinal        int    `json:"ordinal"`
	ID             string `json:"id"`
	Activity       string `json:"activity"`
	ProofChoice    string `json:"proof_choice"`
	IndicatorClass string `json:"indicator_class"`
	MetricID       string `json:"metric_id"`
	Artifact       string `json:"artifact"`
	Evaluator      string `json:"evaluator"`
	Authority      string `json:"authority"`
}

type Unknown struct {
	Stage         string   `json:"stage"`
	Step          string   `json:"step"`
	Reason        string   `json:"reason"`
	UnknownClass  string   `json:"unknown_class"`
	NextOperation string   `json:"next_operation"`
	BlockedBy     []string `json:"blocked_by"`
}

type SemanticChange struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type CellDecision struct {
	Ordinal  int    `json:"ordinal"`
	ID       string `json:"id"`
	Activity string `json:"activity"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type ReplayMetrics struct {
	Comparisons int       `json:"comparisons"`
	Mismatches  int       `json:"mismatches"`
	Unknowns    []Unknown `json:"unknowns"`
}

type TestMetrics struct {
	Total       int `json:"total"`
	Executed    int `json:"executed"`
	Reused      int `json:"reused"`
	Skipped     int `json:"skipped"`
	NotObserved int `json:"not_observed"`
}

type Metrics struct {
	ReleasesCompared        int `json:"releases_compared"`
	SourceFiles             int `json:"source_files"`
	IRNodes                 int `json:"ir_nodes"`
	GeneratedFiles          int `json:"generated_files"`
	SemanticRelationsBefore int `json:"semantic_relations_before"`
	SemanticRelationsAfter  int `json:"semantic_relations_after"`
	EquivalentChanges       int `json:"equivalent_changes"`
	SemanticDriftChanges    int `json:"semantic_drift_changes"`
	UnknownBindings         int `json:"unknown_bindings"`
	ReplayComparisons       int `json:"replay_comparisons"`
	ReplayMismatches        int `json:"replay_mismatches"`
	PeakRSSKiB              int `json:"peak_rss_kib"`
	WallMS                  int `json:"wall_ms"`
	BuildMS                 int `json:"build_ms"`
	TestMS                  int `json:"test_ms"`
	TestsTotal              int `json:"tests_total"`
	TestsExecuted           int `json:"tests_executed"`
	TestsReused             int `json:"tests_reused"`
	TestsSkipped            int `json:"tests_skipped"`
	TestsNotObserved        int `json:"tests_not_observed"`
	GoPhysicalLines         int `json:"go_physical_lines"`
	GoFiles                 int `json:"go_files"`
	GoooPhysicalLines       int `json:"gooo_physical_lines"`
	GoooFiles               int `json:"gooo_files"`
	DescendantDirs          int `json:"descendant_dirs"`
	DescendantFiles         int `json:"descendant_files"`
}

type AuthorityChain struct {
	Source      ArtifactRef `json:"source"`
	SemanticIR  ArtifactRef `json:"semantic_ir"`
	GeneratedGo ArtifactRef `json:"generated_go"`
	Evaluator   ArtifactRef `json:"evaluator"`
	Contract    ArtifactRef `json:"contract"`
}

type Inventory struct {
	DescendantDirs     int      `json:"descendant_dirs"`
	DescendantFiles    int      `json:"descendant_files"`
	GoPhysicalLines    int      `json:"go_physical_lines"`
	GoFiles            int      `json:"go_files"`
	GoooPhysicalLines  int      `json:"gooo_physical_lines"`
	GoooFiles          int      `json:"gooo_files"`
	RootREADMEExcluded bool     `json:"root_readme_excluded"`
	Violations         []string `json:"violations"`
}

type Meta struct {
	Root              string
	SourcePath        string
	SourceDigest      string
	SemanticIRPath    string
	SemanticIRDigest  string
	GeneratedGoPath   string
	GeneratedGoDigest string
	EvaluatorPath     string
	EvaluatorDigest   string
	ContractPath      string
	ContractDigest    string
	Contract          Contract
	AuthorityChain    AuthorityChain
}

type Report struct {
	Schema         string           `json:"schema"`
	CaseID         string           `json:"case_id"`
	Decision       string           `json:"decision"`
	Reason         string           `json:"reason"`
	Precedence     []string         `json:"precedence"`
	Metrics        Metrics          `json:"metrics"`
	TestMetrics    TestMetrics      `json:"test_metrics"`
	Changes        []SemanticChange `json:"changes"`
	CellDecisions  []CellDecision   `json:"cell_decisions"`
	Unknowns       []Unknown        `json:"unknowns"`
	Replay         ReplayMetrics    `json:"replay"`
	Authority      AuthorityReport  `json:"authority"`
	AuthorityChain AuthorityChain   `json:"authority_chain"`
	Inventory      Inventory        `json:"inventory"`
}

func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
