package action

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
	flowerrors "github.com/zhiyunliu/glow-flow/errors"
	"github.com/zhiyunliu/golibs/xtypes"
)

func newTestContext() glowflow.Context {
	return glowflow.NewContext(context.Background(), "inst-1")
}

func newDefinition(nodeType string, ext xtypes.XMap) glowflow.NodeDefinition {
	return glowflow.NodeDefinition{ID: nodeType + "-1", NodeType: nodeType, NodeName: "测试节点", ExtParams: ext}
}

func buildNode(t *testing.T, nodeType string, ext xtypes.XMap) glowflow.CompiledNode {
	t.Helper()
	node, err := tryBuildNode(nodeType, ext)
	if err != nil {
		t.Fatalf("build %s node: %v", nodeType, err)
	}
	return node
}

func tryBuildNode(nodeType string, ext xtypes.XMap) (glowflow.CompiledNode, error) {
	desc, ok := glowflow.DefaultRegistry.GetDescriptor(nodeType)
	if !ok {
		return nil, errors.New("node type is not registered: " + nodeType)
	}
	return desc.Factory.Build(newDefinition(nodeType, ext))
}

func assertInvalidConfig(t *testing.T, nodeType string, ext xtypes.XMap) {
	t.Helper()
	if _, err := tryBuildNode(nodeType, ext); !errors.Is(err, flowerrors.ErrInvalidNodeConfig) {
		t.Fatalf("build %s with %v: err = %v, want ErrInvalidNodeConfig", nodeType, ext, err)
	}
}

func execute(t *testing.T, node glowflow.CompiledNode, msg *glowflow.Message) glowflow.ExecuteResult {
	t.Helper()
	result, err := node.Execute(newTestContext(), msg)
	if err != nil {
		t.Fatalf("execute returned error %v; business failures should use the failure relation", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}
	return result
}

func assertFailure(t *testing.T, result glowflow.ExecuteResult, msg *glowflow.Message) {
	t.Helper()
	if result.RelationType != relationtype.Failure {
		t.Fatalf("relation = %q, want failure", result.RelationType)
	}
	if result.Msg != msg {
		t.Fatalf("failure should pass the original message through")
	}
}

func assertSuccessData(t *testing.T, result glowflow.ExecuteResult, msg *glowflow.Message, want any) {
	t.Helper()
	if result.RelationType != relationtype.Success {
		t.Fatalf("relation = %q, want success", result.RelationType)
	}
	if result.Msg == nil || result.Msg == msg {
		t.Fatalf("success should produce a new message instead of mutating the input")
	}
	if result.Msg.MsgId != msg.MsgId {
		t.Fatalf("msg id = %q, want %q", result.Msg.MsgId, msg.MsgId)
	}
	if !reflect.DeepEqual(result.Msg.Data, want) {
		t.Fatalf("data = %#v, want %#v", result.Msg.Data, want)
	}
}

func TestActionNodesRegistered(t *testing.T) {
	for _, nodeType := range []string{
		HttpCallNodeType, RpcCallNodeType, MsgQueueProducerNodeType,
		CommandNodeType, EmailNodeType, DingtalkNodeType,
		"delay", "xdb", "custom",
	} {
		if _, ok := glowflow.DefaultRegistry.GetDescriptor(nodeType); !ok {
			t.Errorf("node type %q is not registered", nodeType)
		}
	}
}

func TestTemplateEnv(t *testing.T) {
	type order struct {
		OrderNo string `json:"order_no"`
		Amount  int    `json:"amount"`
	}
	data := &order{OrderNo: "A1", Amount: 3}
	env := templateEnv(newTestContext(), glowflow.NewMessage("msg-1", data))

	want := map[string]any{
		"order_no": "A1", "amount": float64(3),
		"data": data, "msgid": "msg-1", "instanceid": "inst-1",
	}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("env = %#v, want %#v", env, want)
	}

	tpl, err := compileTemplate(newDefinition("t", nil), "x", "${order_no}-${amount}-${instanceid}")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := renderTemplate(tpl, env); err != nil || got != "A1-3-inst-1" {
		t.Fatalf("render = %q, %v", got, err)
	}

	if env := templateEnv(nil, glowflow.NewMessage("m", "text")); env["data"] != "text" || len(env) != 2 {
		t.Fatalf("scalar env = %#v", env)
	}
}

func TestConfigHelpers(t *testing.T) {
	ext := xtypes.XMap{
		"d1": "5s", "d2": float64(30), "d3": "1.5", "bad": "abc",
		"list1": []any{"a", " b ", ""}, "list2": "a, b,,c",
		"map1": map[string]any{"k": float64(1)}, "map2": `{"k":"v"}`, "map3": []any{1},
	}

	for key, want := range map[string]time.Duration{"d1": 5 * time.Second, "d2": 30 * time.Second, "d3": 1500 * time.Millisecond, "none": time.Minute} {
		if got, err := getDuration(ext, key, time.Minute); err != nil || got != want {
			t.Errorf("getDuration(%s) = %v, %v; want %v", key, got, err, want)
		}
	}
	if _, err := getDuration(ext, "bad", 0); err == nil {
		t.Errorf("getDuration(bad) should fail")
	}

	for key, want := range map[string]int64{"d2": 30, "n1": 465, "n2": 123, "none": 0} {
		ints := xtypes.XMap{"d2": float64(30), "n1": "465", "n2": int(123)}
		if got, err := getInt64(ints, key); err != nil || got != want {
			t.Errorf("getInt64(%s) = %d, %v; want %d", key, got, err, want)
		}
	}
	for _, bad := range []any{1.5, "abc"} {
		if _, err := getInt64(xtypes.XMap{"k": bad}, "k"); err == nil {
			t.Errorf("getInt64(%v) should fail", bad)
		}
	}

	if got := getStrings(ext, "list1"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("getStrings(list1) = %#v", got)
	}
	if got := getStrings(ext, "list2"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("getStrings(list2) = %#v", got)
	}

	if got, err := getStringMap(ext, "map1"); err != nil || !reflect.DeepEqual(got, map[string]string{"k": "1"}) {
		t.Errorf("getStringMap(map1) = %#v, %v", got, err)
	}
	if got, err := getStringMap(ext, "map2"); err != nil || !reflect.DeepEqual(got, map[string]string{"k": "v"}) {
		t.Errorf("getStringMap(map2) = %#v, %v", got, err)
	}
	if _, err := getStringMap(ext, "map3"); err == nil {
		t.Errorf("getStringMap(map3) should fail")
	}
}

func TestDecodeResult(t *testing.T) {
	cases := map[string]any{
		`{"a":1}`:   map[string]any{"a": float64(1)},
		`[1,"x"]`:   []any{float64(1), "x"},
		"  hi \r\n": "hi",
		"":          "",
		"not json":  "not json",
	}
	for raw, want := range cases {
		if got := decodeResult([]byte(raw)); !reflect.DeepEqual(got, want) {
			t.Errorf("decodeResult(%q) = %#v, want %#v", raw, got, want)
		}
	}
	if _, err := decodeResponse(500, []byte("oops")); err == nil {
		t.Errorf("decodeResponse(500) should fail")
	}
}
