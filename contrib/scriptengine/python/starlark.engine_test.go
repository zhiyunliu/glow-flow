package python

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/language"
	"github.com/zhiyunliu/glow-flow/scriptengine"
	"github.com/zhiyunliu/glue/config"
	_ "github.com/zhiyunliu/xbinding/binding"
	"go.starlark.net/starlark"
)

func newTestEngine(t *testing.T, cfg *Config) scriptengine.Engine {
	t.Helper()
	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("new python engine: %v", err)
	}
	t.Cleanup(engine.Stop)
	return engine
}

func TestPythonEngineExecuteConvertsValues(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: `
def echo(n, s, b, items, obj):
    return {
        "n": n + 1, "s": s + "!", "b": not b,
        "first": items[0], "count": len(items), "name": obj["name"],
        "nested": [1, 2, (3, 4)], "f": 1.5, "none": None, "empty": {},
    }
`})

	out, err := engine.Execute(nil, "echo", 41, "hi", true, []string{"a", "b"}, map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := map[string]any{
		"n": int64(42), "s": "hi!", "b": false,
		"first": "a", "count": int64(2), "name": "x",
		"nested": []any{int64(1), int64(2), []any{int64(3), int64(4)}}, "f": 1.5, "none": nil, "empty": map[string]any{},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %#v, want %#v", out, want)
	}
}

func TestPythonEngineStructArgumentUsesJSONTags(t *testing.T) {
	type order struct {
		OrderNo string `json:"order_no"`
		Amount  int    `json:"amount"`
	}
	engine := newTestEngine(t, &Config{InitScript: `
def describe(o):
    return "%s:%d" % (o["order_no"], o["amount"])
`})
	out, err := engine.Execute(nil, "describe", &order{OrderNo: "A1", Amount: 3})
	if err != nil || out != "A1:3" {
		t.Fatalf("describe = %#v, %v", out, err)
	}
}

func TestPythonEngineGlobalsAndUserdefinedFunctions(t *testing.T) {
	type pair struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	engine := newTestEngine(t, &Config{
		FromVars:   map[string]any{"factor": 3},
		Properties: map[string]string{"env": "prod"},
		UserdefinedFunction: map[string]any{
			"helpers": "def double(x):\n    return x * 2\n",
			"script":  scriptengine.Script{Type: language.Python, Content: "def triple(x):\n    return double(x) + x\n"},
			"jsonly":  scriptengine.Script{Type: language.JavaScript, Content: "function bad( {"},
			"add":     func(a, b int) int { return a + b },
			"sumPair": func(p pair) int { return p.A + p.B },
			"divmod":  func(a, b int) (int, int) { return a / b, a % b },
			"fail":    func(msg string) (string, error) { return "", errors.New(msg) },
		},
		InitScript: `
def run(x):
    return {
        "double": double(x), "triple": triple(x), "add": add(x, factor),
        "pair": sumPair({"a": x, "b": 10}), "divmod": divmod(7, x), "env": properties["env"],
    }

def callFail():
    return fail("boom")
`,
	})

	out, err := engine.Execute(nil, "run", 2)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}
	want := map[string]any{
		"double": int64(4), "triple": int64(6), "add": int64(5),
		"pair": int64(12), "divmod": []any{int64(3), int64(1)}, "env": "prod",
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %#v, want %#v", out, want)
	}

	if _, err := engine.Execute(nil, "callFail"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("callFail error = %v, want boom", err)
	}
}

func TestPythonEngineContextAccess(t *testing.T) {
	var seen glowflow.Context
	engine := newTestEngine(t, &Config{
		UserdefinedFunction: map[string]any{
			"capture": func(c glowflow.Context) string {
				seen = c
				return c.GetInstanceID()
			},
		},
		InitScript: `
def useCtx(v):
    old = ctx.get("k")
    ctx.set("k2", v)
    return {"old": old, "id": ctx.instance_id(), "captured": capture(ctx)}

def noCtx():
    return ctx.get("k")
`,
	})

	flowCtx := glowflow.NewContext(context.Background(), "inst-1")
	flowCtx.Set("k", "v")
	out, err := engine.Execute(flowCtx, "useCtx", [3]int{1, 2, 3})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if want := map[string]any{"old": "v", "id": "inst-1", "captured": "inst-1"}; !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %#v, want %#v", out, want)
	}
	if got := flowCtx.Get("k2"); !reflect.DeepEqual(got, []any{int64(1), int64(2), int64(3)}) {
		t.Fatalf("ctx k2 = %#v", got)
	}
	if seen != flowCtx {
		t.Fatalf("Go function did not receive the call context")
	}

	if _, err := engine.Execute(nil, "noCtx"); err == nil || !strings.Contains(err.Error(), "ctx is not available") {
		t.Fatalf("noCtx error = %v", err)
	}
}

