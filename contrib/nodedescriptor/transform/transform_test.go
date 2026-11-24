package transform

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
	flowerrors "github.com/zhiyunliu/glow-flow/errors"
	"github.com/zhiyunliu/golibs/xtypes"
)

type order struct {
	OrderNo string  `json:"order_no"`
	Name    string  `json:"name"`
	Price   float64 `json:"price"`
	Qty     int     `json:"qty"`
}

func tryBuild(nodeType string, ext xtypes.XMap) (glowflow.CompiledNode, error) {
	desc, ok := glowflow.DefaultRegistry.GetDescriptor(nodeType)
	if !ok {
		return nil, errors.New("node type is not registered: " + nodeType)
	}
	return desc.Factory.Build(glowflow.NodeDefinition{ID: nodeType + "-1", NodeType: nodeType, NodeName: "转换", ExtParams: ext})
}

func build(t *testing.T, nodeType string, ext xtypes.XMap) glowflow.CompiledNode {
	t.Helper()
	node, err := tryBuild(nodeType, ext)
	if err != nil {
		t.Fatalf("build %s: %v", nodeType, err)
	}
	return node
}

func execute(t *testing.T, node glowflow.CompiledNode, msg *glowflow.Message) glowflow.ExecuteResult {
	t.Helper()
	result, err := node.Execute(glowflow.NewContext(context.Background(), "inst-1"), msg)
	if err != nil || result.Error != nil {
		t.Fatalf("execute: err=%v result.Error=%v; failures should use the failure relation", err, result.Error)
	}
	return result
}

// transformed 执行节点并校验走 success 分支、输出新消息且保留消息ID，返回新的消息数据
func transformed(t *testing.T, node glowflow.CompiledNode, msg *glowflow.Message) any {
	t.Helper()
	result := execute(t, node, msg)
	if result.RelationType != relationtype.Success {
		t.Fatalf("relation = %q, want success", result.RelationType)
	}
	if result.Msg == nil || result.Msg == msg || result.Msg.MsgId != msg.MsgId {
		t.Fatalf("should output a new message with the same msg id, got %+v", result.Msg)
	}
	return result.Msg.Data
}

func assertFailure(t *testing.T, node glowflow.CompiledNode, msg *glowflow.Message) {
	t.Helper()
	result := execute(t, node, msg)
	if result.RelationType != relationtype.Failure || result.Msg != msg {
		t.Fatalf("result = %+v, want failure with the original message", result)
	}
}

func assertInvalidConfig(t *testing.T, nodeType string, cases ...xtypes.XMap) {
	t.Helper()
	for _, ext := range cases {
		if _, err := tryBuild(nodeType, ext); !errors.Is(err, flowerrors.ErrInvalidNodeConfig) {
			t.Errorf("build %s with %v: err = %v, want ErrInvalidNodeConfig", nodeType, ext, err)
		}
	}
}

var sampleOrder = &order{OrderNo: "A1", Name: "apple", Price: 2.5, Qty: 3}

// ---------------------------------------------------------------------------
// exprtransform

func TestExprTransform(t *testing.T) {
	msg := glowflow.NewMessage("msg-1", sampleOrder)

	single := build(t, ExprTransformNodeType, xtypes.XMap{"expr": `{"total": ${msg.price} * msg.qty, "no": msg.order_no}`})
	if got, want := transformed(t, single, msg), map[string]any{"total": 7.5, "no": "A1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expr result = %#v, want %#v", got, want)
	}

	mapping := build(t, ExprTransformNodeType, xtypes.XMap{"mapping": map[string]any{
		"name":  "upper(msg.name)",
		"total": "msg.price * msg.qty",
		"id":    "msgid",
		"inst":  "instanceid",
		"const": float64(1),
	}})
	want := map[string]any{"name": "APPLE", "total": 7.5, "id": "msg-1", "inst": "inst-1", "const": 1}
	if got := transformed(t, mapping, msg); !reflect.DeepEqual(got, want) {
		t.Fatalf("mapping result = %#v, want %#v", got, want)
	}

	// mapping 支持 JSON 字符串；expr 优先于 mapping
	jsonMapping := build(t, ExprTransformNodeType, xtypes.XMap{"mapping": `{"qty": "msg.qty + 1"}`})
	if got := transformed(t, jsonMapping, msg); !reflect.DeepEqual(got, map[string]any{"qty": float64(4)}) {
		t.Fatalf("json mapping result = %#v", got)
	}
	both := build(t, ExprTransformNodeType, xtypes.XMap{"expr": "msg.name", "mapping": `{"x": "1"}`})
	if got := transformed(t, both, msg); got != "apple" {
		t.Fatalf("expr should take priority over mapping, got %#v", got)
	}

	assertFailure(t, build(t, ExprTransformNodeType, xtypes.XMap{"expr": "msg.missing + 1"}), msg)
	assertFailure(t, build(t, ExprTransformNodeType, xtypes.XMap{"mapping": `{"x": "msg.missing.y + 1"}`}), msg)
}

func TestExprTransformInvalidConfig(t *testing.T) {
	assertInvalidConfig(t, ExprTransformNodeType,
		xtypes.XMap{},
		xtypes.XMap{"expr": "   "},
		xtypes.XMap{"expr": "msg.a +"},
		xtypes.XMap{"mapping": "not json"},
		xtypes.XMap{"mapping": `{"x": "msg.a +"}`},
		xtypes.XMap{"mapping": map[string]any{"x": []any{"msg.a"}}},
	)
}

