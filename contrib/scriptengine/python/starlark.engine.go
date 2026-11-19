// Package python 基于 go.starlark.net 的脚本引擎实现。
//
// Starlark 是 Python 语法的一个方言（https://github.com/google/starlark-go/blob/master/doc/spec.md），
// 支持 def/lambda/列表推导/dict/list/tuple 等常用语法；本引擎额外开启了 while、set、递归、
// 顶层 if/for 与全局变量重新赋值。不支持 class、import、try/except 以及 Python 标准库。
//
// 脚本中可用的扩展对象：
//   - ctx：当前调用的 glowflow.Context，支持 ctx.get(key)、ctx.set(key, value)、ctx.instance_id()
//   - properties：Config.Properties 组成的字典（global 是 Starlark/Python 保留字，不能用作变量名）
//   - json / math / time：Starlark 官方扩展模块
//   - print：输出到 glowflow 日志
//
// 初始化完成后全部全局变量会被冻结（Starlark 语义），因此引擎可被多个 goroutine 并发调用；
// 函数内部不能修改全局的 list/dict，调用间的状态请使用 ctx 传递。
package python

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/language"
	"github.com/zhiyunliu/glow-flow/scriptengine"
	starlarkmath "go.starlark.net/lib/math"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkjson"
	"go.starlark.net/syntax"
)

var _ scriptengine.Engine = (*StarlarkEngine)(nil)

const (
	GlobalKey = "properties"
	CtxKey    = "ctx"

	ctxLocalKey = "glowflow.ctx"
)

// fileOptions 在 Starlark 默认方言基础上放开常用的 Python 语法
var fileOptions = &syntax.FileOptions{
	Set:             true,
	While:           true,
	TopLevelControl: true,
	GlobalReassign:  true,
	Recursion:       true,
}

// StarlarkEngine Starlark 脚本引擎；globals 在创建后冻结，可安全并发调用
type StarlarkEngine struct {
	config  *Config
	globals starlark.StringDict
}

// NewEngine 创建 Starlark 脚本引擎：注入 FromVars/global → 执行用户自定义函数 → 执行初始化脚本 → 冻结全局变量
func NewEngine(config *Config) (scriptengine.Engine, error) {
	if config == nil {
		config = &Config{}
	}
	engine := &StarlarkEngine{config: config}

	globals := starlark.StringDict{
		CtxKey: contextValue{},
		"json": starlarkjson.Module,
		"math": starlarkmath.Module,
		"time": starlarktime.Module,
	}

	for k, v := range config.FromVars {
		value, err := toStarlark(v)
		if err != nil {
			return nil, fmt.Errorf("set fromVar %s error: %w", k, err)
		}
		globals[k] = value
	}

	if len(config.Properties) != 0 {
		value, err := toStarlark(config.Properties)
		if err != nil {
			return nil, fmt.Errorf("set global properties error: %w", err)
		}
		globals[GlobalKey] = value
	}

	if err := engine.loadUdfs(globals); err != nil {
		return nil, err
	}

	if strings.TrimSpace(config.InitScript) != "" {
		if err := engine.exec("init", config.InitScript, globals); err != nil {
			return nil, fmt.Errorf("run python init script error: %w", err)
		}
	}

	globals.Freeze()
	engine.globals = globals
	return engine, nil
}

func (e *StarlarkEngine) Proto() string {
	return Proto
}

