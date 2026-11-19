// Package lua 基于 github.com/yuin/gopher-lua 的脚本引擎实现。
//
// 脚本中可用的扩展对象：
//   - ctx：当前调用的 glowflow.Context，支持 ctx:get(key)、ctx:set(key, value)、ctx:instance_id()
//   - global：Config.Properties 组成的表
//   - print：输出到 glowflow 日志
//
// 出于安全考虑，仅加载 base/table/string/math/coroutine 库及 os.time/os.clock/os.date/os.difftime，
// 不提供 io、package、require、module、debug、dofile、loadfile 等访问文件系统或进程的能力。
package lua

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	glua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/language"
	"github.com/zhiyunliu/glow-flow/scriptengine"
)

var _ scriptengine.Engine = (*LuaEngine)(nil)

const (
	GlobalKey = "global"
	CtxKey    = "ctx"
)

// LuaEngine gopher-lua 脚本引擎；LState 非并发安全，因此按调用从池中获取
type LuaEngine struct {
	vmPool     sync.Pool
	config     *Config
	initScript *glua.FunctionProto
	udfs       []luaUdf
}

// luaUdf 预处理后的用户自定义函数，proto 与 value 二选一
type luaUdf struct {
	name  string
	proto *glua.FunctionProto
	value any
}

// luaVmSlot 池化的 VM 槽位；err 记录 VM 初始化失败的原因
type luaVmSlot struct {
	vm  *glua.LState
	err error
}

// NewEngine 创建 Lua 脚本引擎；初始化脚本与用户自定义脚本在此预编译并试运行一次，以便尽早暴露错误
func NewEngine(config *Config) (scriptengine.Engine, error) {
	if config == nil {
		config = &Config{}
	}
	initScript, err := compileLua("init", config.InitScript)
	if err != nil {
		return nil, err
	}
	udfs, err := preCompileUdfs(config.UserdefinedFunction)
	if err != nil {
		return nil, err
	}

	engine := &LuaEngine{
		config:     config,
		initScript: initScript,
		udfs:       udfs,
	}
	first := engine.newSlot()
	if first.err != nil {
		return nil, first.err
	}
	engine.vmPool = sync.Pool{
		New: func() any {
			return engine.newSlot()
		},
	}
	engine.vmPool.Put(first)
	return engine, nil
}

func (g *LuaEngine) Proto() string {
	return Proto
}

// Execute 调用脚本中名为 functionName 的全局函数，返回其第一个返回值
func (g *LuaEngine) Execute(ctx glowflow.Context, functionName string, argumentList ...any) (out any, err error) {
	slot := g.vmPool.Get().(*luaVmSlot)
	if slot.err != nil {
		return nil, slot.err
	}
	vm := slot.vm

	// 发生 panic 或执行被中断时 VM 状态不可信，直接丢弃
	reusable := false
	defer func() {
		if obj := recover(); obj != nil {
			err = fmt.Errorf("%v", obj)
		}
		if !reusable {
			vm.Close()
			return
		}
		vm.SetGlobal(CtxKey, glua.LNil)
		vm.SetTop(0)
		g.vmPool.Put(slot)
	}()

	var parent context.Context = context.Background()
	if ctx != nil {
		parent = ctx
		vm.SetGlobal(CtxKey, newCtxUserData(vm, ctx))
	}

	fn, ok := vm.GetGlobal(functionName).(*glua.LFunction)
	if !ok {
		reusable = true
		return nil, errors.New(functionName + " is not a function")
	}

	params := make([]glua.LValue, len(argumentList))
	for i, v := range argumentList {
		if params[i], err = toLValue(vm, v); err != nil {
			reusable = true
			return nil, fmt.Errorf("argument #%d: %w", i+1, err)
		}
	}

	runCtx, cancel := g.withTimeout(parent)
	defer cancel()
	g.bindContext(vm, runCtx)
	err = vm.CallByParam(glua.P{Fn: fn, NRet: 1, Protect: true}, params...)
	vm.RemoveContext()
	if err != nil {
		reusable = runCtx.Err() == nil
		return nil, g.wrapError(parent, runCtx, err)
	}
	reusable = true

	ret := vm.Get(-1)
	vm.Pop(1)
	return fromLValue(ret)
}

func (g *LuaEngine) Stop() {
}

// withTimeout 未配置超时时直接返回 parent，使不可取消的 parent 无需绑定到 VM
func (g *LuaEngine) withTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if g.config.MaxExecTime <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, g.config.MaxExecTime)
}

// bindContext 仅在 ctx 可被取消时绑定，避免无谓的逐指令检查开销
func (g *LuaEngine) bindContext(vm *glua.LState, ctx context.Context) {
	if ctx.Done() != nil {
		vm.SetContext(ctx)
	}
}

// wrapError 被中断时返回可用 errors.Is 判断的 context 错误
func (g *LuaEngine) wrapError(parent context.Context, runCtx context.Context, err error) error {
	ctxErr := runCtx.Err()
	if ctxErr == nil {
		return err
	}
	if errors.Is(ctxErr, context.DeadlineExceeded) && parent.Err() == nil {
		return fmt.Errorf("execution timeout(%s): %w", g.config.MaxExecTime, ctxErr)
	}
	return fmt.Errorf("execution interrupted: %w", ctxErr)
}

func (g *LuaEngine) newSlot() *luaVmSlot {
	vm, err := g.newVm()
	if err != nil {
		glowflow.GetLogger(context.Background()).Errorf("lua vm error: %s", err.Error())
		if vm != nil {
			vm.Close()
		}
		return &luaVmSlot{err: err}
	}
	return &luaVmSlot{vm: vm}
}

