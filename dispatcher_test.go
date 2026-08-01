package glowflow

import (
	"context"
	"testing"
)

type recordingNode struct {
	id        string
	name      string
	nodeType  string
	start     bool
	inputs    []any
	instances []string
	result    ExecuteResult
	execError error
}

func (n *recordingNode) Id() string         { return n.id }
func (n *recordingNode) Name() string       { return n.name }
func (n *recordingNode) Type() string       { return n.nodeType }
func (n *recordingNode) IsStartNode() bool  { return n.start }
func (n *recordingNode) Config() any        { return nil }
func (n *recordingNode) Position() Position { return Position{} }
func (n *recordingNode) Execute(ctx Context, data any) (ExecuteResult, error) {
	n.inputs = append(n.inputs, data)
	n.instances = append(n.instances, ctx.GetInstanceID())
	return n.result, n.execError
}
func (n *recordingNode) NextNodes() []CompiledNode { return nil }

func TestDispatcherRoutesByRelationType(t *testing.T) {
	start := &recordingNode{
		id:       "start",
		name:     "start",
		nodeType: "start",
		start:    true,
		result: ExecuteResult{
			RelationType: "True",
			Data:         "payload-for-next",
		},
	}
	trueNode := &recordingNode{id: "true-node", name: "true", nodeType: "task"}
	falseNode := &recordingNode{id: "false-node", name: "false", nodeType: "task"}
	flow := &CompiledChain{
		Nodes: map[string]CompiledNode{
			"start":      start,
			"true-node":  trueNode,
			"false-node": falseNode,
		},
		Graph: map[string]map[string][]CompiledNode{
			"start": {
				"True":  {trueNode},
				"False": {falseNode},
			},
		},
		StartNodes: []CompiledNode{start},
	}

	err := NewDispatcher().Dispatch(NewContext(context.Background(), "instance-1"), flow, "input")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(start.inputs) != 1 || start.inputs[0] != "input" {
		t.Fatalf("start inputs = %#v, want input", start.inputs)
	}
	if len(trueNode.inputs) != 1 || trueNode.inputs[0] != "payload-for-next" {
		t.Fatalf("true node inputs = %#v, want payload-for-next", trueNode.inputs)
	}
	if len(falseNode.inputs) != 0 {
		t.Fatalf("false node inputs = %#v, want none", falseNode.inputs)
	}
}

func TestDispatcherUsesDefaultRelationFallback(t *testing.T) {
	start := &recordingNode{
		id:       "start",
		name:     "start",
		nodeType: "start",
		start:    true,
		result: ExecuteResult{
			RelationType: "unknown",
			Data:         42,
		},
	}
	defaultNode := &recordingNode{id: "default-node", name: "default", nodeType: "task"}
	flow := &CompiledChain{
		Nodes: map[string]CompiledNode{
			"start":        start,
			"default-node": defaultNode,
		},
		Graph: map[string]map[string][]CompiledNode{
			"start": {
				DefaultRelationType: {defaultNode},
			},
		},
		StartNodes: []CompiledNode{start},
	}

	err := NewDispatcher().Dispatch(NewContext(context.Background(), "instance-1"), flow, "input")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(defaultNode.inputs) != 1 || defaultNode.inputs[0] != 42 {
		t.Fatalf("default node inputs = %#v, want 42", defaultNode.inputs)
	}
}

func TestDispatcherReturnsDifferentInstanceIDPerDispatch(t *testing.T) {
	start := &recordingNode{id: "start", name: "start", nodeType: "start", start: true}
	flow := &CompiledChain{StartNodes: []CompiledNode{start}}
	dispatcher := NewDispatcher()

	err := dispatcher.Dispatch(NewContext(context.Background(), "instance-1"), flow, "first")
	if err != nil {
		t.Fatalf("dispatch first: %v", err)
	}
	err = dispatcher.Dispatch(NewContext(context.Background(), "instance-2"), flow, "second")
	if err != nil {
		t.Fatalf("dispatch second: %v", err)
	}
	if len(start.instances) != 2 {
		t.Fatalf("start instances = %#v, want two dispatches", start.instances)
	}
	if start.instances[0] != "instance-1" || start.instances[1] != "instance-2" {
		t.Fatalf("start instances = %#v, want instance-1 then instance-2", start.instances)
	}
	if start.instances[0] == start.instances[1] {
		t.Fatalf("start instances = %#v, want different instance IDs", start.instances)
	}
}
