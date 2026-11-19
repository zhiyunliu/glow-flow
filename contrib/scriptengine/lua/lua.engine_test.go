package lua

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	glua "github.com/yuin/gopher-lua"
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/language"
	"github.com/zhiyunliu/glow-flow/scriptengine"
	"github.com/zhiyunliu/glue/config"
	_ "github.com/zhiyunliu/xbinding/binding"
)

func newTestEngine(t *testing.T, cfg *Config) scriptengine.Engine {
	t.Helper()
	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("new lua engine: %v", err)
	}
	t.Cleanup(engine.Stop)
	return engine
}

func TestLuaEngineExecuteConvertsValues(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: `
function echo(n, s, b, list, obj)
	return {
		n = n + 1, s = s .. "!", b = not b,
		first = list[1], count = #list, name = obj.name,
		nested = { 1, 2, 3 }, f = 1.5, empty = {},
	}
end`})

	out, err := engine.Execute(nil, "echo", 41, "hi", true, []string{"a", "b"}, map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := map[string]any{
		"n": int64(42), "s": "hi!", "b": false,
		"first": "a", "count": int64(2), "name": "x",
		"nested": []any{int64(1), int64(2), int64(3)}, "f": 1.5, "empty": map[string]any{},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %#v, want %#v", out, want)
	}
}

func TestLuaEngineGlobalsAndUserdefinedFunctions(t *testing.T) {
	engine := newTestEngine(t, &Config{
		FromVars:   map[string]any{"factor": 3},
		Properties: map[string]string{"env": "prod"},
		UserdefinedFunction: map[string]any{
			"helpers": "function double(x) return x * 2 end",
			"script":  scriptengine.Script{Type: language.Lua, Content: "function triple(x) return x * 3 end"},
			"jsonly":  scriptengine.Script{Type: language.JavaScript, Content: "function bad( {"},
			"add":     func(a, b int) int { return a + b },
			"fail":    func(msg string) (string, error) { return "", errors.New(msg) },
			"upper": glua.LGFunction(func(vm *glua.LState) int {
				vm.Push(glua.LString(strings.ToUpper(vm.CheckString(1))))
				return 1
			}),
		},
		InitScript: `
function run(x)
	return { double = double(x), triple = triple(x), add = add(x, factor), env = global.env, upper = upper("ab") }
end
function callFail() return fail("boom") end`,
	})

	out, err := engine.Execute(nil, "run", 2)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}
	want := map[string]any{"double": int64(4), "triple": int64(6), "add": int64(5), "env": "prod", "upper": "AB"}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %#v, want %#v", out, want)
	}

	if _, err := engine.Execute(nil, "callFail"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("callFail error = %v, want boom", err)
	}
}

func TestLuaEngineContextAccess(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: `
function useCtx(v)
	local old = ctx:get("k")
	ctx:set("k2", v)
	return { old = old, id = ctx:instance_id() }
end
function hasCtx() return ctx ~= nil end`})

	flowCtx := glowflow.NewContext(context.Background(), "inst-1")
	flowCtx.Set("k", "v")
	out, err := engine.Execute(flowCtx, "useCtx", 7)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if want := map[string]any{"old": "v", "id": "inst-1"}; !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %#v, want %#v", out, want)
	}
	if got := flowCtx.Get("k2"); got != int64(7) {
		t.Fatalf("ctx k2 = %#v, want 7", got)
	}

	// ctx 不应泄漏到下一次调用
	if out, err := engine.Execute(nil, "hasCtx"); err != nil || out != false {
		t.Fatalf("hasCtx = %#v, %v; want false", out, err)
	}
}

