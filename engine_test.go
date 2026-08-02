package glowflow

import (
	"context"
	"testing"
)

func TestEngineDispatchesLoadedFlowWithData(t *testing.T) {
	registry := NewRegistry()
	executed := make(map[string]int)
	inputs := make(map[string][]any)
	instances := make(map[string][]string)
	err := registry.Register(NodeDescriptor{
		Type: "start",
		Factory: func(def NodeDefinition) (CompiledNode, error) {
			return &recordingEngineNode{id: def.ID, nodeType: def.Type, start: true, executed: executed, inputs: inputs, instances: instances}, nil
		},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	engine := NewEngine(WithRegistry(registry))
	err = engine.Load(
		ChainDefinition{ID: "flow-a", Version: "v1", Nodes: []NodeDefinition{{ID: "start-a", Type: "start"}}},
		ChainDefinition{ID: "flow-b", Version: "v1", Nodes: []NodeDefinition{{ID: "start-b", Type: "start"}}},
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
	if len(instances["start-a"]) != 1 || instances["start-a"][0] != instanceID {
		t.Fatalf("start-a instances = %#v, want %q", instances["start-a"], instanceID)
	}

	secondInstanceID, err := engine.Dispatch("flow-a", "payload-b")
	if err != nil {
		t.Fatalf("dispatch flow-a second time: %v", err)
	}
	if secondInstanceID == "" {
		t.Fatal("dispatch flow-a second instanceID is empty")
	}
	if secondInstanceID == instanceID {
		t.Fatalf("dispatch flow-a instanceIDs = %q and %q, want different", instanceID, secondInstanceID)
	}
	if executed["start-a"] != 2 || executed["start-b"] != 0 {
		t.Fatalf("executed after second dispatch = %#v, want only flow-a twice", executed)
	}
	if len(inputs["start-a"]) != 2 || inputs["start-a"][1] != "payload-b" {
		t.Fatalf("start-a inputs after second dispatch = %#v, want payload-a then payload-b", inputs["start-a"])
	}
	if len(instances["start-a"]) != 2 || instances["start-a"][1] != secondInstanceID {
		t.Fatalf("start-a instances after second dispatch = %#v, want returned instance IDs", instances["start-a"])
	}
}

func TestEnginePauseResumeFlow(t *testing.T) {
	registry := NewRegistry()
	executed := make(map[string]int)
	err := registry.Register(NodeDescriptor{
		Type: "start",
		Factory: func(def NodeDefinition) (CompiledNode, error) {
			return &recordingEngineNode{id: def.ID, nodeType: def.Type, start: true, executed: executed}, nil
		},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	engine := NewEngine(WithRegistry(registry))
	err = engine.Load(
		ChainDefinition{ID: "flow-a", Version: "v1", Nodes: []NodeDefinition{{ID: "start-a", Type: "start"}}},
		ChainDefinition{ID: "flow-b", Version: "v1", Nodes: []NodeDefinition{{ID: "start-b", Type: "start"}}},
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
		Type: "start",
		Factory: func(def NodeDefinition) (CompiledNode, error) {
			return &recordingEngineNode{id: def.ID, nodeType: def.Type, start: true, executed: executed}, nil
		},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	engine := NewEngine(WithRegistry(registry))
	err = engine.Load(ChainDefinition{ID: "flow-a", Version: "v1", Nodes: []NodeDefinition{{ID: "start-v1", Type: "start"}}})
	if err != nil {
		t.Fatalf("load flow: %v", err)
	}
	err = engine.Reload(ChainDefinition{ID: "flow-a", Version: "v2", Nodes: []NodeDefinition{{ID: "start-v2", Type: "start"}}})
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
	id        string
	nodeType  string
	start     bool
	executed  map[string]int
	inputs    map[string][]any
	instances map[string][]string
}

func (n *recordingEngineNode) Id() string        { return n.id }
func (n *recordingEngineNode) Name() string      { return n.id }
func (n *recordingEngineNode) Type() string      { return n.nodeType }
func (n *recordingEngineNode) IsStartNode() bool { return n.start }
func (n *recordingEngineNode) Config() any       { return nil }
func (n *recordingEngineNode) Layout() Layout    { return Layout{} }
func (n *recordingEngineNode) Execute(ctx Context, data any) (ExecuteResult, error) {
	n.executed[n.id]++
	if n.inputs != nil {
		n.inputs[n.id] = append(n.inputs[n.id], data)
	}
	if n.instances != nil {
		n.instances[n.id] = append(n.instances[n.id], ctx.GetInstanceID())
	}
	return ExecuteResult{Data: data}, nil
}
func (n *recordingEngineNode) NextNodes() []CompiledNode { return nil }

func TestEngineRunUsesBackgroundContext(t *testing.T) {

	instanceID, err := newInstanceID()
	if err != nil {
		t.Fatalf("new instanceID: %v", err)
	}
	ctx := NewContext(context.Background(), instanceID)
	if ctx.Err() != nil {
		t.Fatalf("new context err = %v, want nil", ctx.Err())
	}
}