// ---------------------------------------------------------------------------
// jstransform

func TestJsTransform(t *testing.T) {
	data := map[string]any{"price": 2.5, "qty": 3, "tags": []any{"a"}}
	msg := glowflow.NewMessage("msg-1", data)

	node := build(t, JsTransformNodeType, xtypes.XMap{"script": "msg.total = msg.price * msg.qty;\nmsg.tags.push('b');\nreturn msg;"})
	got, ok := transformed(t, node, msg).(map[string]any)
	if !ok || got["total"] != 7.5 || len(got["tags"].([]any)) != 2 {
		t.Fatalf("js result = %#v", got)
	}
	// 脚本操作的是副本，原消息不受影响
	if !reflect.DeepEqual(data, map[string]any{"price": 2.5, "qty": 3, "tags": []any{"a"}}) {
		t.Fatalf("original message mutated: %#v", data)
	}

	scalar := build(t, JsTransformNodeType, xtypes.XMap{"script": "return msg.price + 1;"})
	if got := transformed(t, scalar, msg); got != 3.5 {
		t.Fatalf("scalar result = %#v", got)
	}
	ctxNode := build(t, JsTransformNodeType, xtypes.XMap{"script": "return $ctx.GetInstanceID() + ':' + msgId;"})
	if got := transformed(t, ctxNode, msg); got != "inst-1:msg-1" {
		t.Fatalf("context result = %#v", got)
	}
	upper := build(t, JsTransformNodeType, xtypes.XMap{"script": "return msg.toUpperCase();"})
	if got := transformed(t, upper, glowflow.NewMessage("msg-1", "hello")); got != "HELLO" {
		t.Fatalf("string data should be passed as is, got %#v", got)
	}
	structNode := build(t, JsTransformNodeType, xtypes.XMap{"script": "return msg.order_no;"})
	if got := transformed(t, structNode, glowflow.NewMessage("msg-1", sampleOrder)); got != "A1" {
		t.Fatalf("struct field should use json tag, got %#v", got)
	}

	assertFailure(t, build(t, JsTransformNodeType, xtypes.XMap{"script": "return msg.missing.value;"}), msg)
	assertFailure(t, build(t, JsTransformNodeType, xtypes.XMap{"script": "while (true) {}", "timeout": "100ms"}), msg)
}

func TestJsTransformInvalidConfig(t *testing.T) {
	assertInvalidConfig(t, JsTransformNodeType,
		xtypes.XMap{},
		xtypes.XMap{"script": "  "},
		xtypes.XMap{"script": "return msg +;"},
		xtypes.XMap{"script": "return msg;", "timeout": "abc"},
	)
	// 用户脚本紧跟函数头，报错行号与用户脚本一致
	if _, err := tryBuild(JsTransformNodeType, xtypes.XMap{"script": "var a = 1;\nreturn a +;"}); err == nil || !strings.Contains(err.Error(), " at 2:") {
		t.Fatalf("syntax error should report line 2: %v", err)
	}
}

// ---------------------------------------------------------------------------
// template

func TestTemplate(t *testing.T) {
	msg := glowflow.NewMessage("msg-1", sampleOrder)

	text := build(t, TemplateNodeType, xtypes.XMap{"template": "订单{{ .msg.order_no }}共{{ .msg.qty }}件，单价{{ .msg.price }}({{ .msgid }}/{{ .instanceid }})"})
	if got := transformed(t, text, msg); got != "订单A1共3件，单价2.5(msg-1/inst-1)" {
		t.Fatalf("template result = %q", got)
	}

	escaped := build(t, TemplateNodeType, xtypes.XMap{"template": `{"name": "{{ .msg.name | escape }}"}`})
	if got := transformed(t, escaped, glowflow.NewMessage("m", map[string]any{"name": "a\"b\\c\n\td"})); got != `{"name": "a\"b\\c\n\td"}` {
		t.Fatalf("escape result = %q", got)
	}

	asJSON := build(t, TemplateNodeType, xtypes.XMap{"template": `{{ json .msg }}`})
	if got := transformed(t, asJSON, msg); got != `{"name":"apple","order_no":"A1","price":2.5,"qty":3}` {
		t.Fatalf("json result = %q", got)
	}

	assertFailure(t, build(t, TemplateNodeType, xtypes.XMap{"template": "{{ index .msg 5 }}"}), msg)
}

func TestTemplateInvalidConfig(t *testing.T) {
	assertInvalidConfig(t, TemplateNodeType,
		xtypes.XMap{},
		xtypes.XMap{"template": "  "},
		xtypes.XMap{"template": "{{ .msg"},
		xtypes.XMap{"template": "{{ unknownFunc .msg }}"},
	)
}

func TestStripVarMarks(t *testing.T) {
	for src, want := range map[string]string{
		"${msg.a} + 1":         "msg.a + 1",
		`${msg.s} + "${keep}"`: `msg.s + "${keep}"`,
		"msg.s + '\\'${keep}'": "msg.s + '\\'${keep}'",
		"${unclosed":           "${unclosed",
	} {
		if got := stripVarMarks(src); got != want {
			t.Errorf("stripVarMarks(%q) = %q, want %q", src, got, want)
		}
	}
}
