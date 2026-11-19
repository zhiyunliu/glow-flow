package lua

import "time"

type Config struct {
	// MaxExecTime 单次初始化或函数调用的最长执行时间，<=0 表示不限制
	MaxExecTime time.Duration
	// Properties 以 global 表暴露给脚本
	Properties map[string]string
	// InitScript 每个 VM 创建时执行的初始化脚本，通常在其中定义被调用的函数
	InitScript string
	// FromVars 注入为脚本全局变量
	FromVars map[string]any
	// UserdefinedFunction 用户自定义函数：
	//   - string / scriptengine.Script{Type: "lua"|""}：Lua 源码（或 *lua.FunctionProto），在 InitScript 之前执行
	//   - Go 函数（含 lua.LGFunction）：注册为同名全局函数
	//   - 其他值：转换后注册为同名全局变量
	UserdefinedFunction map[string]any
}
