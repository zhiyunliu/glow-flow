package action

import (
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
)

type delayNode struct {
	glowflow.Node
	delay time.Duration
}

func (n *delayNode) Execute(ctx glowflow.Context, data any) (glowflow.ExecuteResult, error) {
	time.Sleep(n.delay)
	return glowflow.ExecuteResult{
		RelationType: relationtype.Success,
		Data:         data,
	}, nil
}

type delayFactory struct{}

func (f *delayFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	delay := def.ExtParams.GetString("delay")
	d, err := time.ParseDuration(delay)
	if err != nil {
		return nil, err
	}
	return &delayNode{
		Node:  def,
		delay: d,
	}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    "delay",
		Factory: &delayFactory{},
	})
}
