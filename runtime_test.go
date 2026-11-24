package glowflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type funcNode struct {
	testNode
	exec func(ctx Context, msg *Message) (ExecuteResult, error)
}

func (n *funcNode) Execute(ctx Context, msg *Message) (ExecuteResult, error) {
	return n.exec(ctx, msg)
}

func newFuncNode(id string, exec func(ctx Context, msg *Message) (ExecuteResult, error)) *funcNode {
	return &funcNode{testNode: testNode{id: id, name: id, nodeTyp: "func"}, exec: exec}
}

func TestDispatchReturnsNodeErrorAndRecoversPanic(t *testing.T) {
	boom := errors.New("boom")
	failing := newFuncNode("failing", func(Context, *Message) (ExecuteResult, error) { return ExecuteResult{}, boom })
	panicking := newFuncNode("panicking", func(Context, *Message) (ExecuteResult, error) { panic("bad") })
	other := &recordingNode{id: "other"}

	for _, start := range []CompiledNode{failing, panicking} {
		other.inputs = nil
		flow := &CompiledChain{StartNodes: []CompiledNode{start, other}}
		err := NewDispatcher().Dispatch(NewContext(context.Background(), "i"), flow, NewMessage("m", "x"))
		if err == nil || !strings.Contains(err.Error(), "execute node "+start.Id()) {
			t.Fatalf("dispatch error = %v, want error from %s", err, start.Id())
		}
		if len(other.inputs) != 1 {
			t.Fatalf("other branch should still run when %s fails", start.Id())
		}
	}
	err := NewDispatcher().Dispatch(NewContext(context.Background(), "i"), &CompiledChain{StartNodes: []CompiledNode{failing}}, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("dispatch error = %v, want wrapped boom", err)
	}
}

func TestNextNodesOfMultipleRelations(t *testing.T) {
	a, b, d := &testNode{id: "a"}, &testNode{id: "b"}, &testNode{id: "d"}
	flow := &CompiledChain{Graph: map[string]map[string][]CompiledNode{
		"start": {"x": {a}, "y": {b, a}, DefaultRelationType: {d}},
	}}
	ids := func(nodes []CompiledNode) []string {
		var result []string
		for _, n := range nodes {
			result = append(result, n.Id())
		}
		return result
	}
	if got := ids(flow.nextNodesOf("start", ExecuteResult{RelationTypes: []string{"x", "y", "z"}})); strings.Join(got, ",") != "a,b,d" {
		t.Fatalf("next nodes = %v, want a,b,d (deduplicated, z falls back to default)", got)
	}
	if got := flow.nextNodesOf("start", ExecuteResult{RelationType: "x", End: true}); len(got) != 0 {
		t.Fatalf("End result next nodes = %v, want none", ids(got))
	}
}

type referrerNode struct {
	testNode
	refs []string
}

func (n *referrerNode) ReferencedNodes() []string { return n.refs }

func TestCompilerExcludesReferencedNodesFromStart(t *testing.T) {
	registry := NewNodeRegistry()
	_ = registry.Register(&NodeDescriptor{Type: "task", Factory: NodeFactoryFunc(func(def NodeDefinition) (CompiledNode, error) {
		return &testNode{id: def.ID, nodeTyp: def.NodeType}, nil
	})})
	_ = registry.Register(&NodeDescriptor{Type: "loop", Factory: NodeFactoryFunc(func(def NodeDefinition) (CompiledNode, error) {
		return &referrerNode{testNode: testNode{id: def.ID, nodeTyp: def.NodeType}, refs: []string{def.ExtParams.GetString("do")}}, nil
	})})

	def := ChainDefinition{ID: "f", Version: "v1", Nodes: []NodeDefinition{
		{ID: "loop", NodeType: "loop", ExtParams: map[string]any{"do": "body"}},
		{ID: "body", NodeType: "task"},
		{ID: "other", NodeType: "task"},
	}}
	compiled, err := NewChainCompiler().Compile(def, registry)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var starts []string
	for _, n := range compiled.StartNodes {
		starts = append(starts, n.Id())
	}
	if strings.Join(starts, ",") != "loop,other" {
		t.Fatalf("start nodes = %v, want loop,other", starts)
	}

	def.Nodes[0].ExtParams = map[string]any{"do": "missing"}
	if _, err := NewChainCompiler().Compile(def, registry); err == nil {
		t.Fatal("compile should fail for unknown referenced node")
	}
}
