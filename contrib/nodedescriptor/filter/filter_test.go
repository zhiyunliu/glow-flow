package filter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
	flowerrors "github.com/zhiyunliu/glow-flow/errors"
	"github.com/zhiyunliu/golibs/xtypes"
)

type sensor struct {
	DeviceID    string  `json:"device_id"`
	Temperature float64 `json:"temperature"`
}

func newTestContext() glowflow.Context {
	return glowflow.NewContext(context.Background(), "inst-1")
}

func tryBuild(nodeType string, ext xtypes.XMap) (glowflow.CompiledNode, error) {
	desc, ok := glowflow.DefaultRegistry.GetDescriptor(nodeType)
	if !ok {
		return nil, errors.New("node type is not registered: " + nodeType)
	}
	return desc.Factory.Build(glowflow.NodeDefinition{ID: nodeType + "-1", NodeType: nodeType, NodeName: "过滤", ExtParams: ext})
}

func build(t *testing.T, nodeType string, ext xtypes.XMap) glowflow.CompiledNode {
	t.Helper()
	node, err := tryBuild(nodeType, ext)
	if err != nil {
		t.Fatalf("build %s: %v", nodeType, err)
	}
	return node
}

// relation 执行节点并返回分支；校验执行异常不返回 error 且消息原样传递
func relation(t *testing.T, node glowflow.CompiledNode, msg *glowflow.Message) string {
	t.Helper()
	result, err := node.Execute(newTestContext(), msg)
	if err != nil || result.Error != nil {
		t.Fatalf("execute: err=%v result.Error=%v; failures should use the failure relation", err, result.Error)
	}
	if result.Msg != msg {
		t.Fatalf("filter should pass the original message through")
	}
	return result.RelationType
}

// filterCase 同一判定逻辑在三种过滤器中的写法
type filterCase struct {
	nodeType string
	key      string
	pass     string // 判定 temperature > 50
	notBool  string // 返回非布尔值
	broken   string // 运行时出错
	mutate   string // 修改 msg 后返回 true
	ctxCheck string // 校验上下文及消息ID
	loop     string // 死循环
	syntax   string // 第 2 行语法错误
}

var filterCases = []filterCase{
	{
		nodeType: ExprFilterNodeType, key: "expr",
		pass:     "msg.temperature > 50",
		notBool:  "msg.temperature",
		broken:   "msg.missing > 1",
		ctxCheck: `instanceid == "inst-1" && msgid == "msg-1" && ts > 0`,
		syntax:   "msg.temperature >\n> 50",
	},
	{
		nodeType: JsFilterNodeType, key: "script",
		pass:     "return msg.temperature > 50;",
		notBool:  "return msg.temperature;",
		broken:   "return msg.missing.value > 1;",
		mutate:   "msg.temperature = 1; msg.extra = true;\nreturn true;",
		ctxCheck: `return $ctx.GetInstanceID() === "inst-1" && msgId === "msg-1";`,
		loop:     "while (true) {}",
		syntax:   "var a = 1;\nreturn a +;",
	},
	{
		nodeType: LuaFilterNodeType, key: "script",
		pass:     "return msg.temperature > 50",
		notBool:  "return msg.temperature",
		broken:   "return msg.missing > 1",
		mutate:   "msg.temperature = 1\nmsg.extra = true\nreturn true",
		ctxCheck: `return ctx:instance_id() == "inst-1" and msgId == "msg-1"`,
		loop:     "while true do end",
		syntax:   "local a = 1\nreturn a +",
	},
}

func TestFilterRouting(t *testing.T) {
	for _, tc := range filterCases {
		t.Run(tc.nodeType, func(t *testing.T) {
			node := build(t, tc.nodeType, xtypes.XMap{tc.key: tc.pass})
			for _, item := range []struct {
				name string
				data any
				want string
			}{
				{"map hot", map[string]any{"temperature": 60}, relationtype.True},
				{"map cold", map[string]any{"temperature": 20.5}, relationtype.False},
				{"xmap", xtypes.XMap{"temperature": float64(51)}, relationtype.True},
				{"struct uses json tag", &sensor{DeviceID: "d1", Temperature: 70}, relationtype.True},
				{"struct value", sensor{Temperature: 10}, relationtype.False},
			} {
				if got := relation(t, node, glowflow.NewMessage("msg-1", item.data)); got != item.want {
					t.Errorf("%s: relation = %q, want %q", item.name, got, item.want)
				}
			}

			notBool := build(t, tc.nodeType, xtypes.XMap{tc.key: tc.notBool})
			if got := relation(t, notBool, glowflow.NewMessage("msg-1", map[string]any{"temperature": 60})); got != relationtype.False {
				t.Errorf("non-bool result: relation = %q, want false", got)
			}

			broken := build(t, tc.nodeType, xtypes.XMap{tc.key: tc.broken})
			if got := relation(t, broken, glowflow.NewMessage("msg-1", map[string]any{"temperature": 60})); got != relationtype.Failure {
				t.Errorf("runtime error: relation = %q, want failure", got)
			}

			ctxNode := build(t, tc.nodeType, xtypes.XMap{tc.key: tc.ctxCheck})
			if got := relation(t, ctxNode, glowflow.NewMessage("msg-1", "text")); got != relationtype.True {
				t.Errorf("context variables: relation = %q, want true", got)
			}
		})
	}
}

