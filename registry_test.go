package glowflow

import "testing"

func TestRegistryUsesStringNodeTypes(t *testing.T) {
	nodeType := "custom-task"
	registry := NewRegistry()

	err := registry.Register(NodeDescriptor{
		Type: nodeType,
		Factory: nodeFactoryFunc(func(def NodeDefinition) (CompiledNode, error) {
			return nil, nil
		}),
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	desc, ok := registry.Get(nodeType)
	if !ok {
		t.Fatalf("get node %q ok = false, want true", nodeType)
	}
	if desc.Type != nodeType {
		t.Fatalf("descriptor type = %q, want %q", desc.Type, nodeType)
	}
}
