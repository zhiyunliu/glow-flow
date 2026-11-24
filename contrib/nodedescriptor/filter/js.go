package filter

import (
	glowflow "github.com/zhiyunliu/glow-flow"
	gojaengine "github.com/zhiyunliu/glow-flow/contrib/scriptengine/goja"
	"github.com/zhiyunliu/glow-flow/scriptengine"
)

const JsFilterNodeType = "jsfilter"

// jsFilterFuncTemplate 用户脚本紧跟函数头，使报错行号与用户脚本一致
const jsFilterFuncTemplate = "function " + FilterFuncName + "(msg, msgId) {"

// JavaScript 过滤器
/*
使用 JavaScript(goja) 脚本对消息进行布尔判定(节点类型 jsfilter)

extparams 配置：
  - script：脚本内容，必填。脚本作为函数体执行：function Filter(msg, msgId) { <script> }，
    需通过 return 返回布尔值，如 return msg.temperature > 50;
  - timeout：单次执行超时时间，如 500ms 或秒数，默认 2s，<=0 表示不限制

脚本中可用的变量：
  - msg：消息数据(结构体按 json tag 转换为对象)；为消息数据的副本，修改不会影响下游节点
  - msgId：消息ID
  - $ctx：流程上下文(glowflow.Context)，如 $ctx.GetInstanceID()

返回 true 走 true 分支，false 或非布尔结果走 false 分支，消息原样传递；
脚本执行出错或超时走 failure 分支。
*/
type nodeJsFilter struct {
	glowflow.Node
	engine scriptengine.Engine
}

func (n *nodeJsFilter) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	out, err := n.engine.Execute(ctx, FilterFuncName, scriptData(msg), msgID(msg))
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}
	return boolResult(msg, out), nil
}

type jsFilterFactory struct{}

func (f *jsFilterFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	script, err := getScript(def, "script")
	if err != nil {
		return nil, err
	}
	timeout, err := getTimeout(def, "timeout")
	if err != nil {
		return nil, err
	}
	engine, err := gojaengine.NewEngine(&gojaengine.Config{
		InitScript:  jsFilterFuncTemplate + script + "\n}",
		MaxExecTime: timeout,
	})
	if err != nil {
		return nil, invalidConfig(def, "compile script: %v", err)
	}
	return &nodeJsFilter{Node: def, engine: engine}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    JsFilterNodeType,
		Factory: &jsFilterFactory{},
	})
}
