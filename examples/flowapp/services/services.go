package services

import (
	"github.com/zhiyunliu/glow-flow/examples/flowapp/services/flow"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/services/order"
	"github.com/zhiyunliu/glue/engine"
	"github.com/zhiyunliu/glue/server/api"
	"github.com/zhiyunliu/glue/server/mqc"
)

// BindAPI 注册接口
func BindAPI(server *api.Server) {
	flowGroup := server.Group("/flow") // 流程
	flowGroup.Handle("/run", flow.RunFlow, engine.MethodPost)
	flowGroup.Handle("/list", flow.ListFlow, engine.MethodGet)
	flowGroup.Handle("/reload", flow.ReloadFlow, engine.MethodPost)
}

// BindMQC 注册消息消费；队列与服务路径的对应关系在配置文件 servers.mqcserver.tasks 中配置
func BindMQC(server *mqc.Server) {
	server.Handle("/order/created", order.OrderCreated) // 下单事件 -> 履约流程
}