func TestLuaEngineTimeoutInterruptsAndEngineRecovers(t *testing.T) {
	engine := newTestEngine(t, &Config{
		MaxExecTime: 50 * time.Millisecond,
		InitScript: `
function loop() while true do end end
function ok() return 1 end`,
	})

	start := time.Now()
	_, err := engine.Execute(nil, "loop")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("loop error = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}

	if out, err := engine.Execute(nil, "ok"); err != nil || out != int64(1) {
		t.Fatalf("ok after timeout = %#v, %v", out, err)
	}
}

func TestLuaEngineParentContextCancel(t *testing.T) {
	for _, maxExecTime := range []time.Duration{0, 5 * time.Second} {
		t.Run(maxExecTime.String(), func(t *testing.T) {
			engine := newTestEngine(t, &Config{
				MaxExecTime: maxExecTime,
				InitScript: `
function loop() while true do end end
function ok() return 1 end`,
			})
			parent, cancel := context.WithCancel(context.Background())
			time.AfterFunc(20*time.Millisecond, cancel)

			_, err := engine.Execute(glowflow.NewContext(parent, "inst-1"), "loop")
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("loop error = %v, want Canceled", err)
			}
			if out, err := engine.Execute(nil, "ok"); err != nil || out != int64(1) {
				t.Fatalf("ok after cancel = %#v, %v", out, err)
			}
		})
	}
}

func TestLuaEngineErrors(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: `
function bad() error("oops") end
function kind(x) return type(x) end
function identity(x) return x end`})

	if _, err := engine.Execute(nil, "missing"); err == nil || !strings.Contains(err.Error(), "is not a function") {
		t.Fatalf("missing error = %v", err)
	}
	if _, err := engine.Execute(nil, "bad"); err == nil || !strings.Contains(err.Error(), "oops") {
		t.Fatalf("bad error = %v", err)
	}

	// 无法映射的 Go 类型以 userdata 传入，并可原样取回
	ch := make(chan int)
	if out, err := engine.Execute(nil, "kind", ch); err != nil || out != "userdata" {
		t.Fatalf("kind(chan) = %#v, %v", out, err)
	}
	if out, err := engine.Execute(nil, "identity", ch); err != nil || out != ch {
		t.Fatalf("identity(chan) = %#v, %v", out, err)
	}

	if _, err := NewEngine(&Config{InitScript: "function ("}); err == nil {
		t.Fatalf("syntax error in init script should fail")
	}
	if _, err := NewEngine(&Config{InitScript: `error("init failed")`}); err == nil || !strings.Contains(err.Error(), "init failed") {
		t.Fatalf("runtime error in init script = %v", err)
	}
	if _, err := NewEngine(&Config{UserdefinedFunction: map[string]any{"x": scriptengine.Script{Content: 1}}}); err == nil {
		t.Fatalf("unsupported script content should fail")
	}
}

func TestLuaEngineSandbox(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: `
function probe()
	return {
		io = io == nil, execute = os.execute == nil, remove = os.remove == nil,
		dofile = dofile == nil, loadfile = loadfile == nil, require = require == nil,
		time = type(os.time()),
	}
end`})

	out, err := engine.Execute(nil, "probe")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := map[string]any{
		"io": true, "execute": true, "remove": true,
		"dofile": true, "loadfile": true, "require": true,
		"time": "number",
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %#v, want %#v", out, want)
	}
}

func TestLuaEngineConcurrentExecute(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: `function add(a, b) return a + b end`})

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				out, err := engine.Execute(nil, "add", g, i)
				if err != nil {
					errs <- err
					return
				}
				if out != int64(g+i) {
					errs <- errors.New("unexpected result")
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestLuaEngineResolverRegistered(t *testing.T) {
	cfg := config.New()
	if err := cfg.Source(config.NewStrSource(`{"lua":{"InitScript":"function f() return 'ok' end"}}`)); err != nil {
		t.Fatalf("load config: %v", err)
	}
	engine, err := scriptengine.NewEngine(Proto, cfg.Get("lua"))
	if err != nil {
		t.Fatalf("resolve engine: %v", err)
	}
	if engine.Proto() != Proto {
		t.Fatalf("proto = %s", engine.Proto())
	}
	if out, err := engine.Execute(nil, "f"); err != nil || out != "ok" {
		t.Fatalf("f = %#v, %v", out, err)
	}
}
