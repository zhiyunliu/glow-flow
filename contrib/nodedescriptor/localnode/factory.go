package localnode

import glowflow "github.com/zhiyunliu/glow-flow"

type localNodeFactory struct {
	name string
}

var _ glowflow.NodeFactory = NewLocalNodeFactory()

func NewLocalNodeFactory() glowflow.NodeFactory {
	return &localNodeFactory{}
}

func (f *localNodeFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	return &localNode{
		name: def.Name,
	}, nil
}
