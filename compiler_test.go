package glowflow

import (
	"testing"
)

type testNode struct {
	id      string
	name    string
	nodeTyp string
	start   bool
	config  any
	layout  Layout
	result  ExecuteResult
}

func (n *testNode) Id() string                                           { return n.id }
func (n *testNode) Name() string                                         { return n.name }
func (n *testNode) Type() string                                         { return n.nodeTyp }
func (n *testNode) IsStartNode() bool                                    { return n.start }
func (n *testNode) Config() any                                          { return n.config }
func (n *testNode) Layout() Layout                                       { return n.layout }
func (n *testNode) Execute(ctx Context, data any) (ExecuteResult, error) { return n.result, nil }
func (n *testNode) NextNodes() []CompiledNode                            { return nil }

func TestFlowCompilerBuildsRelationGraph(t *testing.T) {
	registry := NewNodeRegistry()
	err := registry.Register(&NodeDescriptor{
		Type: "start",
		Factory: NodeFactoryFunc(func(def NodeDefinition) (CompiledNode, error) {
			return &testNode{id: def.ID, name: def.NodeName, nodeTyp: def.NodeType, start: true, config: def.ExtParams}, nil
		}),
	})
	if err != nil {
		t.Fatalf("register start node: %v", err)
	}
	err = registry.Register(&NodeDescriptor{
		Type: "task",
		Factory: NodeFactoryFunc(func(def NodeDefinition) (CompiledNode, error) {
			return &testNode{id: def.ID, name: def.NodeName, nodeTyp: def.NodeType, config: def.ExtParams}, nil
		}),
	})
	if err != nil {
		t.Fatalf("register task node: %v", err)
	}

	compiled, err := NewChainCompiler().Compile(ChainDefinition{
		ID:      "flow-a",
		Version: "v1",
		Nodes: []NodeDefinition{
			{ID: "start", NodeName: "start", NodeType: "start"},
			{ID: "true-node", NodeName: "true", NodeType: "task"},
			{ID: "false-node", NodeName: "false", NodeType: "task"},
		},
		Connections: []ConnectionDefinition{
			{FromID: "start", ToID: "true-node", Type: "True"},
			{FromID: "start", ToID: "false-node", Type: "False"},
		},
	}, registry)
	if err != nil {
		t.Fatalf("compile flow: %v", err)
	}

	if compiled.Definition.ID != "flow-a" {
		t.Fatalf("compiled flow ID = %q, want %q", compiled.Definition.ID, "flow-a")
	}
	if len(compiled.StartNodes) != 1 || compiled.StartNodes[0].Id() != "start" {
		t.Fatalf("start nodes = %#v, want only start", compiled.StartNodes)
	}
	trueNodes := compiled.NextNodes("start", "True")
	if len(trueNodes) != 1 || trueNodes[0].Id() != "true-node" {
		t.Fatalf("True next nodes = %#v, want true-node", trueNodes)
	}
	falseNodes := compiled.NextNodes("start", "False")
	if len(falseNodes) != 1 || falseNodes[0].Id() != "false-node" {
		t.Fatalf("False next nodes = %#v, want false-node", falseNodes)
	}
}

func TestChainCompilerRejectsUnknownNodeType(t *testing.T) {
	_, err := NewChainCompiler().Compile(ChainDefinition{
		ID:      "flow-a",
		Version: "v1",
		Nodes: []NodeDefinition{
			{ID: "start", NodeName: "start", NodeType: "start"},
		},
	}, NewNodeRegistry())
	if err == nil {
		t.Fatal("compile error is nil, want unknown node type error")
	}
}
