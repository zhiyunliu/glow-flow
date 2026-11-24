package logical

import (
	glowflow "github.com/zhiyunliu/glow-flow"
)

const ForkNodeType = "fork"

// 并行网关
/*
将消息原样发送给所有 success(或 default)连接的后续节点并行执行(节点类型 fork)，通常与 join 配合使用。
无需配置。
*/
type nodeFork struct {
	glowflow.Node
}

func (n *nodeFork) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	return successResult(msg), nil
}

type forkFactory struct{}

func (f *forkFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	return &nodeFork{Node: def}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    ForkNodeType,
		Factory: &forkFactory{},
	})
}
