package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func DigestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func DigestJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return DigestBytes(raw), nil
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func canonicalGraph(graph SemanticGraph) SemanticGraph {
	copyGraph := SemanticGraph{Schema: graph.Schema}
	copyGraph.Nodes = append([]ActivityNode(nil), graph.Nodes...)
	copyGraph.Relations = append([]SemanticRelation(nil), graph.Relations...)
	sort.Slice(copyGraph.Nodes, func(i, j int) bool { return copyGraph.Nodes[i].ID < copyGraph.Nodes[j].ID })
	sort.Slice(copyGraph.Relations, func(i, j int) bool {
		if copyGraph.Relations[i].ID != copyGraph.Relations[j].ID {
			return copyGraph.Relations[i].ID < copyGraph.Relations[j].ID
		}
		if copyGraph.Relations[i].From != copyGraph.Relations[j].From {
			return copyGraph.Relations[i].From < copyGraph.Relations[j].From
		}
		return copyGraph.Relations[i].To < copyGraph.Relations[j].To
	})
	return copyGraph
}

func graphDigest(graph SemanticGraph) (string, error) {
	canonical := canonicalGraph(graph)
	return DigestJSON(canonical)
}

type releaseDigestPayload struct {
	Schema     string `json:"schema"`
	ReleaseID  string `json:"release_id"`
	ReleaseTag string `json:"release_tag"`
	Source     string `json:"source_digest"`
	SemanticIR string `json:"semantic_ir_digest"`
	Generated  string `json:"generated_digest"`
}

func bundleDigest(bundle Bundle) (string, error) {
	return DigestJSON(releaseDigestPayload{
		Schema: bundle.Schema, ReleaseID: bundle.ReleaseID, ReleaseTag: bundle.ReleaseTag,
		Source: bundle.Source.Digest, SemanticIR: bundle.SemanticIR.Digest, Generated: bundle.Generated.Digest,
	})
}

func readArtifact(root string, ref ArtifactRef) ([]byte, error) {
	if ref.Path == "" || !validDigest(ref.Digest) {
		return nil, errors.New("missing artifact path or digest")
	}
	path := ref.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.Clean(path))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if DigestBytes(raw) != ref.Digest {
		return nil, errors.New("artifact digest is stale")
	}
	return raw, nil
}

func inventory(root string) Inventory {
	result := Inventory{RootREADMEExcluded: true, Violations: []string{}}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			result.Violations = append(result.Violations, path+":unreadable")
			return nil
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			result.DescendantDirs++
			return nil
		}
		if entry.Name() == "README.md" && filepath.Dir(path) == root {
			return nil
		}
		result.DescendantFiles++
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".go" && ext != ".gooo" {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			result.Violations = append(result.Violations, path+":unreadable")
			return nil
		}
		lines := physicalLines(raw)
		if ext == ".go" {
			result.GoFiles++
			result.GoPhysicalLines += lines
		} else {
			result.GoooFiles++
			result.GoooPhysicalLines += lines
		}
		return nil
	})
	return result
}

func physicalLines(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	count := strings.Count(string(raw), "\n")
	if raw[len(raw)-1] != '\n' {
		count++
	}
	return count
}

func peakRSSKiB() int {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	value := int(stats.Sys / 1024)
	if value < 1 {
		return 1
	}
	return value
}
