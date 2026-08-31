package guard

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const graphSchema = "gooo/semantic-drift-guard/semantic-graph/v1"

func ParseSource(raw []byte) (SemanticGraph, error) {
	graph := SemanticGraph{Schema: graphSchema, Nodes: []ActivityNode{}, Relations: []SemanticRelation{}}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	line := 0
	graphSeen := false
	seen := map[string]bool{}
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		text = stripLineComment(text)
		if text == "" {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		values := map[string]string{}
		for _, field := range fields[1:] {
			parts := strings.SplitN(field, "=", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return SemanticGraph{}, fmt.Errorf("line %d: invalid key/value", line)
			}
			value := strings.Trim(parts[1], "\"'")
			if _, duplicate := values[parts[0]]; duplicate {
				return SemanticGraph{}, fmt.Errorf("line %d: duplicate key %s", line, parts[0])
			}
			values[parts[0]] = value
		}
		switch fields[0] {
		case "graph":
			if graphSeen || values["id"] != "gooo.semantic-drift-guard.v1" {
				return SemanticGraph{}, fmt.Errorf("line %d: invalid graph declaration", line)
			}
			graphSeen = true
		case "activity":
			node := ActivityNode{
				ID: values["id"], Activity: values["activity"], ProofChoice: values["proof"],
				IndicatorClass: values["indicator"], MetricID: values["metric"], Artifact: values["artifact"], Authority: values["authority"],
			}
			if node.ID == "" || node.Activity == "" || node.ProofChoice == "" || node.IndicatorClass == "" || node.MetricID == "" || node.Artifact == "" || node.Authority == "" {
				return SemanticGraph{}, fmt.Errorf("line %d: incomplete activity", line)
			}
			if seen[node.ID] {
				return SemanticGraph{}, fmt.Errorf("line %d: duplicate activity %s", line, node.ID)
			}
			seen[node.ID] = true
			graph.Nodes = append(graph.Nodes, node)
			graph.Relations = append(graph.Relations, SemanticRelation{ID: relationID(node.ID, node.MetricID), From: node.ID, To: node.MetricID, Kind: "MEASURES"})
		default:
			return SemanticGraph{}, fmt.Errorf("line %d: unsupported declaration %s", line, fields[0])
		}
	}
	if err := scanner.Err(); err != nil {
		return SemanticGraph{}, err
	}
	if !graphSeen || len(graph.Nodes) == 0 {
		return SemanticGraph{}, errors.New("source graph is empty")
	}
	return canonicalGraph(graph), nil
}

func stripLineComment(value string) string {
	for _, marker := range []string{"//", "#"} {
		if index := strings.Index(value, marker); index >= 0 {
			value = value[:index]
		}
	}
	return strings.TrimSpace(value)
}

func relationID(from, to string) string {
	return from + "|MEASURES|" + to
}

func LoadContract(path string) (Contract, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, nil, err
	}
	var contract Contract
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return Contract{}, nil, err
	}
	if err := validateContract(contract); err != nil {
		return Contract{}, nil, err
	}
	return contract, raw, nil
}

func validateContract(contract Contract) error {
	if contract.Schema != "gooo/semantic-drift-guard/denominator/v1" || contract.Total != 12 || len(contract.Cells) != 12 {
		return errors.New("invalid fixed denominator")
	}
	if len(contract.Proofs) != 3 || len(contract.IndicatorClasses) != 3 {
		return errors.New("invalid denominator balances")
	}
	proofs := map[string]int{}
	for _, balance := range contract.Proofs {
		proofs[balance.Choice] = balance.Total
	}
	indicators := map[string]int{}
	for _, balance := range contract.IndicatorClasses {
		indicators[balance.Class] = balance.Total
	}
	for _, choice := range []string{"FOUNDATION", "COHERENCE", "REGRESSION"} {
		if proofs[choice] != 4 {
			return errors.New("proof balance must be 4/4/4")
		}
	}
	for _, class := range []string{"DRIVER", "OUTCOME", "GUARDRAIL"} {
		if indicators[class] != 4 {
			return errors.New("indicator balance must be 4/4/4")
		}
	}
	seen := map[string]bool{}
	for index, cell := range contract.Cells {
		if cell.Ordinal != index+1 || cell.ID == "" || seen[cell.ID] || cell.Activity == "" || cell.MetricID == "" || cell.Authority != "READ_ONLY" || cell.Evaluator != "guard.Compare" {
			return errors.New("invalid denominator cell")
		}
		seen[cell.ID] = true
	}
	return nil
}

