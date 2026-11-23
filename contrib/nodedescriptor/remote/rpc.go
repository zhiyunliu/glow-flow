package remote

import glowflow "github.com/zhiyunliu/glow-flow"

var _ glowflow.CompiledNode = &rpcNode{}

type rpcNode struct {
	name string
}

func (n *rpcNode) Id() string {
	return n.name
}
func (n *rpcNode) Name() string {
	return n.name
}
func (n *rpcNode) Type() string {
	return "rpc"
}

func (n *rpcNode) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	return glowflow.ExecuteResult{
		RelationType: "rpc",
		Msg:          nil,
	}, nil
}

func (n *rpcNode) Config() any {
	return nil
}
func (n *rpcNode) Layout() glowflow.Layout {
	return glowflow.Layout{}
}

func (n *rpcNode) IsStartNode() bool {
	return false
}
