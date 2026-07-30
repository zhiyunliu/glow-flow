package glowflow

import (
	"context"
	"testing"

	"github.com/zhiyunliu/glow-flow/nodetype"
)

type recordingNode struct {
	id        string
	name      string
	nodeType  nodetype.NodeType
	start     bool
	inputs    []any
	result    ExecuteResult
	execError error
}

func (n *recordingNode) Id() string              { return n.id }
func (n *recordingNode) Name() string            { return n.name }
func (n *recordingNode) Type() nodetype.NodeType { return n.nodeType }
func (n *recordingNode) IsStartNode() bool       { return n.start }
func (n *recordingNode) Config() any             { return nil }
func (n *recordingNode) Position() Position      { return Position{} }
func (n *recordingNode) Execute(ctx Context, data any) (ExecuteResult, error) {
	n.inputs = append(n.inputs, data)
	return n.result, n.execError
}
func (n *recordingNode) NextNodes() []CompiledNode { return nil }

func TestDispatcherRoutesByRelationType(t *testing.T) {
	start := &recordingNode{
		id:       "start",
		name:     "start",
		nodeType: nodetype.StartNode,
		start:    true,
		result: ExecuteResult{
			RelationType: "True",
			Data:         "payload-for-next",
		},
	}
	trueNode := &recordingNode{id: "true-node", name: "true", nodeType: nodetype.TaskNode}
	falseNode := &recordingNode{id: "false-node", name: "false", nodeType: nodetype.TaskNode}
	flow := &CompiledFlow{
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

	instanceID, err := NewDispatcher().Dispatch(NewContext(context.Background()), flow, "input")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if instanceID == "" {
		t.Fatal("dispatch instanceID is empty")
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
		nodeType: nodetype.StartNode,
		start:    true,
		result: ExecuteResult{
			RelationType: "unknown",
			Data:         42,
		},
	}
	defaultNode := &recordingNode{id: "default-node", name: "default", nodeType: nodetype.TaskNode}
	flow := &CompiledFlow{
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

	instanceID, err := NewDispatcher().Dispatch(NewContext(context.Background()), flow, "input")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if instanceID == "" {
		t.Fatal("dispatch instanceID is empty")
	}
	if len(defaultNode.inputs) != 1 || defaultNode.inputs[0] != 42 {
		t.Fatalf("default node inputs = %#v, want 42", defaultNode.inputs)
	}
}

func TestDispatcherReturnsDifferentInstanceIDPerDispatch(t *testing.T) {
	start := &recordingNode{id: "start", name: "start", nodeType: nodetype.StartNode, start: true}
	flow := &CompiledFlow{StartNodes: []CompiledNode{start}}
	dispatcher := NewDispatcher()

	firstID, err := dispatcher.Dispatch(NewContext(context.Background()), flow, "first")
	if err != nil {
		t.Fatalf("dispatch first: %v", err)
	}
	secondID, err := dispatcher.Dispatch(NewContext(context.Background()), flow, "second")
	if err != nil {
		t.Fatalf("dispatch second: %v", err)
	}

	if firstID == "" || secondID == "" {
		t.Fatalf("instanceIDs = %q, %q, want non-empty", firstID, secondID)
	}
	if firstID == secondID {
		t.Fatalf("instanceIDs = %q and %q, want different", firstID, secondID)
	}
}
