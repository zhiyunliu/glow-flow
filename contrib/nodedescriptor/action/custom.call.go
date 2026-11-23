package action

import (
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
	"github.com/zhiyunliu/glow-flow/errors"
)

/**
 * Custom Call Node
 * 根据用户自己定义的函数进行调用
 */
type nodeCustomCall struct {
	glowflow.Node
	funcCall glowflow.NodeCall
}

type customCallFactory struct{}

func (n *nodeCustomCall) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	if n.funcCall == nil {
		return glowflow.ExecuteResult{
			RelationType: relationtype.Failure,
			Msg:          msg,
		}, nil
	}
	return n.funcCall(ctx, msg)
}
func (f *customCallFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	nodeType := def.ExtParams.GetString("nodetype")

	nodeCall, ok := glowflow.DefaultRegistry.GetCall(nodeType)
	if !ok {
		return nil, errors.ErrInvalidNodeConfig
	}
	return &nodeCustomCall{
		Node:     def,
		funcCall: nodeCall,
	}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    "custom",
		Factory: &customCallFactory{},
	})
}