func TestPythonEngineDialectFeatures(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: `
def fact(n):
    return 1 if n <= 1 else n * fact(n - 1)

def count(n):
    i = 0
    while i < n:
        i += 1
    return i

def features():
    return {
        "fact": fact(10), "while": count(5), "set": len(set([1, 1, 2])),
        "json": json.encode({"a": 1}), "sqrt": math.sqrt(16),
        "comp": [x * x for x in range(4) if x % 2 == 0],
    }
`})

	out, err := engine.Execute(nil, "features")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := map[string]any{
		"fact": int64(3628800), "while": int64(5), "set": int64(2),
		"json": `{"a":1}`, "sqrt": 4.0, "comp": []any{int64(0), int64(4)},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %#v, want %#v", out, want)
	}
}

func TestPythonEngineGlobalsAreFrozen(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: `
CACHE = []

def push():
    CACHE.append(1)
`})
	if _, err := engine.Execute(nil, "push"); err == nil || !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("push error = %v, want frozen list error", err)
	}
}

func TestPythonEngineTimeoutAndStepLimit(t *testing.T) {
	const script = `
def loop():
    while True:
        pass

def ok():
    return 1
`
	engine := newTestEngine(t, &Config{MaxExecTime: 50 * time.Millisecond, InitScript: script})

	start := time.Now()
	if _, err := engine.Execute(nil, "loop"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("loop error = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
	if out, err := engine.Execute(nil, "ok"); err != nil || out != int64(1) {
		t.Fatalf("ok after timeout = %#v, %v", out, err)
	}

	stepped := newTestEngine(t, &Config{MaxExecSteps: 10000, InitScript: script})
	if _, err := stepped.Execute(nil, "loop"); err == nil || !strings.Contains(err.Error(), "too many steps") {
		t.Fatalf("step-limited loop error = %v", err)
	}
}

func TestPythonEngineParentContextCancel(t *testing.T) {
	for _, maxExecTime := range []time.Duration{0, 5 * time.Second} {
		t.Run(maxExecTime.String(), func(t *testing.T) {
			engine := newTestEngine(t, &Config{
				MaxExecTime: maxExecTime,
				InitScript:  "def loop():\n    while True:\n        pass\n",
			})
			parent, cancel := context.WithCancel(context.Background())
			time.AfterFunc(20*time.Millisecond, cancel)

			if _, err := engine.Execute(glowflow.NewContext(parent, "inst-1"), "loop"); !errors.Is(err, context.Canceled) {
				t.Fatalf("loop error = %v, want Canceled", err)
			}
		})
	}
}

func TestPythonEngineErrors(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: `
def bad():
    return 1 // 0
`})

	if _, err := engine.Execute(nil, "missing"); err == nil || !strings.Contains(err.Error(), "is not a function") {
		t.Fatalf("missing error = %v", err)
	}

	_, err := engine.Execute(nil, "bad")
	var scriptErr *ScriptError
	if !errors.As(err, &scriptErr) {
		t.Fatalf("bad error = %T %v, want *ScriptError", err, err)
	}
	var evalErr *starlark.EvalError
	if !errors.As(err, &evalErr) {
		t.Fatalf("bad error should unwrap to *starlark.EvalError")
	}
	if msg := err.Error(); !strings.Contains(msg, "init:3") || !strings.Contains(msg, "division by zero") {
		t.Fatalf("bad error should include position and cause, got:\n%s", msg)
	}

	if _, err := engine.Execute(nil, "bad", make(chan int)); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("unsupported argument error = %v", err)
	}

	if _, err := NewEngine(&Config{InitScript: "def ("}); err == nil {
		t.Fatalf("syntax error in init script should fail")
	}
	if _, err := NewEngine(&Config{InitScript: "fail('init failed')"}); err == nil {
		t.Fatalf("undefined name in init script should fail")
	}
	if _, err := NewEngine(&Config{UserdefinedFunction: map[string]any{"x": scriptengine.Script{Content: 1}}}); err == nil {
		t.Fatalf("unsupported script content should fail")
	}
}

func TestPythonEngineConcurrentExecute(t *testing.T) {
	engine := newTestEngine(t, &Config{InitScript: "def add(a, b):\n    return a + b\n"})

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

func TestPythonEngineResolverRegistered(t *testing.T) {
	cfg := config.New()
	if err := cfg.Source(config.NewStrSource(`{"python":{"InitScript":"def f():\n    return 'ok'\n"}}`)); err != nil {
		t.Fatalf("load config: %v", err)
	}
	engine, err := scriptengine.NewEngine(Proto, cfg.Get("python"))
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