func TestScriptFilterDoesNotMutateMessage(t *testing.T) {
	for _, tc := range filterCases {
		if tc.mutate == "" {
			continue
		}
		t.Run(tc.nodeType, func(t *testing.T) {
			node := build(t, tc.nodeType, xtypes.XMap{tc.key: tc.mutate})
			data := map[string]any{"temperature": 60, "tags": []any{"a"}}
			if got := relation(t, node, glowflow.NewMessage("msg-1", data)); got != relationtype.True {
				t.Fatalf("relation = %q, want true", got)
			}
			want := map[string]any{"temperature": 60, "tags": []any{"a"}}
			if !reflect.DeepEqual(data, want) {
				t.Fatalf("message data mutated by script: %#v", data)
			}
		})
	}
}

func TestScriptFilterTimeout(t *testing.T) {
	for _, tc := range filterCases {
		if tc.loop == "" {
			continue
		}
		t.Run(tc.nodeType, func(t *testing.T) {
			node := build(t, tc.nodeType, xtypes.XMap{tc.key: tc.loop, "timeout": "100ms"})
			msg := glowflow.NewMessage("msg-1", nil)
			if got := relation(t, node, msg); got != relationtype.Failure {
				t.Fatalf("relation = %q, want failure on timeout", got)
			}
			// 超时后引擎仍可正常使用
			ok := build(t, tc.nodeType, xtypes.XMap{tc.key: "return true", "timeout": "100ms"})
			if got := relation(t, ok, msg); got != relationtype.True {
				t.Fatalf("relation = %q, want true", got)
			}
		})
	}
}

func TestFilterConcurrent(t *testing.T) {
	for _, tc := range filterCases {
		t.Run(tc.nodeType, func(t *testing.T) {
			node := build(t, tc.nodeType, xtypes.XMap{tc.key: tc.pass})
			var wg sync.WaitGroup
			errs := make(chan error, 64)
			for i := 0; i < 64; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					want := relationtype.False
					if i%2 == 0 {
						want = relationtype.True
					}
					msg := glowflow.NewMessage(fmt.Sprint(i), map[string]any{"temperature": 40 + (1-i%2)*20})
					result, err := node.Execute(newTestContext(), msg)
					if err != nil || result.RelationType != want {
						errs <- fmt.Errorf("#%d: relation=%q err=%v, want %q", i, result.RelationType, err, want)
					}
				}(i)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
		})
	}
}

func TestFilterInvalidConfig(t *testing.T) {
	for _, tc := range filterCases {
		t.Run(tc.nodeType, func(t *testing.T) {
			cases := []xtypes.XMap{
				{},
				{tc.key: "   "},
				{tc.key: tc.syntax},
			}
			if tc.loop != "" { // 仅脚本过滤器支持 timeout
				cases = append(cases, xtypes.XMap{tc.key: tc.pass, "timeout": "abc"})
			}
			for _, ext := range cases {
				if _, err := tryBuild(tc.nodeType, ext); !errors.Is(err, flowerrors.ErrInvalidNodeConfig) {
					t.Errorf("build with %v: err = %v, want ErrInvalidNodeConfig", ext, err)
				}
			}
		})
	}
}

// 用户脚本紧跟函数头，编译错误的行号应与用户脚本一致
func TestScriptFilterErrorLine(t *testing.T) {
	_, err := tryBuild(JsFilterNodeType, xtypes.XMap{"script": "var a = 1;\nreturn a +;"})
	if err == nil || !strings.Contains(err.Error(), " at 2:") {
		t.Errorf("js syntax error should report line 2: %v", err)
	}
	_, err = tryBuild(LuaFilterNodeType, xtypes.XMap{"script": "local a = 1\nlocal = 2\nreturn a"})
	if err == nil || !strings.Contains(err.Error(), "line:2") {
		t.Errorf("lua syntax error should report the user line: %v", err)
	}
}

func TestStripVarMarks(t *testing.T) {
	cases := map[string]string{
		"msg.a > 1":                       "msg.a > 1",
		"${msg.a} > 1":                    "msg.a > 1",
		"${msg.a > 1}":                    "msg.a > 1",
		`${msg.name} == "${keep}"`:        `msg.name == "${keep}"`,
		`msg.s == '\'${keep}'`:            `msg.s == '\'${keep}'`,
		"msg.s == `${keep}` && ${msg.ok}": "msg.s == `${keep}` && msg.ok",
		"${unclosed":                      "${unclosed",
	}
	for src, want := range cases {
		if got := stripVarMarks(src); got != want {
			t.Errorf("stripVarMarks(%q) = %q, want %q", src, got, want)
		}
	}

	node := build(t, ExprFilterNodeType, xtypes.XMap{"expr": `${msg.name} == "${x}" && ${msg.n} >= 2`})
	if got := relation(t, node, glowflow.NewMessage("m", map[string]any{"name": "${x}", "n": 2})); got != relationtype.True {
		t.Fatalf("relation = %q, want true", got)
	}
}

func TestScriptData(t *testing.T) {
	for _, item := range []struct {
		data any
		want any
	}{
		{nil, nil},
		{"text", "text"},
		{[]byte("raw"), "raw"},
		{42, 42},
		{true, true},
		{&sensor{DeviceID: "d1", Temperature: 1.5}, map[string]any{"device_id": "d1", "temperature": 1.5}},
		{[]int{1, 2}, []any{float64(1), float64(2)}},
	} {
		if got := scriptData(glowflow.NewMessage("m", item.data)); !reflect.DeepEqual(got, item.want) {
			t.Errorf("scriptData(%#v) = %#v, want %#v", item.data, got, item.want)
		}
	}
	// 无法 JSON 序列化的数据原样返回
	ch := make(chan int)
	if got := scriptData(glowflow.NewMessage("m", ch)); got != any(ch) {
		t.Errorf("scriptData(chan) = %#v", got)
	}
}
