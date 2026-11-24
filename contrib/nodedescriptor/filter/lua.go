package filter

import (
	glowflow "github.com/zhiyunliu/glow-flow"
	luaengine "github.com/zhiyunliu/glow-flow/contrib/scriptengine/lua"
	"github.com/zhiyunliu/glow-flow/scriptengine"
)

const LuaFilterNodeType = "luafilter"

// luaFilterFuncTemplate 用户脚本紧跟函数头，使报错行号与用户脚本一致
const luaFilterFuncTemplate = "function " + FilterFuncName + "(msg, msgId) "

// Lua 过滤器
/*
使用 Lua(gopher-lua) 脚本对消息进行布尔判定(节点类型 luafilter)

extparams 配置：
  - script：脚本内容，必填。脚本作为函数体执行：function Filter(msg, msgId) <script> end，
    需通过 return 返回布尔值，如 return msg.temperature > 50
  - timeout：单次执行超时时间，如 500ms 或秒数，默认 2s，<=0 表示不限制

脚本中可用的变量：
  - msg：消息数据(map/结构体转换为 table，结构体按 json tag)；为消息数据的副本，修改不会影响下游节点
  - msgId：消息ID
  - ctx：流程上下文，如 ctx:instance_id()、ctx:get(key)

出于安全考虑，脚本环境不提供 io、require、debug 等访问文件系统或进程的能力。

返回 true 走 true 分支，false/nil 或非布尔结果走 false 分支，消息原样传递；
脚本执行出错或超时走 failure 分支。
*/
type nodeLuaFilter struct {
	glowflow.Node
	engine scriptengine.Engine
}

func (n *nodeLuaFilter) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	out, err := n.engine.Execute(ctx, FilterFuncName, scriptData(msg), msgID(msg))
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}
	return boolResult(msg, out), nil
}

type luaFilterFactory struct{}

func (f *luaFilterFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	script, err := getScript(def, "script")
	if err != nil {
		return nil, err
	}
	timeout, err := getTimeout(def, "timeout")
	if err != nil {
		return nil, err
	}
	engine, err := luaengine.NewEngine(&luaengine.Config{
		InitScript:  luaFilterFuncTemplate + script + "\nend",
		MaxExecTime: timeout,
	})
	if err != nil {
		return nil, invalidConfig(def, "compile script: %v", err)
	}
	return &nodeLuaFilter{Node: def, engine: engine}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    LuaFilterNodeType,
		Factory: &luaFilterFactory{},
	})
}
