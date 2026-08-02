package localnode

import glowflow "github.com/zhiyunliu/glow-flow"

var _ glowflow.CompiledNode = &localNode{}

type localNode struct {
	name string
}

func (n *localNode) Id() string {
	return n.name
}

func (n *localNode) Name() string {
	return n.name
}

func (n *localNode) Type() string {
	return "local"
}

func (n *localNode) Execute(ctx glowflow.Context, data any) (glowflow.ExecuteResult, error) {
	return glowflow.ExecuteResult{
		RelationType: "local",
		Data:         nil,
	}, nil
}

func (n *localNode) Config() any {
	return nil
}

func (n *localNode) Layout() glowflow.Layout {
	return glowflow.Layout{}
}

func (n *localNode) IsStartNode() bool {
	return false
}
