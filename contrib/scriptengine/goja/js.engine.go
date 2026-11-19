package goja

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dop251/goja"
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/language"
	"github.com/zhiyunliu/glow-flow/scriptengine"
)

var _ scriptengine.Engine = (*GojaJsEngine)(nil)

const (
	GlobalKey = "global"
	CtxKey    = "$ctx"
)

// GojaJsEngine goja js engine
type GojaJsEngine struct {
	vmPool            sync.Pool
	config            *Config
	initScript        *goja.Program
	jsUdfProgramCache map[string]*goja.Program
}

// gojaVmSlot 池化的 VM 槽位；超时定时器不进槽位，避免迟到回调把中断锁进空闲 VM
type gojaVmSlot struct {
	vm *goja.Runtime
}

// NewGojaJsEngine Create a new instance of the JavaScript engine
func NewEngine(config *Config) (scriptengine.Engine, error) {
	program, err := goja.Compile("", config.InitScript, true)
	if err != nil {
		return nil, err
	}
	jsEngine := &GojaJsEngine{
		config:     config,
		initScript: program,
	}
	if err = jsEngine.preCompileJs(config); err != nil {
		return nil, err
	}
	jsEngine.vmPool = sync.Pool{
		New: func() any {
			vm := jsEngine.newVm(config)
			return &gojaVmSlot{vm: vm}
		},
	}
	return jsEngine, nil
}

func (g *GojaJsEngine) Proto() string {
	return Proto
}

// Execute Execute JavaScript script
func (g *GojaJsEngine) Execute(ctx glowflow.Context, functionName string, argumentList ...any) (out any, err error) {
	defer func() {
		if obj := recover(); obj != nil {
			err = fmt.Errorf("%s", obj)
		}
	}()

	slot := g.vmPool.Get().(*gojaVmSlot)
	vm := slot.vm
	defer func() {
		_ = vm.Set(CtxKey, nil)
		g.vmPool.Put(slot)
	}()

	if ctx != nil {
		vm.Set(CtxKey, ctx)
	}

	vm.ClearInterrupt()

	timer := g.startTimeout(vm)
	defer g.stopTimeout(timer)

	// Get function
	f, ok := goja.AssertFunction(vm.Get(functionName))
	if !ok {
		return nil, errors.New(functionName + " is not a function")
	}

	var params []goja.Value
	if len(argumentList) > 0 {
		params = make([]goja.Value, len(argumentList))
		for i, v := range argumentList {
			params[i] = vm.ToValue(v)
		}
	}

	// Execute function
	res, err := f(goja.Undefined(), params...)
	if err != nil {
		return nil, err
	}
	return res.Export(), nil
}

func (g *GojaJsEngine) Stop() {
}

func (g *GojaJsEngine) startTimeout(vm *goja.Runtime) *time.Timer {
	if g.config.MaxExecTime <= 0 {
		return nil
	}

	return time.AfterFunc(g.config.MaxExecTime, func() {
		vm.Interrupt("execution timeout")
	})
}

func (g *GojaJsEngine) stopTimeout(timer *time.Timer) {
	if timer != nil {
		timer.Stop()
	}
}

// NewVm new a js VM
func (g *GojaJsEngine) newVm(config *Config) *goja.Runtime {
	vm := goja.New()

	logger := glowflow.GetLogger(context.Background())

	if config.FromVars != nil {
		for k, v := range config.FromVars {
			if err := vm.Set(k, v); err != nil {
				logger.Errorf("set fromVar %s error: %s", k, err.Error())
			}
		}
	}

	if len(config.Properties) != 0 {
		if err := vm.Set(GlobalKey, config.Properties); err != nil {
			logger.Errorf("set global properties error: %s", err.Error())
		}
	}

	for k, v := range config.UserdefinedFunction {
		var err error
		if _, ok := v.(string); ok {
			if p, exists := g.jsUdfProgramCache[k]; exists {
				_, err = vm.RunProgram(p)
			}
		} else {
			err = vm.Set(k, v)
		}

		if err != nil {
			logger.Errorf("parse js script=%s error: %s", k, err.Error())
		}
	}

	// Run main script with timeout
	timer := g.startTimeout(vm)
	_, err := vm.RunProgram(g.initScript)
	g.stopTimeout(timer)

	if err != nil {
		logger.Errorf("js vm error: %s", err.Error())
	}
	return vm
}
func (g *GojaJsEngine) preCompileJs(config *Config) error {
	var jsUdfProgramCache = make(map[string]*goja.Program)
	for k, v := range config.UserdefinedFunction {
		if jsFuncStr, ok := v.(string); ok {
			if p, err := goja.Compile(k, jsFuncStr, true); err != nil {
				return err
			} else {
				jsUdfProgramCache[k] = p
			}
		} else if script, scriptOk := v.(scriptengine.Script); scriptOk {
			if script.Type == language.JavaScript || script.Type == "" {
				if c, ok := script.Content.(string); ok {
					if p, err := goja.Compile(k, c, true); err != nil {
						return err
					} else {
						jsUdfProgramCache[k] = p
					}
				} else if p, ok := script.Content.(*goja.Program); ok {
					jsUdfProgramCache[k] = p
				}
			}
		}
	}
	g.jsUdfProgramCache = jsUdfProgramCache
	return nil
}
