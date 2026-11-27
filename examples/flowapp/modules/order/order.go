package order

import (
	"context"

	"github.com/zhiyunliu/glow-flow/examples/flowapp/config"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/models"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/modules/flow"
	"github.com/zhiyunliu/glue/log"
)

// Orderer 订单事件处理
type Orderer interface {
	// Fulfill 处理下单事件(order_create 流程中 mqp 节点发布)：执行履约流程
	Fulfill(ctx context.Context, order map[string]any) (models.RunResult, error)
}

type OrdererImpl struct {
	logger log.Logger
	flower flow.Flower
}

func NewOrderer(logger log.Logger) Orderer {
	return &OrdererImpl{logger: logger, flower: flow.NewFlower(logger)}
}

func (svc *OrdererImpl) Fulfill(ctx context.Context, order map[string]any) (models.RunResult, error) {
	chainID := config.Sys.FulfillChain
	result, err := svc.flower.Run(ctx, chainID, order)
	if err != nil {
		svc.logger.Errorf("fulfill order failed, chain:%s, order_no:%v, err:%+v", chainID, order["order_no"], err)
		return result, err
	}
	if result.Error != "" {
		svc.logger.Errorf("fulfill order failed, chain:%s, order_no:%v, instance:%s, err:%s", chainID, order["order_no"], result.InstanceID, result.Error)
		return result, nil
	}
	svc.logger.Infof("fulfill order completed, chain:%s, order_no:%v, instance:%s, nodes:%d, results:%v",
		chainID, order["order_no"], result.InstanceID, len(result.Trace), result.Results)
	return result, nil
}
