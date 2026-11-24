package transform

import (
	"encoding/json"

	glowflow "github.com/zhiyunliu/glow-flow"
	gojaengine "github.com/zhiyunliu/glow-flow/contrib/scriptengine/goja"
	"github.com/zhiyunliu/glow-flow/scriptengine"
)

const JsTransformNodeType = "jstransform"

// TransformFuncName 脚本转换的入口函数名
const TransformFuncName = "Transform"

// 用户脚本作为 __transform 的函数体，且紧跟函数头，使报错行号与用户脚本一致。
// 入口函数 Transform 将 JSON 字符串形式的消息数据解析为原生 JS 对象后再调用：
// goja 包装的 Go map/slice 上的 push 等操作不会反映到返回值中。
const (
	jsTransformFuncPrefix = "function __transform(msg, msgId) {"
	jsTransformFuncSuffix = "\n}\nfunction " + TransformFuncName +
		"(input, msgId, isJSON) { return __transform(isJSON ? JSON.parse(input) : input, msgId); }"
)

// JavaScript 转换
/*
使用 JavaScript(goja) 脚本转换消息数据(节点类型 jstransform)

extparams 配置：
  - script：脚本内容，必填。脚本作为函数体执行：function (msg, msgId) { <script> }，
    通过 return 返回新的消息数据，如 msg.total = msg.price * msg.qty; msg.tags.push("new"); return msg;
    未 return(或返回 null/undefined)时新的消息数据为 nil
  - timeout：单次执行超时时间，如 500ms 或整数秒，默认 2s，<=0 表示不限制

脚本中可用的变量：
  - msg：消息数据(结构体按 json tag 转换为对象)；为消息数据的副本，可直接修改后返回
  - msgId：消息ID
  - $ctx：流程上下文(glowflow.Context)，如 $ctx.GetInstanceID()

转换成功走 success 分支并输出新消息(不修改原消息)；脚本执行出错或超时走 failure 分支，消息原样传递。
*/
type nodeJavascript struct {
	glowflow.Node
	engine scriptengine.Engine
}

func (n *nodeJavascript) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	msgID := ""
	if msg != nil {
		msgID = msg.MsgId
	}
	input, isJSON := jsInput(msg)
	out, err := n.engine.Execute(ctx, TransformFuncName, input, msgID, isJSON)
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}
	return successResult(msg, out), nil
}

// jsInput 基础类型原样传入；map/slice/结构体等序列化为 JSON 字符串，由脚本解析为原生对象(同时实现了副本隔离)
func jsInput(msg *glowflow.Message) (any, bool) {
	data := scriptData(msg)
	switch data.(type) {
	case map[string]any, []any:
		raw, err := json.Marshal(data)
		if err != nil {
			return data, false
		}
		return string(raw), true
	}
	return data, false
}

type jsFactory struct{}

func (f *jsFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	script, err := getRequired(def, "script")
	if err != nil {
		return nil, err
	}
	timeout, err := getTimeout(def, "timeout")
	if err != nil {
		return nil, err
	}
	engine, err := gojaengine.NewEngine(&gojaengine.Config{
		InitScript:  jsTransformFuncPrefix + script + jsTransformFuncSuffix,
		MaxExecTime: timeout,
	})
	if err != nil {
		return nil, invalidConfig(def, "compile script: %v", err)
	}
	return &nodeJavascript{Node: def, engine: engine}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    JsTransformNodeType,
		Factory: &jsFactory{},
	})
}