// newVm 创建 VM：加载安全库 → 注入 FromVars/global → 执行用户自定义函数 → 执行初始化脚本
func (g *LuaEngine) newVm() (*glua.LState, error) {
	vm := glua.NewState(glua.Options{SkipOpenLibs: true})
	openSafeLibs(vm)
	registerCtxType(vm)
	vm.SetGlobal("print", vm.NewFunction(luaPrint))

	for k, v := range g.config.FromVars {
		lv, err := toLValue(vm, v)
		if err != nil {
			return vm, fmt.Errorf("set fromVar %s error: %w", k, err)
		}
		vm.SetGlobal(k, lv)
	}

	if len(g.config.Properties) != 0 {
		lv, err := toLValue(vm, g.config.Properties)
		if err != nil {
			return vm, fmt.Errorf("set global properties error: %w", err)
		}
		vm.SetGlobal(GlobalKey, lv)
	}

	for _, udf := range g.udfs {
		if udf.proto != nil {
			if err := g.runProto(vm, udf.proto); err != nil {
				return vm, fmt.Errorf("run lua script=%s error: %w", udf.name, err)
			}
			continue
		}
		lv, err := toLValue(vm, udf.value)
		if err != nil {
			return vm, fmt.Errorf("set userdefined function=%s error: %w", udf.name, err)
		}
		vm.SetGlobal(udf.name, lv)
	}

	if err := g.runProto(vm, g.initScript); err != nil {
		return vm, fmt.Errorf("run lua init script error: %w", err)
	}
	return vm, nil
}

func (g *LuaEngine) runProto(vm *glua.LState, proto *glua.FunctionProto) error {
	parent := context.Background()
	runCtx, cancel := g.withTimeout(parent)
	defer cancel()
	g.bindContext(vm, runCtx)
	defer vm.RemoveContext()

	vm.Push(vm.NewFunctionFromProto(proto))
	if err := vm.PCall(0, 0, nil); err != nil {
		return g.wrapError(parent, runCtx, err)
	}
	return nil
}

func compileLua(name string, source string) (*glua.FunctionProto, error) {
	chunk, err := parse.Parse(strings.NewReader(source), name)
	if err != nil {
		return nil, err
	}
	return glua.Compile(chunk, name)
}

// preCompileUdfs 按名称排序以保证执行顺序确定；非 Lua 类型的 Script 被忽略
func preCompileUdfs(functions map[string]any) ([]luaUdf, error) {
	names := make([]string, 0, len(functions))
	for name := range functions {
		names = append(names, name)
	}
	sort.Strings(names)

	udfs := make([]luaUdf, 0, len(names))
	for _, name := range names {
		switch v := functions[name].(type) {
		case string:
			proto, err := compileLua(name, v)
			if err != nil {
				return nil, err
			}
			udfs = append(udfs, luaUdf{name: name, proto: proto})
		case scriptengine.Script:
			if v.Type != language.Lua && v.Type != "" {
				continue
			}
			switch content := v.Content.(type) {
			case string:
				proto, err := compileLua(name, content)
				if err != nil {
					return nil, err
				}
				udfs = append(udfs, luaUdf{name: name, proto: proto})
			case *glua.FunctionProto:
				udfs = append(udfs, luaUdf{name: name, proto: content})
			default:
				return nil, fmt.Errorf("lua script=%s: unsupported content type %T", name, v.Content)
			}
		default:
			udfs = append(udfs, luaUdf{name: name, value: v})
		}
	}
	return udfs, nil
}

func openSafeLibs(vm *glua.LState) {
	libs := []struct {
		name string
		fn   glua.LGFunction
	}{
		{glua.BaseLibName, glua.OpenBase},
		{glua.TabLibName, glua.OpenTable},
		{glua.StringLibName, glua.OpenString},
		{glua.MathLibName, glua.OpenMath},
		{glua.CoroutineLibName, glua.OpenCoroutine},
		{glua.OsLibName, glua.OpenOs},
	}
	for _, lib := range libs {
		vm.Push(vm.NewFunction(lib.fn))
		vm.Push(glua.LString(lib.name))
		vm.Call(1, 0)
	}

	// base 库自带的 require/module 会从文件系统加载模块
	for _, name := range []string{"dofile", "loadfile", "require", "module"} {
		vm.SetGlobal(name, glua.LNil)
	}

	// os 仅保留时间相关函数
	if osLib, ok := vm.GetGlobal(glua.OsLibName).(*glua.LTable); ok {
		safeOs := vm.NewTable()
		for _, name := range []string{"clock", "date", "difftime", "time"} {
			safeOs.RawSetString(name, osLib.RawGetString(name))
		}
		vm.SetGlobal(glua.OsLibName, safeOs)
	}
}

func luaPrint(vm *glua.LState) int {
	top := vm.GetTop()
	parts := make([]string, top)
	for i := 1; i <= top; i++ {
		parts[i-1] = vm.ToStringMeta(vm.Get(i)).String()
	}
	glowflow.GetLogger(vm.Context()).Infof("%s", strings.Join(parts, "\t"))
	return 0
}

// wrapGoFunc 将任意 Go 函数包装为 Lua 函数
func wrapGoFunc(fn reflect.Value) glua.LGFunction {
	return func(vm *glua.LState) int {
		return callGoFunc(vm, fn)
	}
}
