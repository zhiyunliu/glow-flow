package glowflow

import (
	"fmt"
	"sync"
)

type NodeFactory func(def NodeDefinition) (CompiledNode, error)

type NodeDescriptor struct {
	Type          string
	Factory       NodeFactory
	DefaultConfig any
}

type Registry interface {
	Register(desc NodeDescriptor) error
	Get(nodeType string) (NodeDescriptor, bool)
	List() []NodeDescriptor
}

type registry struct {
	mu    sync.RWMutex
	nodes map[string]NodeDescriptor
}

func NewRegistry() Registry {
	return &registry{nodes: make(map[string]NodeDescriptor)}
}

func (r *registry) Register(desc NodeDescriptor) error {
	if desc.Type == "" {
		return fmt.Errorf("node type is empty")
	}
	if desc.Factory == nil {
		return fmt.Errorf("node factory is nil: %s", desc.Type)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[desc.Type]; ok {
		return fmt.Errorf("node type already registered: %s", desc.Type)
	}
	r.nodes[desc.Type] = desc
	return nil
}

func (r *registry) Get(nodeType string) (NodeDescriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.nodes[nodeType]
	return desc, ok
}

func (r *registry) List() []NodeDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	descriptors := make([]NodeDescriptor, 0, len(r.nodes))
	for _, desc := range r.nodes {
		descriptors = append(descriptors, desc)
	}
	return descriptors
}
