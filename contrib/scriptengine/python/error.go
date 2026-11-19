package python

import "go.starlark.net/starlark"

// ScriptError 脚本运行错误，Error() 包含 Starlark 调用栈（文件:行:列）
type ScriptError struct {
	EvalError *starlark.EvalError
}

func (e *ScriptError) Error() string {
	return e.EvalError.Backtrace()
}

func (e *ScriptError) Unwrap() error {
	return e.EvalError
}
