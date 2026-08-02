package glowflow

import (
	"encoding/json"
	"testing"
)

func TestChainDefinitionPreservesJSONShape(t *testing.T) {
	def := ChainDefinition{
		ID:      "chain-a",
		Version: "v1",
		Metadata: ChainMetadata{
			ID:     "meta-a",
			Name:   "Chain A",
			Status: 1,
			ExtParams: map[string]any{
				"level": "gold",
			},
			Layout: Layout{Icon: "bolt", H: 1, X: 2, Y: 3, W: 4},
		},
		Endpoints: []EndpointDefinition{{ID: "endpoint-a", Name: "Endpoint A"}},
		Nodes:     []NodeDefinition{{ID: "node-a", Type: "start", Name: "Start"}},
		Connections: []ConnectionDefinition{{
			FromID: "node-a",
			ToID:   "node-b",
			Type:   "default",
		}},
	}

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal chain definition: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal chain definition: %v", err)
	}

	for _, field := range []string{"id", "version", "metadata", "endpoints", "nodes", "connections"} {
		if _, ok := got[field]; !ok {
			t.Fatalf("json field %q is missing in %s", field, string(data))
		}
	}

	metadata, ok := got["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v, want object", got["metadata"])
	}
	for _, field := range []string{"id", "name", "status", "extparams", "layout"} {
		if _, ok := metadata[field]; !ok {
			t.Fatalf("metadata json field %q is missing in %s", field, string(data))
		}
	}
	if _, ok := metadata["root"]; ok {
		t.Fatalf("metadata root should not be serialized in %s", string(data))
	}
	if _, ok := metadata["disabled"]; ok {
		t.Fatalf("metadata disabled should not be serialized in %s", string(data))
	}
	if metadata["status"] != float64(1) {
		t.Fatalf("metadata status = %#v, want 1", metadata["status"])
	}
}

func TestNodeDefinitionTypeUsesStringNodeTypeValues(t *testing.T) {
	for _, nodeType := range []string{"start", "task", "end"} {
		def := NodeDefinition{Type: nodeType}
		if def.Type != nodeType {
			t.Fatalf("node definition type = %q, want %q", def.Type, nodeType)
		}
	}
}
