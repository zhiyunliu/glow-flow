package glowflow

import (
	"fmt"
)

const DefaultRelationType = "default"

type FlowCompiler interface {
	Compile(def ChainDefinition, registry Registry) (*CompiledFlow, error)
}

type flowCompiler struct{}

func NewFlowCompiler() FlowCompiler {
	return &flowCompiler{}
}

type CompiledFlow struct {
	Definition ChainDefinition
	Nodes      map[string]CompiledNode
	Graph      map[string]map[string][]CompiledNode
	StartNodes []CompiledNode
}

func (f *CompiledFlow) NextNodes(nodeID string, relationType string) []CompiledNode {
	if f == nil {
		return nil
	}
	byRelation, ok := f.Graph[nodeID]
	if !ok {
		return nil
	}
	nodes := byRelation[relationType]
	if len(nodes) == 0 && relationType != DefaultRelationType {
		nodes = byRelation[DefaultRelationType]
	}
	return append([]CompiledNode(nil), nodes...)
}

func (c *flowCompiler) Compile(def ChainDefinition, registry Registry) (*CompiledFlow, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry is nil")
	}
	compiled := &CompiledFlow{
		Definition: def,
		Nodes:      make(map[string]CompiledNode, len(def.Nodes)),
		Graph:      make(map[string]map[string][]CompiledNode),
	}

	for _, nodeDef := range def.Nodes {
		if nodeDef.ID == "" {
			return nil, fmt.Errorf("node id is empty")
		}
		if _, exists := compiled.Nodes[nodeDef.ID]; exists {
			return nil, fmt.Errorf("duplicate node id: %s", nodeDef.ID)
		}
		desc, ok := registry.Get(nodeDef.Type)
		if !ok {
			return nil, fmt.Errorf("node type is not registered: %s", nodeDef.Type)
		}
		node, err := desc.Factory(nodeDef)
		if err != nil {
			return nil, fmt.Errorf("create node %s: %w", nodeDef.ID, err)
		}
		compiled.Nodes[nodeDef.ID] = node
		if node.IsStartNode() {
			compiled.StartNodes = append(compiled.StartNodes, node)
		}
	}

	for _, connection := range def.Connections {
		fromNode, ok := compiled.Nodes[connection.FromID]
		if !ok {
			return nil, fmt.Errorf("connection from node not found: %s", connection.FromID)
		}
		toNode, ok := compiled.Nodes[connection.ToID]
		if !ok {
			return nil, fmt.Errorf("connection to node not found: %s", connection.ToID)
		}
		relationType := connection.Type
		if relationType == "" {
			relationType = DefaultRelationType
		}
		if compiled.Graph[fromNode.Id()] == nil {
			compiled.Graph[fromNode.Id()] = make(map[string][]CompiledNode)
		}
		compiled.Graph[fromNode.Id()][relationType] = append(compiled.Graph[fromNode.Id()][relationType], toNode)
	}

	return compiled, nil
}
