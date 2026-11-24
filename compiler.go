package glowflow

import (
	"fmt"
	"sync"
)

const DefaultRelationType = "default"

// ChainCompiler is an interface for compiling chain definitions into executable chains.
type ChainCompiler interface {
	Compile(def ChainDefinition, registry NodeRegistry) (*CompiledChain, error)
}

type chainCompiler struct{}

var (
	DefaultChainCompiler = NewChainCompiler()
)

// NewChainCompiler creates a new instance of the default chain compiler.
func NewChainCompiler() ChainCompiler {
	return &chainCompiler{}
}

type CompiledChain struct {
	Definition ChainDefinition
	Nodes      map[string]CompiledNode
	Graph      map[string]map[string][]CompiledNode
	StartNodes []CompiledNode

	ancestorMu    sync.Mutex
	ancestorCache map[string]map[string]struct{}
}

func (c *CompiledChain) NextNodes(nodeID string, relationType string) []CompiledNode {
	if c == nil {
		return nil
	}
	byRelation, ok := c.Graph[nodeID]
	if !ok {
		return nil
	}
	nodes := byRelation[relationType]
	if len(nodes) == 0 && relationType != DefaultRelationType {
		nodes = byRelation[DefaultRelationType]
	}
	return append([]CompiledNode{}, nodes...)
}

// nextNodesOf 按执行结果查找后续节点：End 时没有后续节点；RelationTypes 非空时合并各关系的后续节点并去重
func (c *CompiledChain) nextNodesOf(nodeID string, result ExecuteResult) []CompiledNode {
	if result.End {
		return nil
	}
	if len(result.RelationTypes) == 0 {
		return c.NextNodes(nodeID, result.RelationType)
	}
	var nodes []CompiledNode
	seen := make(map[string]struct{})
	for _, relationType := range result.RelationTypes {
		for _, node := range c.NextNodes(nodeID, relationType) {
			if _, ok := seen[node.Id()]; ok {
				continue
			}
			seen[node.Id()] = struct{}{}
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// ancestors 返回可经连接到达 nodeID 的所有节点(不含 nodeID 自身)，用于判断汇聚节点是否还需等待
func (c *CompiledChain) ancestors(nodeID string) map[string]struct{} {
	c.ancestorMu.Lock()
	defer c.ancestorMu.Unlock()
	if set, ok := c.ancestorCache[nodeID]; ok {
		return set
	}

	reverse := make(map[string][]string)
	for fromID, byRelation := range c.Graph {
		for _, nodes := range byRelation {
			for _, node := range nodes {
				reverse[node.Id()] = append(reverse[node.Id()], fromID)
			}
		}
	}
	set := make(map[string]struct{})
	queue := []string{nodeID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, fromID := range reverse[id] {
			if _, seen := set[fromID]; seen || fromID == nodeID {
				continue
			}
			set[fromID] = struct{}{}
			queue = append(queue, fromID)
		}
	}

	if c.ancestorCache == nil {
		c.ancestorCache = make(map[string]map[string]struct{})
	}
	c.ancestorCache[nodeID] = set
	return set
}

func (c *chainCompiler) Compile(def ChainDefinition, registry NodeRegistry) (*CompiledChain, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry is nil")
	}
	compiled := &CompiledChain{
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
		desc, ok := registry.GetDescriptor(nodeDef.NodeType)
		if !ok {
			return nil, fmt.Errorf("node type is not registered: %s", nodeDef.NodeType)
		}
		node, err := desc.Factory.Build(nodeDef)
		if err != nil {
			return nil, fmt.Errorf("create node %s: %w", nodeDef.ID, err)
		}
		compiled.Nodes[nodeDef.ID] = node
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
	referenced, err := c.collectReferences(compiled)
	if err != nil {
		return nil, err
	}
	c.caclStartNodes(compiled, referenced)
	return compiled, nil
}

// collectReferences 校验 NodeReferrer 引用的节点均存在，并返回被引用的节点集合
func (c *chainCompiler) collectReferences(compiled *CompiledChain) (map[string]struct{}, error) {
	referenced := make(map[string]struct{})
	for _, nodeDef := range compiled.Definition.Nodes {
		referrer, ok := compiled.Nodes[nodeDef.ID].(NodeReferrer)
		if !ok {
			continue
		}
		for _, refID := range referrer.ReferencedNodes() {
			if refID == nodeDef.ID {
				return nil, fmt.Errorf("node %s references itself", nodeDef.ID)
			}
			if _, exists := compiled.Nodes[refID]; !exists {
				return nil, fmt.Errorf("node %s references unknown node: %s", nodeDef.ID, refID)
			}
			referenced[refID] = struct{}{}
		}
	}
	return referenced, nil
}

// caclStartNodes 没有入边且未被其他节点引用的节点作为起始节点
func (c *chainCompiler) caclStartNodes(compiled *CompiledChain, referenced map[string]struct{}) {
	compiled.StartNodes = nil
	incoming := make(map[string]struct{}, len(compiled.Definition.Connections))
	for _, connection := range compiled.Definition.Connections {
		incoming[connection.ToID] = struct{}{}
	}

	for _, nodeDef := range compiled.Definition.Nodes {
		if _, exists := incoming[nodeDef.ID]; exists {
			continue
		}
		if _, exists := referenced[nodeDef.ID]; exists {
			continue
		}
		node, ok := compiled.Nodes[nodeDef.ID]
		if ok {
			compiled.StartNodes = append(compiled.StartNodes, node)
		}
	}
}
