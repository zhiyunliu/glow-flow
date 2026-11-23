package glowflow

import (
	"fmt"
	"sync"
)

type NodeFactoryFunc func(def NodeDefinition) (CompiledNode, error)

func (f NodeFactoryFunc) Build(def NodeDefinition) (CompiledNode, error) {
	return f(def)
}

type NodeFactory interface {
	Build(def NodeDefinition) (CompiledNode, error)
}

type NodeDescriptor struct {
	Type          string
	Factory       NodeFactory
	DefaultConfig any
}

type NodeRegistry interface {
	Register(desc *NodeDescriptor) error
	GetDescriptor(nodeType string) (*NodeDescriptor, bool)
	RegisterCall(nodeType string, call NodeCall) error
	GetCall(nodeType string) (NodeCall, bool)
	List() []*NodeDescriptor
}

type registry struct {
	mu    sync.RWMutex
	nodes map[string]*NodeDescriptor
	calls map[string]NodeCall
}

var (
	DefaultRegistry = NewNodeRegistry()
)

func NewNodeRegistry() NodeRegistry {
	return &registry{
		nodes: make(map[string]*NodeDescriptor),
		calls: make(map[string]NodeCall),
	}
}

func (r *registry) Register(desc *NodeDescriptor) error {
	if desc.Type == "" {
		return fmt.Errorf("node type is empty")
	}
	if desc.Factory == nil {
		return fmt.Errorf("node factory is nil: %s", desc.Type)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[desc.Type]; ok {
		return fmt.Errorf("nodedesc:node type already registered: %s", desc.Type)
	}
	r.nodes[desc.Type] = desc
	return nil
}

func (r *registry) GetDescriptor(nodeType string) (*NodeDescriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.nodes[nodeType]
	return desc, ok
}

func (r *registry) RegisterCall(nodeType string, call NodeCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.calls[nodeType]; ok {
		return fmt.Errorf("nodecall:node type already registered: %s", nodeType)
	}
	r.calls[nodeType] = call
	return nil
}

func (r *registry) GetCall(nodeType string) (NodeCall, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	call, ok := r.calls[nodeType]
	return call, ok
}

func (r *registry) List() []*NodeDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	descriptors := make([]*NodeDescriptor, 0, len(r.nodes))
	for _, desc := range r.nodes {
		descriptors = append(descriptors, desc)
	}
	return descriptors
}
