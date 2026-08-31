package guard

import (
	"os"
	"testing"
)

func fixtureSource(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../fixtures/bundles/" + name + "/source.gooo")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func fixtureGraph(t *testing.T, name string) SemanticGraph {
	t.Helper()
	graph, err := ParseSource(fixtureSource(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func TestParseSourceProducesTwelveNodes(t *testing.T) {
	graph := fixtureGraph(t, "base")
	if len(graph.Nodes) != 12 || len(graph.Relations) != 12 {
		t.Fatalf("got %d nodes and %d relations", len(graph.Nodes), len(graph.Relations))
	}
}

func TestCanonicalGraphSortsNodes(t *testing.T) {
	graph := fixtureGraph(t, "base")
	if graph.Nodes[0].ID > graph.Nodes[len(graph.Nodes)-1].ID {
		t.Fatal("canonical graph is not sorted")
	}
}

func TestCanonicalGraphIgnoresCommentFormattingAndOrder(t *testing.T) {
	base := fixtureGraph(t, "base")
	equivalent := fixtureGraph(t, "equivalent-candidate")
	if !stringMustEqual(base, equivalent) {
		t.Fatal("formatting/comment/order changed the canonical graph")
	}
}

func TestCanonicalGraphDigestIsStable(t *testing.T) {
	base := fixtureGraph(t, "base")
	equivalent := fixtureGraph(t, "equivalent-candidate")
	left, err := graphDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	right, err := graphDigest(equivalent)
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("digests differ: %s %s", left, right)
	}
}

func TestContractHasFixedBalances(t *testing.T) {
	contract, _, err := LoadContract("../../contracts/semantic-drift-guard-denominator-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if contract.Total != 12 || len(contract.Cells) != 12 {
		t.Fatal("fixed denominator is not twelve cells")
	}
}

func TestGraphShapeIsBound(t *testing.T) {
	if err := validateGraphShape(fixtureGraph(t, "base"), 12); err != nil {
		t.Fatal(err)
	}
}

func TestGraphDiffDetectsRelation(t *testing.T) {
	before := fixtureGraph(t, "base")
	after := fixtureGraph(t, "base")
	after.Relations[0].To = "gooo.metric.changed"
	changes := graphDiff(before, after)
	found := false
	for _, change := range changes {
		if change.Kind == "relation" {
			found = true
		}
	}
	if !found {
		t.Fatal("relation change was not detected")
	}
}

func TestGraphDiffDetectsActivity(t *testing.T) {
	before := fixtureGraph(t, "base")
	after := fixtureGraph(t, "base")
	after.Nodes[0].Activity = "ChangedActivity"
	changes := graphDiff(before, after)
	if len(changes) == 0 || changes[0].Kind != "activity" {
		t.Fatal("activity change was not detected")
	}
}

func TestGraphDiffDetectsAuthority(t *testing.T) {
	before := fixtureGraph(t, "base")
	after := fixtureGraph(t, "base")
	after.Nodes[0].Authority = "WRITE_REPOSITORY"
	changes := graphDiff(before, after)
	found := false
	for _, change := range changes {
		if change.Kind == "authority" {
			found = true
		}
	}
	if !found {
		t.Fatal("authority change was not detected")
	}
}

func TestReleaseDigestBindsArtifactDigests(t *testing.T) {
	bundle := Bundle{Schema: BundleSchema, ReleaseID: "release", ReleaseTag: "v0.0.0", Source: ArtifactRef{Digest: "sha256:" + "1111111111111111111111111111111111111111111111111111111111111111"}, SemanticIR: ArtifactRef{Digest: "sha256:" + "2222222222222222222222222222222222222222222222222222222222222222"}, Generated: ArtifactRef{Digest: "sha256:" + "3333333333333333333333333333333333333333333333333333333333333333"}}
	digest, err := bundleDigest(bundle)
	if err != nil || !validDigest(digest) {
		t.Fatalf("invalid release digest: %s %v", digest, err)
	}
}

func TestUnknownRecordHasSixFields(t *testing.T) {
	unknown := Unknown{Stage: "IR", Step: "VERIFY", Reason: "STALE", UnknownClass: UnknownStale, NextOperation: "REGENERATE", BlockedBy: []string{"ir"}}
	if unknown.Stage == "" || unknown.Step == "" || unknown.Reason == "" || unknown.UnknownClass == "" || unknown.NextOperation == "" || len(unknown.BlockedBy) == 0 {
		t.Fatal("unknown record is incomplete")
	}
}

func TestGeneratedBindingIsOneToOne(t *testing.T) {
	raw, err := os.ReadFile("../../fixtures/bundles/base/semantic.gooo.go")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := parseGeneratedBinding(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(binding.NodeIDs) != 12 || len(binding.RelationIDs) != 12 {
		t.Fatal("generated binding is not one-to-one")
	}
}
