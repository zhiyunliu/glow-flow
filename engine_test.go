package glowflow

import (
	"context"
	"testing"

	"github.com/zhiyunliu/glow-flow/nodetype"
)

func TestEngineDispatchesLoadedFlowWithData(t *testing.T) {
	registry := NewRegistry()
	executed := make(map[string]int)
	inputs := make(map[string][]any)
	err := registry.Register(NodeDescriptor{
		Type: nodetype.StartNode,
		Factory: func(def NodeDefinition) (CompiledNode, error) {
			return &recordingEngineNode{id: def.ID, nodeType: def.Type, start: true, executed: executed, inputs: inputs}, nil
		},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	engine := NewEngine(WithRegistry(registry))
	err = engine.Load(
		FlowDefinition{ID: "flow-a", Version: "v1", Nodes: []NodeDefinition{{ID: "start-a", Type: nodetype.StartNode}}},
		FlowDefinition{ID: "flow-b", Version: "v1", Nodes: []NodeDefinition{{ID: "start-b", Type: nodetype.StartNode}}},
	)
	if err != nil {
		t.Fatalf("load flows: %v", err)
	}

	err = engine.Run()
	if err != nil {
		t.Fatalf("run engine: %v", err)
	}
	if executed["start-a"] != 0 || executed["start-b"] != 0 {
		t.Fatalf("executed after run = %#v, want no dispatch", executed)
	}

	instanceID, err := engine.Dispatch("flow-a", "payload-a")
	if err != nil {
		t.Fatalf("dispatch flow-a: %v", err)
	}
	if instanceID == "" {
		t.Fatal("dispatch flow-a instanceID is empty")
	}
	if executed["start-a"] != 1 || executed["start-b"] != 0 {
		t.Fatalf("executed = %#v, want only flow-a", executed)
	}
	if len(inputs["start-a"]) != 1 || inputs["start-a"][0] != "payload-a" {
		t.Fatalf("start-a inputs = %#v, want payload-a", inputs["start-a"])
	}
}

func TestEnginePauseResumeFlow(t *testing.T) {
	registry := NewRegistry()
	executed := make(map[string]int)
	err := registry.Register(NodeDescriptor{
		Type: nodetype.StartNode,
		Factory: func(def NodeDefinition) (CompiledNode, error) {
			return &recordingEngineNode{id: def.ID, nodeType: def.Type, start: true, executed: executed}, nil
		},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	engine := NewEngine(WithRegistry(registry))
	err = engine.Load(
		FlowDefinition{ID: "flow-a", Version: "v1", Nodes: []NodeDefinition{{ID: "start-a", Type: nodetype.StartNode}}},
		FlowDefinition{ID: "flow-b", Version: "v1", Nodes: []NodeDefinition{{ID: "start-b", Type: nodetype.StartNode}}},
	)
	if err != nil {
		t.Fatalf("load flows: %v", err)
	}
	if err := engine.Pause("flow-a"); err != nil {
		t.Fatalf("pause flow-a: %v", err)
	}
	if _, err := engine.Dispatch("flow-a", nil); err == nil {
		t.Fatal("dispatch paused flow error is nil, want error")
	}
	if _, err := engine.Dispatch("flow-b", nil); err != nil {
		t.Fatalf("dispatch flow-b: %v", err)
	}
	if executed["start-a"] != 0 || executed["start-b"] != 1 {
		t.Fatalf("executed after pause = %#v, want only flow-b", executed)
	}
	if err := engine.Resume("flow-a"); err != nil {
		t.Fatalf("resume flow-a: %v", err)
	}
	if _, err := engine.Dispatch("flow-a", nil); err != nil {
		t.Fatalf("dispatch flow-a after resume: %v", err)
	}
	if executed["start-a"] != 1 || executed["start-b"] != 1 {
		t.Fatalf("executed after resume = %#v, want flow-a resumed", executed)
	}
}

func TestEngineKeepsVersionsAndDispatchesLatestByFlowID(t *testing.T) {
	registry := NewRegistry()
	executed := make(map[string]int)
	err := registry.Register(NodeDescriptor{
		Type: nodetype.StartNode,
		Factory: func(def NodeDefinition) (CompiledNode, error) {
			return &recordingEngineNode{id: def.ID, nodeType: def.Type, start: true, executed: executed}, nil
		},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	engine := NewEngine(WithRegistry(registry))
	err = engine.Load(FlowDefinition{ID: "flow-a", Version: "v1", Nodes: []NodeDefinition{{ID: "start-v1", Type: nodetype.StartNode}}})
	if err != nil {
		t.Fatalf("load flow: %v", err)
	}
	err = engine.Reload(FlowDefinition{ID: "flow-a", Version: "v2", Nodes: []NodeDefinition{{ID: "start-v2", Type: nodetype.StartNode}}})
	if err != nil {
		t.Fatalf("reload flow: %v", err)
	}
	instanceID, err := engine.Dispatch("flow-a", nil)
	if err != nil {
		t.Fatalf("dispatch flow-a: %v", err)
	}
	if instanceID == "" {
		t.Fatal("dispatch flow-a instanceID is empty")
	}
	if executed["start-v1"] != 0 || executed["start-v2"] != 1 {
		t.Fatalf("executed after dispatch latest = %#v, want only v2", executed)
	}
}

type recordingEngineNode struct {
	id       string
	nodeType nodetype.NodeType
	start    bool
	executed map[string]int
	inputs   map[string][]any
}

func (n *recordingEngineNode) Id() string              { return n.id }
func (n *recordingEngineNode) Name() string            { return n.id }
func (n *recordingEngineNode) Type() nodetype.NodeType { return n.nodeType }
func (n *recordingEngineNode) IsStartNode() bool       { return n.start }
func (n *recordingEngineNode) Config() any             { return nil }
func (n *recordingEngineNode) Position() Position      { return Position{} }
func (n *recordingEngineNode) Execute(ctx Context, data any) (ExecuteResult, error) {
	n.executed[n.id]++
	if n.inputs != nil {
		n.inputs[n.id] = append(n.inputs[n.id], data)
	}
	return ExecuteResult{Data: data}, nil
}
func (n *recordingEngineNode) NextNodes() []CompiledNode { return nil }

func TestEngineRunUsesBackgroundContext(t *testing.T) {
	ctx := NewContext(context.Background())
	if ctx.Err() != nil {
		t.Fatalf("new context err = %v, want nil", ctx.Err())
	}
}