// Execute 调用脚本中名为 functionName 的全局函数
func (e *StarlarkEngine) Execute(ctx glowflow.Context, functionName string, argumentList ...any) (out any, err error) {
	defer func() {
		if obj := recover(); obj != nil {
			err = fmt.Errorf("%v", obj)
		}
	}()

	fn, ok := e.globals[functionName].(starlark.Callable)
	if !ok {
		return nil, errors.New(functionName + " is not a function")
	}

	args := make(starlark.Tuple, len(argumentList))
	for i, v := range argumentList {
		if args[i], err = toStarlark(v); err != nil {
			return nil, fmt.Errorf("argument #%d: %w", i+1, err)
		}
	}

	var parent context.Context = context.Background()
	if ctx != nil {
		parent = ctx
	}
	thread := e.newThread(ctx, functionName)

	var result starlark.Value
	err = e.run(parent, thread, func() (err error) {
		result, err = starlark.Call(thread, fn, args, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return fromStarlark(result)
}

func (e *StarlarkEngine) Stop() {
}

// loadUdfs 按名称排序依次加载用户自定义函数；非 Python 类型的 Script 被忽略
func (e *StarlarkEngine) loadUdfs(globals starlark.StringDict) error {
	names := make([]string, 0, len(e.config.UserdefinedFunction))
	for name := range e.config.UserdefinedFunction {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		switch v := e.config.UserdefinedFunction[name].(type) {
		case string:
			if err := e.exec(name, v, globals); err != nil {
				return fmt.Errorf("run python script=%s error: %w", name, err)
			}
		case scriptengine.Script:
			if v.Type != language.Python && v.Type != "" {
				continue
			}
			switch content := v.Content.(type) {
			case string, *starlark.Program:
				if err := e.exec(name, content, globals); err != nil {
					return fmt.Errorf("run python script=%s error: %w", name, err)
				}
			default:
				return fmt.Errorf("python script=%s: unsupported content type %T", name, v.Content)
			}
		default:
			value, err := e.udfValue(name, v)
			if err != nil {
				return fmt.Errorf("set userdefined function=%s error: %w", name, err)
			}
			globals[name] = value
		}
	}
	return nil
}

func (e *StarlarkEngine) udfValue(name string, v any) (starlark.Value, error) {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Func && !rv.IsNil() {
		if _, ok := v.(starlark.Value); !ok {
			return wrapGoFunc(name, rv), nil
		}
	}
	return toStarlark(v)
}

// exec 执行一段脚本（源码或已编译的 *starlark.Program），并把其定义的全局名称合并进 globals
func (e *StarlarkEngine) exec(name string, src any, globals starlark.StringDict) error {
	prog, ok := src.(*starlark.Program)
	if !ok {
		var err error
		if _, prog, err = starlark.SourceProgramOptions(fileOptions, name, src, globals.Has); err != nil {
			return err
		}
	}

	thread := e.newThread(nil, name)
	var module starlark.StringDict
	err := e.run(context.Background(), thread, func() (err error) {
		module, err = prog.Init(thread, globals)
		return err
	})
	if err != nil {
		return err
	}
	for k, v := range module {
		globals[k] = v
	}
	return nil
}

func (e *StarlarkEngine) newThread(ctx glowflow.Context, name string) *starlark.Thread {
	thread := &starlark.Thread{
		Name: name,
		Print: func(_ *starlark.Thread, msg string) {
			glowflow.GetLogger(ctx).Infof("%s", msg)
		},
	}
	if ctx != nil {
		thread.SetLocal(ctxLocalKey, ctx)
	}
	if e.config.MaxExecSteps > 0 {
		thread.SetMaxExecutionSteps(e.config.MaxExecSteps)
	}
	return thread
}

// run 在超时/父 context 取消时中断 thread
func (e *StarlarkEngine) run(parent context.Context, thread *starlark.Thread, fn func() error) error {
	runCtx, cancel := e.withTimeout(parent)
	defer cancel()
	stop := context.AfterFunc(runCtx, func() {
		thread.Cancel(runCtx.Err().Error())
	})
	defer stop()

	if err := fn(); err != nil {
		return e.wrapError(parent, runCtx, err)
	}
	return nil
}

// withTimeout 未配置超时时直接返回 parent
func (e *StarlarkEngine) withTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if e.config.MaxExecTime <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, e.config.MaxExecTime)
}

// wrapError 被中断时返回可用 errors.Is 判断的 context 错误；脚本错误附带 Starlark 调用栈
func (e *StarlarkEngine) wrapError(parent context.Context, runCtx context.Context, err error) error {
	if ctxErr := runCtx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) && parent.Err() == nil {
			return fmt.Errorf("execution timeout(%s): %w", e.config.MaxExecTime, ctxErr)
		}
		return fmt.Errorf("execution interrupted: %w", ctxErr)
	}
	var evalErr *starlark.EvalError
	if errors.As(err, &evalErr) {
		return &ScriptError{EvalError: evalErr}
	}
	return err
}
