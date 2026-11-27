// flowapp 基于 glue 框架使用 glow-flow 的样例服务：
//   - apiserver：按流程ID执行流程并返回结果与节点执行轨迹
//   - mqcserver：消费 order_create 流程中 mqp 节点发布的下单事件，执行履约流程
package main

import (
	"context"

	"github.com/zhiyunliu/glow-flow/examples/flowapp/config"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/modules/flow"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/services"
	"github.com/zhiyunliu/glue"
	"github.com/zhiyunliu/glue/global"
	"github.com/zhiyunliu/glue/log"
	"github.com/zhiyunliu/glue/server/api"
	"github.com/zhiyunliu/glue/server/mqc"

	_ "github.com/zhiyunliu/glue/contrib/xhttp/http" // httpcall 节点使用的 http 客户端
	_ "github.com/zhiyunliu/glue/contrib/xrpc/grpc"  // rpccall 节点使用的 grpc 客户端
	_ "github.com/zhiyunliu/queue-redis"             // mqp 节点与 mqcserver 使用的 redis 队列
	_ "github.com/zhiyunliu/xdb-mssql"               // xdb 节点使用的 SQL Server 驱动

	// 注册内置节点与样例的自定义节点
	_ "github.com/zhiyunliu/glow-flow/contrib/nodedescriptor/action"
	_ "github.com/zhiyunliu/glow-flow/contrib/nodedescriptor/filter"
	_ "github.com/zhiyunliu/glow-flow/contrib/nodedescriptor/logical"
	_ "github.com/zhiyunliu/glow-flow/contrib/nodedescriptor/transform"
	_ "github.com/zhiyunliu/glow-flow/examples/flowapp/modules/flow/nodes"
)

func main() {
	global.AppName = "glowflow-example"

	apiSrv := api.New("apiserver", api.WithServiceName(global.AppName), api.Log(log.WithRequest()))
	services.BindAPI(apiSrv)

	mqcSrv := mqc.New("mqcserver")
	services.BindMQC(mqcSrv)

	app := glue.NewApp(glue.Server(apiSrv, mqcSrv), glue.StartingHook(func(ctx context.Context) error {
		if err := config.Load(global.Config); err != nil {
			return err
		}
		// 流程定义从工作目录下的 glowflow/*.json 加载
		return flow.Init(ctx, log.DefaultLogger)
	}))
	_ = app.Start()
}
