package rpcnode

import (
	glowflow "github.com/zhiyunliu/glow-flow"
)

type rpcNodeFactory struct {
	name string
}

var _ glowflow.NodeFactory = NewRpcNodeFactory()

func NewRpcNodeFactory() glowflow.NodeFactory {
	return &rpcNodeFactory{}
}

func (f *rpcNodeFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	return &rpcNode{
		name: def.Name,
	}, nil
}
