// Package nodes 样例的自定义节点，通过 glowflow.DefaultRegistry.RegisterCall 注册，
// 流程中以 {"type": "custom", "extparams": {"nodetype": "<名称>"}} 使用。
package nodes

import (
	"encoding/json"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/modules/flow/trace"
)

// CollectCall 收集流程结果
const CollectCall = "example.collect"

func init() {
	if err := glowflow.DefaultRegistry.RegisterCall(CollectCall, collect); err != nil {
		panic(err)
	}
}

// collect 将消息数据作为流程结果记录到本次执行中并输出日志，消息原样传递
func collect(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	var data any
	if msg != nil {
		data = msg.Data
	}
	if rec := trace.FromContext(ctx); rec != nil {
		rec.Collect(data)
	}
	raw, _ := json.Marshal(data)
	glowflow.GetLogger(ctx).Infof("flow instance %s result: %s", ctx.GetInstanceID(), raw)
	return glowflow.ExecuteResult{RelationType: relationtype.Success, Msg: msg}, nil
}
