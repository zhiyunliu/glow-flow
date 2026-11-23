package glowflow

import "testing"

func TestRegistryUsesStringNodeTypes(t *testing.T) {
	nodeType := "custom-task"
	registry := NewNodeRegistry()

	err := registry.Register(&NodeDescriptor{
		Type: nodeType,
		Factory: NodeFactoryFunc(func(def NodeDefinition) (CompiledNode, error) {
			return nil, nil
		}),
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	desc, ok := registry.GetDescriptor(nodeType)
	if !ok {
		t.Fatalf("get node %q ok = false, want true", nodeType)
	}
	if desc.Type != nodeType {
		t.Fatalf("descriptor type = %q, want %q", desc.Type, nodeType)
	}
}

func TestRegistryRegisterCall(t *testing.T) {
	registry := NewNodeRegistry()
	call := NodeCall(func(ctx Context, msg *Message) (ExecuteResult, error) {
		return ExecuteResult{RelationType: "success", Msg: msg}, nil
	})

	if err := registry.RegisterCall("custom-call", call); err != nil {
		t.Fatalf("register call: %v", err)
	}
	if err := registry.RegisterCall("custom-call", call); err == nil {
		t.Fatal("duplicate register call error is nil")
	}

	got, ok := registry.GetCall("custom-call")
	if !ok || got == nil {
		t.Fatalf("get call ok = %v, want registered call", ok)
	}
	msg := NewMessage("msg-1", 1)
	result, err := got(nil, msg)
	if err != nil || result.RelationType != "success" || result.Msg != msg {
		t.Fatalf("call result = %+v, %v", result, err)
	}
	if _, ok := registry.GetCall("missing"); ok {
		t.Fatal("get missing call ok = true, want false")
	}
}
