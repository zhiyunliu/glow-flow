// mockrpc 模拟 rpc(grpc) 外部服务，供样例流程中的 rpccall 节点调用
package main

import (
	"github.com/zhiyunliu/glue"
	"github.com/zhiyunliu/glue/global"
	"github.com/zhiyunliu/glue/log"
	"github.com/zhiyunliu/glue/server/rpc"
)

func main() {
	global.AppName = "mockrpc"

	rpcSrv := rpc.New("rpcserver", rpc.WithServiceName(global.AppName), rpc.Log(log.WithRequest()))
	rpcSrv.Handle("/risk/evaluate", riskEvaluate) // 风控评估
	rpcSrv.Handle("/stock/reserve", stockReserve) // 库存预占(按商品)
	rpcSrv.Handle("/points/add", pointsAdd)       // 积分发放

	app := glue.NewApp(glue.Server(rpcSrv))
	_ = app.Start()
}
