package flow

import (
	"github.com/zhiyunliu/glow-flow/examples/flowapp/models"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/modules/flow"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/pkg/resp"
	"github.com/zhiyunliu/glue/context"
)

// RunFlow 流程-执行：返回流程结果与节点执行轨迹
func RunFlow(ctx context.Context) interface{} {
	logger := ctx.Log()
	params := models.RunFlowParams{}
	if err := ctx.Bind(&params); err != nil {
		logger.Errorf("RunFlow bind params failed, err:%+v", err)
		return resp.BindError(err)
	}
	result, err := flow.NewFlower(logger).Run(ctx.Context(), params.Chain, params.Data)
	if err != nil {
		return resp.Error(err)
	}
	return resp.Success(result)
}

// ListFlow 流程-已加载列表
func ListFlow(ctx context.Context) interface{} {
	list, err := flow.NewFlower(ctx.Log()).List(ctx.Context())
	if err != nil {
		return resp.Error(err)
	}
	return resp.Success(list)
}

// ReloadFlow 流程-重新加载流程定义
func ReloadFlow(ctx context.Context) interface{} {
	result, err := flow.NewFlower(ctx.Log()).Reload(ctx.Context())
	if err != nil {
		return resp.Error(err)
	}
	return resp.Success(result)
}
