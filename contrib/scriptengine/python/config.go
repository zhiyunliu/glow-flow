package python

import "time"

type Config struct {
	// MaxExecTime 单次初始化或函数调用的最长执行时间，<=0 表示不限制
	MaxExecTime time.Duration
	// MaxExecSteps 单次函数调用的最大执行步数（防止死循环耗尽 CPU），0 表示不限制
	MaxExecSteps uint64
	// Properties 以 properties 字典暴露给脚本（global 为 Python 保留字）
	Properties map[string]string
	// InitScript 引擎创建时执行一次的初始化脚本，通常在其中定义被调用的函数
	InitScript string
	// FromVars 注入为脚本全局变量
	FromVars map[string]any
	// UserdefinedFunction 用户自定义函数：
	//   - string / scriptengine.Script{Type: "python"|""}：Starlark 源码（或 *starlark.Program），在 InitScript 之前执行，
	//     其定义的全局名称对后续脚本可见
	//   - Go 函数：注册为同名内置函数
	//   - 其他值：转换后注册为同名全局变量
	UserdefinedFunction map[string]any
}
