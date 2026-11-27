package order

import (
	"github.com/zhiyunliu/glow-flow/examples/flowapp/modules/order"
	"github.com/zhiyunliu/glue/context"
)

// OrderCreated 订单-消费下单事件，执行履约流程
func OrderCreated(ctx context.Context) interface{} {
	logger := ctx.Log()
	data := map[string]any{}
	if err := ctx.Request().Body().ScanTo(&data); err != nil {
		// 消息格式错误时重试无意义，记录日志后确认消息
		logger.Errorf("OrderCreated invalid message, body:%s, err:%+v", ctx.Request().Body().Bytes(), err)
		return nil
	}
	// 履约结果与错误已在模块中记录日志；流程执行失败不重试，避免重复预占库存
	_, _ = order.NewOrderer(logger).Fulfill(ctx.Context(), data)
	return nil
}