func validateGraphAgainstContract(graph SemanticGraph, contract Contract) error {
	if graph.Schema != graphSchema || len(graph.Nodes) != contract.Total || len(graph.Relations) != contract.Total {
		return errors.New("graph does not match fixed denominator cardinality")
	}
	nodes := map[string]ActivityNode{}
	for _, node := range graph.Nodes {
		if _, exists := nodes[node.ID]; exists {
			return errors.New("ambiguous graph node")
		}
		nodes[node.ID] = node
	}
	relations := map[string]SemanticRelation{}
	for _, relation := range graph.Relations {
		if _, exists := relations[relation.ID]; exists {
			return errors.New("ambiguous graph relation")
		}
		relations[relation.ID] = relation
	}
	for _, cell := range contract.Cells {
		node, ok := nodes[cell.ID]
		if !ok || node.Activity != cell.Activity || node.ProofChoice != cell.ProofChoice || node.IndicatorClass != cell.IndicatorClass || node.MetricID != cell.MetricID || node.Artifact != cell.Artifact || node.Authority != cell.Authority {
			return fmt.Errorf("graph cell binding mismatch: %s", cell.ID)
		}
		relation, ok := relations[relationID(node.ID, node.MetricID)]
		if !ok || relation.From != node.ID || relation.To != node.MetricID || relation.Kind != "MEASURES" {
			return fmt.Errorf("graph relation binding mismatch: %s", cell.ID)
		}
	}
	return nil
}

func Compile(sourceRaw []byte, sourcePath string, contractRaw []byte, contractPath string, irPath string) ([]byte, []byte, error) {
	graph, err := ParseSource(sourceRaw)
	if err != nil {
		return nil, nil, err
	}
	var contract Contract
	decoder := json.NewDecoder(bytes.NewReader(contractRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return nil, nil, err
	}
	if err := validateContract(contract); err != nil {
		return nil, nil, err
	}
	if err := validateGraphShape(graph, contract.Total); err != nil {
		return nil, nil, err
	}
	graphDigestValue, err := graphDigest(graph)
	if err != nil {
		return nil, nil, err
	}
	ir := SemanticIR{
		Schema: IRSchema, SourcePath: sourcePath, SourceSemanticDigest: graphDigestValue,
		ContractPath: contractPath, ContractDigest: DigestBytes(contractRaw), GraphDigest: graphDigestValue, Graph: canonicalGraph(graph),
	}
	irRaw, err := json.MarshalIndent(ir, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	nodeIDs := make([]string, 0, len(graph.Nodes))
	for _, node := range canonicalGraph(graph).Nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	relationIDs := make([]string, 0, len(graph.Relations))
	for _, relation := range canonicalGraph(graph).Relations {
		relationIDs = append(relationIDs, relation.ID)
	}
	binding := GeneratedBinding{
		Schema: GeneratedSchema, SourcePath: sourcePath, SourceSemanticDigest: graphDigestValue,
		IRPath: irPath, IRDigest: DigestBytes(irRaw), GraphDigest: graphDigestValue,
		NodeIDs: nodeIDs, RelationIDs: relationIDs,
	}
	bindingRaw, err := json.Marshal(binding)
	if err != nil {
		return nil, nil, err
	}
	generated := []byte("// Code generated by gooo-semantic-drift-guard compile; DO NOT EDIT.\npackage generated\n\n// semantic-binding: " + string(bindingRaw) + "\nconst SemanticBindingJSON = `" + string(bindingRaw) + "`\n")
	return irRaw, generated, nil
}

func WriteCompileOutputs(sourcePath, contractPath, outputIR, outputGo, irPath string) error {
	sourceRaw, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	contractRaw, err := os.ReadFile(contractPath)
	if err != nil {
		return err
	}
	irRaw, generatedRaw, err := Compile(sourceRaw, filepath.ToSlash(sourcePath), contractRaw, filepath.ToSlash(contractPath), filepath.ToSlash(irPath))
	if err != nil {
		return err
	}
	if err := writeCallerOutput(outputIR, irRaw); err != nil {
		return err
	}
	return writeCallerOutput(outputGo, generatedRaw)
}

func writeCallerOutput(path string, raw []byte) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("compile outputs must be absolute caller-owned paths")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func sortedStrings(values []string) []string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return copyValues
}
