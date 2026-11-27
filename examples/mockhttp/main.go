// mockhttp 模拟 http 外部服务，供样例流程中的 httpcall、dingtalk 节点调用
package main

import (
	"github.com/zhiyunliu/glue"
	"github.com/zhiyunliu/glue/engine"
	"github.com/zhiyunliu/glue/global"
	"github.com/zhiyunliu/glue/log"
	"github.com/zhiyunliu/glue/server/api"
)

func main() {
	global.AppName = "mockhttp"

	apiSrv := api.New("apiserver", api.WithServiceName(global.AppName), api.Log(log.WithRequest()))
	apiSrv.Handle("/api/member/profile", memberProfile, engine.MethodGet) // 会员信息(httpcall GET)
	apiSrv.Handle("/api/sms/send", smsSend, engine.MethodPost)            // 发货短信(httpcall POST)
	apiSrv.Handle("/robot/send", robotSend, engine.MethodPost)            // 钉钉群机器人 Webhook(dingtalk)
	apiSrv.Handle("/robot/messages", robotMessages, engine.MethodGet)     // 查看收到的钉钉消息

	app := glue.NewApp(glue.Server(apiSrv))
	_ = app.Start()
}
