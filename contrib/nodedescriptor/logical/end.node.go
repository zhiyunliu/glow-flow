package logical

import (
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
)

const EndNodeType = "end"

// 结束节点
/*
显式结束当前分支(节点类型 end)：即使配置了后续连接也不再执行，当前消息作为该分支的结果；
在 for/while 循环体及 groupaction 中，end 节点的消息即为该分支的输出。
无需配置。
*/
type nodeEnd struct {
	glowflow.Node
}

func (n *nodeEnd) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	return glowflow.ExecuteResult{RelationType: relationtype.Success, Msg: msg, End: true}, nil
}

type endFactory struct{}

func (f *endFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	return &nodeEnd{Node: def}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    EndNodeType,
		Factory: &endFactory{},
	})
}
