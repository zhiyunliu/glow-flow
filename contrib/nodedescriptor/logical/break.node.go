package logical

import (
	glowflow "github.com/zhiyunliu/glow-flow"
)

const BreakNodeType = "break"

// 跳出循环
/*
在 for/while 循环体中使用(节点类型 break)：标记最近一层循环在本次迭代结束后退出，
本次迭代的其他分支仍会执行完毕；消息原样走 success 分支。不在循环中时仅记录警告。
无需配置。
*/
type nodeBreak struct {
	glowflow.Node
}

func (n *nodeBreak) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	if !BreakLoop(ctx) {
		glowflow.GetLogger(ctx).Warnf("node[%s:%s] break is not inside a loop", n.Type(), n.Id())
	}
	return successResult(msg), nil
}

type breakFactory struct{}

func (f *breakFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	return &nodeBreak{Node: def}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    BreakNodeType,
		Factory: &breakFactory{},
	})
}
