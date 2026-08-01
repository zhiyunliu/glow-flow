package glowflow

import "testing"

func TestNodeDefinitionTypeUsesStringNodeTypeValues(t *testing.T) {
	for _, nodeType := range []string{"start", "task", "end"} {
		def := NodeDefinition{Type: nodeType}
		if def.Type != nodeType {
			t.Fatalf("node definition type = %q, want %q", def.Type, nodeType)
		}
	}
}
