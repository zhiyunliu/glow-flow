package logical

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	flowerrors "github.com/zhiyunliu/glow-flow/errors"
	"github.com/zhiyunliu/golibs/xtypes"
)

// ---------------------------------------------------------------------------
// 测试节点 t.step：记录每次执行，并按配置产生输出
//   - set：输出数据
//   - add：数据为 map 时将其 n 字段加上该值
//   - relation：返回的关系类型，默认 success
//   - sleep：执行耗时(可被 ctx 取消)
//   - fail：返回该错误

const stepNodeType = "t.step"

type stepCall struct {
	data   any
	loop   LoopState
	inLoop bool
}

type stepRecorder struct {
	mu    sync.Mutex
	calls map[string][]stepCall
}

var recorder = &stepRecorder{calls: make(map[string][]stepCall)}

func (r *stepRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = make(map[string][]stepCall)
}

func (r *stepRecorder) record(nodeID string, call stepCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[nodeID] = append(r.calls[nodeID], call)
}

func (r *stepRecorder) get(nodeID string) []stepCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]stepCall(nil), r.calls[nodeID]...)
}

func (r *stepRecorder) inputs(nodeID string) []any {
	var data []any
	for _, call := range r.get(nodeID) {
		data = append(data, call.data)
	}
	return data
}

type stepNode struct {
	glowflow.Node
	set      any
	hasSet   bool
	add      float64
	relation string
	sleep    time.Duration
	fail     string
}

func (n *stepNode) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	loop, inLoop := CurrentLoop(ctx)
	recorder.record(n.Id(), stepCall{data: messageData(msg), loop: loop, inLoop: inLoop})
	if n.sleep > 0 {
		select {
		case <-time.After(n.sleep):
		case <-ctx.Done():
			return glowflow.ExecuteResult{}, ctx.Err()
		}
	}
	if n.fail != "" {
		return glowflow.ExecuteResult{}, errors.New(n.fail)
	}
	data := messageData(msg)
	if n.hasSet {
		data = n.set
	}
	if n.add != 0 {
		m := map[string]any{}
		if src, ok := plainData(data).(map[string]any); ok {
			m = src
		}
		num, _ := m["n"].(float64)
		m["n"] = num + n.add
		data = m
	}
	return glowflow.ExecuteResult{RelationType: n.relation, Msg: newMessage(msg, data)}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type: stepNodeType,
		Factory: glowflow.NodeFactoryFunc(func(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
			set, hasSet := def.ExtParams.Get("set")
			sleep, _ := time.ParseDuration(def.ExtParams.GetString("sleep"))
			relation := def.ExtParams.GetString("relation")
			if relation == "" {
				relation = "success"
			}
			add, _ := def.ExtParams.Get("add")
			addNum, _ := add.(float64)
			return &stepNode{Node: def, set: set, hasSet: hasSet, add: addNum, relation: relation, sleep: sleep, fail: def.ExtParams.GetString("fail")}, nil
		}),
	})
}

// ---------------------------------------------------------------------------
// 流程构造及执行

func node(id, nodeType string, ext xtypes.XMap) glowflow.NodeDefinition {
	return glowflow.NodeDefinition{ID: id, NodeType: nodeType, NodeName: id, ExtParams: ext}
}

func step(id string, ext xtypes.XMap) glowflow.NodeDefinition {
	return node(id, stepNodeType, ext)
}

func conn(from, to, relation string) glowflow.ConnectionDefinition {
	return glowflow.ConnectionDefinition{FromID: from, ToID: to, Type: relation}
}

type flow struct {
	nodes []glowflow.NodeDefinition
	conns []glowflow.ConnectionDefinition
}

func (f flow) compile() (*glowflow.CompiledChain, error) {
	return glowflow.NewChainCompiler().Compile(glowflow.ChainDefinition{
		ID: "test", Version: "v1", Nodes: f.nodes, Connections: f.conns,
	}, glowflow.DefaultRegistry)
}

// run 编译并执行流程，返回执行耗时
func (f flow) run(t *testing.T, data any) time.Duration {
	t.Helper()
	compiled, err := f.compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	start := time.Now()
	if err := glowflow.NewDispatcher().Dispatch(glowflow.NewContext(context.Background(), "inst-1"), compiled, glowflow.NewMessage("msg-1", data)); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	return time.Since(start)
}

func assertCalls(t *testing.T, nodeID string, want int) {
	t.Helper()
	if got := len(recorder.get(nodeID)); got != want {
		t.Fatalf("node %s executed %d times, want %d", nodeID, got, want)
	}
}

func assertInputs(t *testing.T, nodeID string, want ...any) {
	t.Helper()
	if got := recorder.inputs(nodeID); !reflect.DeepEqual(got, want) {
		t.Fatalf("node %s inputs = %#v, want %#v", nodeID, got, want)
	}
}

// ---------------------------------------------------------------------------
// switch / inclusive / fork / end / break

func TestSwitch(t *testing.T) {
	f := flow{
		nodes: []glowflow.NodeDefinition{
			step("start", nil),
			node("sw", SwitchNodeType, xtypes.XMap{"cases": []any{
				map[string]any{"case": "msg.t > 30", "then": "hot"},
				map[string]any{"case": "msg.missing.x > 1", "then": "broken"}, // 计算出错，跳过
				map[string]any{"case": "${msg.t} < 0", "then": "cold"},
			}}),
			step("hot", nil), step("cold", nil), step("other", nil), step("broken", nil),
		},
		conns: []glowflow.ConnectionDefinition{
			conn("start", "sw", ""), conn("sw", "hot", "hot"), conn("sw", "cold", "cold"),
			conn("sw", "broken", "broken"), conn("sw", "other", "default"),
		},
	}
	for _, tc := range []struct {
		t    int
		want string
	}{{40, "hot"}, {-5, "cold"}, {10, "other"}} {
		recorder.reset()
		f.run(t, map[string]any{"t": tc.t})
		for _, id := range []string{"hot", "cold", "other", "broken"} {
			want := 0
			if id == tc.want {
				want = 1
			}
			assertCalls(t, id, want)
		}
	}
}

func TestInclusive(t *testing.T) {
	f := flow{
		nodes: []glowflow.NodeDefinition{
			node("inc", InclusiveNodeType, xtypes.XMap{"cases": `[{"case":"msg.t > 0","then":"a"},{"case":"msg.t > 10","then":"b"},{"case":"msg.t > 10","then":"c"}]`}),
			step("a", nil), step("b", nil), step("other", nil),
		},
		conns: []glowflow.ConnectionDefinition{
			conn("inc", "a", "a"), conn("inc", "b", "b"), conn("inc", "other", "default"),
		},
	}
	for _, tc := range []struct {
		t       any
		a, b, o int
	}{{20, 1, 1, 1}, {5, 1, 0, 0}, {-1, 0, 0, 1}} {
		recorder.reset()
		f.run(t, map[string]any{"t": tc.t})
		// t=20 时 c 没有对应连接，回退到 default 分支
		assertCalls(t, "a", tc.a)
		assertCalls(t, "b", tc.b)
		assertCalls(t, "other", tc.o)
	}

	failing := flow{
		nodes: []glowflow.NodeDefinition{
			node("inc", InclusiveNodeType, xtypes.XMap{"cases": []any{map[string]any{"case": "msg.missing > 1", "then": "a"}}}),
			step("a", nil), step("failed", nil),
		},
		conns: []glowflow.ConnectionDefinition{conn("inc", "a", "a"), conn("inc", "failed", "failure")},
	}
	recorder.reset()
	failing.run(t, map[string]any{})
	assertCalls(t, "a", 0)
	assertCalls(t, "failed", 1)
}

func TestEndStopsBranch(t *testing.T) {
	f := flow{
		nodes: []glowflow.NodeDefinition{step("start", nil), node("end", EndNodeType, nil), step("after", nil)},
		conns: []glowflow.ConnectionDefinition{conn("start", "end", ""), conn("end", "after", "")},
	}
	recorder.reset()
	f.run(t, "x")
	assertCalls(t, "start", 1)
	assertCalls(t, "after", 0)
}

func TestBreakOutsideLoop(t *testing.T) {
	f := flow{
		nodes: []glowflow.NodeDefinition{node("brk", BreakNodeType, nil), step("after", nil)},
		conns: []glowflow.ConnectionDefinition{conn("brk", "after", "success")},
	}
	recorder.reset()
	f.run(t, "x")
	assertInputs(t, "after", "x")

	if _, err := glowflow.ExecuteNode(glowflow.NewContext(context.Background(), "i"), "after", nil); !errors.Is(err, glowflow.ErrNoRuntime) {
		t.Fatalf("ExecuteNode outside dispatcher: err = %v, want ErrNoRuntime", err)
	}
}

// ---------------------------------------------------------------------------
// fork / join

func forkJoinFlow(mergeToMap bool) flow {
	return flow{
		nodes: []glowflow.NodeDefinition{
			step("start", nil),
			node("fork", ForkNodeType, nil),
			step("a", xtypes.XMap{"set": map[string]any{"a": 1}}),
			step("a2", xtypes.XMap{"sleep": "30ms", "add": float64(5)}), // a -> a2 较慢，join 需等待
			step("b", xtypes.XMap{"set": map[string]any{"b": 2}}),
			step("c", xtypes.XMap{"set": "text"}),
			node("join", JoinNodeType, xtypes.XMap{"mergetomap": mergeToMap}),
			step("after", nil),
		},
		conns: []glowflow.ConnectionDefinition{
			conn("start", "fork", ""), conn("fork", "a", "success"), conn("fork", "b", "success"), conn("fork", "c", "success"),
			conn("a", "a2", ""), conn("a2", "join", ""), conn("b", "join", ""), conn("c", "join", ""),
			conn("join", "after", "success"),
		},
	}
}

func TestForkJoinMergeToMap(t *testing.T) {
	recorder.reset()
	forkJoinFlow(true).run(t, "input")
	assertInputs(t, "after", map[string]any{"a": float64(1), "n": float64(5), "b": float64(2), "c": "text"})
}

func TestForkJoinList(t *testing.T) {
	recorder.reset()
	forkJoinFlow(false).run(t, "input")
	inputs := recorder.inputs("after")
	if len(inputs) != 1 {
		t.Fatalf("after executed %d times, want 1", len(inputs))
	}
	got := map[string]any{}
	for _, item := range inputs[0].([]any) {
		m := item.(map[string]any)
		got[m["nodeId"].(string)] = m["data"]
	}
	want := map[string]any{
		"a2": map[string]any{"a": float64(1), "n": float64(5)},
		"b":  map[string]any{"b": 2},
		"c":  "text",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("joined = %#v, want %#v", got, want)
	}
}

// 条件分支未走到的路径不阻塞 join
func TestJoinAfterConditionalBranch(t *testing.T) {
	f := flow{
		nodes: []glowflow.NodeDefinition{
			node("sw", SwitchNodeType, xtypes.XMap{"cases": []any{
				map[string]any{"case": `msg == "a"`, "then": "a"},
				map[string]any{"case": `msg == "b"`, "then": "b"},
			}}),
			step("a", nil), step("b", nil),
			node("join", JoinNodeType, xtypes.XMap{"timeout": "5s", "mergetomap": true}),
			step("after", nil),
		},
		conns: []glowflow.ConnectionDefinition{
			conn("sw", "a", "a"), conn("sw", "b", "b"), conn("a", "join", ""), conn("b", "join", ""), conn("join", "after", ""),
		},
	}
	recorder.reset()
	if elapsed := f.run(t, "a"); elapsed > time.Second {
		t.Fatalf("join waited %s for a branch that was never taken", elapsed)
	}
	assertInputs(t, "after", map[string]any{"a": "a"})
}

func TestJoinTimeout(t *testing.T) {
	f := flow{
		nodes: []glowflow.NodeDefinition{
			node("fork", ForkNodeType, nil),
			step("slow", xtypes.XMap{"sleep": "300ms", "set": "slow"}),
			step("fast", xtypes.XMap{"set": "fast"}),
			node("join", JoinNodeType, xtypes.XMap{"timeout": "50ms", "mergetomap": true}),
			step("ok", nil), step("failed", nil),
		},
		conns: []glowflow.ConnectionDefinition{
			conn("fork", "slow", ""), conn("fork", "fast", ""), conn("slow", "join", ""), conn("fast", "join", ""),
			conn("join", "ok", "success"), conn("join", "failed", "failure"),
		},
	}
	recorder.reset()
	f.run(t, nil)
	assertCalls(t, "ok", 0)
	// 超时后到达的 slow 消息被丢弃，join 只执行一次
	assertInputs(t, "failed", map[string]any{"fast": "fast"})
}

func TestForkJoinConcurrentDispatch(t *testing.T) {
	compiled, err := forkJoinFlow(true).compile()
	if err != nil {
		t.Fatal(err)
	}
	recorder.reset()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx := glowflow.NewContext(context.Background(), fmt.Sprint("inst-", i))
			if err := glowflow.NewDispatcher().Dispatch(ctx, compiled, glowflow.NewMessage("m", i)); err != nil {
				t.Errorf("dispatch %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	inputs := recorder.inputs("after")
	if len(inputs) != 20 {
		t.Fatalf("after executed %d times, want 20", len(inputs))
	}
	for _, in := range inputs {
		if m, ok := in.(map[string]any); !ok || len(m) != 4 {
			t.Fatalf("joined data = %#v, want 4 keys", in)
		}
	}
}

// ---------------------------------------------------------------------------
// for

func forFlow(ext xtypes.XMap, body ...glowflow.NodeDefinition) flow {
	nodes := append([]glowflow.NodeDefinition{
		node("loop", ForNodeType, ext), step("after", nil), step("failed", nil),
	}, body...)
	return flow{nodes: nodes, conns: []glowflow.ConnectionDefinition{
		conn("loop", "after", "success"), conn("loop", "failed", "failure"),
	}}
}

func TestForModes(t *testing.T) {
	items := []any{map[string]any{"n": 1}, map[string]any{"n": 100}, map[string]any{"n": 100}}
	data := map[string]any{"items": items}
	body := step("body", xtypes.XMap{"add": float64(1)})

	cases := []struct {
		mode any
		want any
	}{
		{nil, data},
		{"merge", []any{map[string]any{"n": float64(2)}, map[string]any{"n": float64(101)}, map[string]any{"n": float64(101)}}},
		{float64(2), map[string]any{"n": float64(4)}}, // replace：后续迭代以上一次输出为输入
		{"async", data},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.mode), func(t *testing.T) {
			recorder.reset()
			forFlow(xtypes.XMap{"range": "msg.items", "do": "body", "mode": tc.mode}, body).run(t, data)
			// 循环体不作为起始节点：只在迭代中执行
			assertCalls(t, "body", 3)
			assertInputs(t, "after", tc.want)
		})
	}
}

func TestForRangeAndLoopVars(t *testing.T) {
	recorder.reset()
	forFlow(xtypes.XMap{"range": "1..3", "do": "body", "mode": "merge"}, step("body", nil)).run(t, nil)
	assertInputs(t, "body", 1, 2, 3)
	assertInputs(t, "after", []any{1, 2, 3})

	// 遍历 map 时按键排序，loop.key 为键
	recorder.reset()
	forFlow(xtypes.XMap{"do": "body"}, step("body", nil)).run(t, map[string]any{"b": "B", "a": "A"})
	calls := recorder.get("body")
	if len(calls) != 2 || calls[0].loop.Key != "a" || calls[1].loop.Key != "b" || calls[1].loop.Index != 1 || calls[1].data != "B" {
		t.Fatalf("map iteration calls = %+v", calls)
	}

	// 非可遍历数据走 failure
	recorder.reset()
	forFlow(xtypes.XMap{"do": "body"}, step("body", nil)).run(t, "text")
	assertCalls(t, "body", 0)
	assertInputs(t, "failed", "text")
}

func TestForBreak(t *testing.T) {
	recorder.reset()
	forFlow(xtypes.XMap{"range": "1..5", "do": "sw", "mode": "merge"},
		node("sw", SwitchNodeType, xtypes.XMap{"cases": []any{map[string]any{"case": "loop.index == 2", "then": "stop"}}}),
		node("brk", BreakNodeType, nil),
		step("body", nil),
	).withConns(conn("sw", "brk", "stop"), conn("sw", "body", "default")).run(t, nil)
	assertInputs(t, "body", 1, 2)
	assertInputs(t, "after", []any{1, 2, 3})
}

// 嵌套循环中 break 只退出最近一层循环，循环体看到的是最近一层循环的迭代信息
func TestNestedLoopBreak(t *testing.T) {
	recorder.reset()
	forFlow(xtypes.XMap{"range": "1..3", "do": "inner"},
		node("inner", ForNodeType, xtypes.XMap{"range": `["a", "b", "c"]`, "do": "sw"}),
		node("sw", SwitchNodeType, xtypes.XMap{"cases": []any{map[string]any{"case": "loop.index == 1", "then": "stop"}}}),
		node("brk", BreakNodeType, nil),
		step("body", nil),
	).withConns(conn("sw", "brk", "stop"), conn("sw", "body", "default")).run(t, nil)
	assertInputs(t, "body", "a", "a", "a")
	for _, call := range recorder.get("body") {
		if !call.inLoop || call.loop.Index != 0 || call.loop.Item != "a" {
			t.Fatalf("body loop state = %+v, want inner loop index 0", call)
		}
	}
	assertCalls(t, "after", 1)
}

func TestForBodyError(t *testing.T) {
	recorder.reset()
	forFlow(xtypes.XMap{"range": "1..3", "do": "body"}, step("body", xtypes.XMap{"fail": "boom"})).run(t, "input")
	assertCalls(t, "body", 1)
	assertCalls(t, "after", 0)
	assertInputs(t, "failed", "input")
}

func TestForBodyEndNode(t *testing.T) {
	recorder.reset()
	forFlow(xtypes.XMap{"range": "[1, 2]", "do": "end", "mode": "merge"},
		node("end", EndNodeType, nil), step("skipped", nil),
	).withConns(conn("end", "skipped", "")).run(t, nil)
	assertCalls(t, "skipped", 0)
	assertInputs(t, "after", []any{1, 2})
}

func (f flow) withConns(conns ...glowflow.ConnectionDefinition) flow {
	f.conns = append(append([]glowflow.ConnectionDefinition(nil), f.conns...), conns...)
	return f
}

// ---------------------------------------------------------------------------
// while

func whileFlow(ext xtypes.XMap, body ...glowflow.NodeDefinition) flow {
	nodes := append([]glowflow.NodeDefinition{
		node("loop", WhileNodeType, ext), step("after", nil), step("failed", nil),
	}, body...)
	return flow{nodes: nodes, conns: []glowflow.ConnectionDefinition{
		conn("loop", "after", "success"), conn("loop", "failed", "failure"),
	}}
}

func TestWhile(t *testing.T) {
	inc := step("inc", xtypes.XMap{"add": float64(1)})
	start := map[string]any{"n": 0}

	recorder.reset()
	whileFlow(xtypes.XMap{"condition": "msg.n < 3", "do": "inc"}, inc).run(t, start)
	assertCalls(t, "inc", 3)
	assertInputs(t, "after", map[string]any{"n": float64(3)})

	recorder.reset()
	whileFlow(xtypes.XMap{"condition": "${msg.n} < 3", "do": "inc", "mode": "merge"}, inc).run(t, start)
	assertInputs(t, "after", []any{map[string]any{"n": float64(1)}, map[string]any{"n": float64(2)}, map[string]any{"n": float64(3)}})

	recorder.reset()
	whileFlow(xtypes.XMap{"condition": "msg.n < 3", "do": "inc", "mode": "ignore"}, inc).run(t, start)
	assertInputs(t, "after", start)

	// loop.index 可用于条件
	recorder.reset()
	whileFlow(xtypes.XMap{"condition": "loop.index < 2", "do": "inc"}, inc).run(t, start)
	assertCalls(t, "inc", 2)

	recorder.reset()
	whileFlow(xtypes.XMap{"condition": "msg.n < 3", "do": "inc", "maxloops": float64(2)}, inc).run(t, start)
	assertCalls(t, "inc", 2)
	assertInputs(t, "failed", start)

	recorder.reset()
	whileFlow(xtypes.XMap{"condition": "msg.missing > 1", "do": "inc"}, inc).run(t, start)
	assertCalls(t, "inc", 0)
	assertInputs(t, "failed", start)
}

func TestWhileBreak(t *testing.T) {
	recorder.reset()
	whileFlow(xtypes.XMap{"condition": "true", "do": "inc"},
		step("inc", xtypes.XMap{"add": float64(1)}),
		node("sw", SwitchNodeType, xtypes.XMap{"cases": []any{map[string]any{"case": "msg.n >= 2", "then": "stop"}}}),
		node("brk", BreakNodeType, nil),
	).withConns(conn("inc", "sw", ""), conn("sw", "brk", "stop")).run(t, map[string]any{"n": 0})
	assertCalls(t, "inc", 2)
	assertInputs(t, "after", map[string]any{"n": float64(2)})
}

// ---------------------------------------------------------------------------
// groupaction

func groupFlow(ext xtypes.XMap, members ...glowflow.NodeDefinition) flow {
	nodes := append([]glowflow.NodeDefinition{
		node("group", GroupActionNodeType, ext), step("after", nil), step("failed", nil), step("downstream", nil),
	}, members...)
	conns := []glowflow.ConnectionDefinition{conn("group", "after", "success"), conn("group", "failed", "failure")}
	for _, m := range members {
		conns = append(conns, conn(m.ID, "downstream", "")) // 组内节点只执行自身
	}
	return flow{nodes: nodes, conns: conns}
}

func TestGroupActionAllMatched(t *testing.T) {
	recorder.reset()
	groupFlow(xtypes.XMap{"nodeids": "ga,gb"},
		step("ga", xtypes.XMap{"set": "A"}), step("gb", xtypes.XMap{"set": "B"}),
	).run(t, "input")
	assertInputs(t, "ga", "input")
	assertCalls(t, "downstream", 0)
	assertInputs(t, "after", []any{
		map[string]any{"nodeId": "ga", "relationType": "success", "data": "A"},
		map[string]any{"nodeId": "gb", "relationType": "success", "data": "B"},
	})
}

func TestGroupActionNotMatched(t *testing.T) {
	recorder.reset()
	groupFlow(xtypes.XMap{"nodeids": []any{"ga", "gb", "gc"}, "mergetomap": true},
		step("ga", xtypes.XMap{"set": map[string]any{"a": 1}}),
		step("gb", xtypes.XMap{"relation": "failure", "set": "B"}),
		step("gc", xtypes.XMap{"fail": "boom"}),
	).run(t, nil)
	assertCalls(t, "after", 0)
	assertInputs(t, "failed", map[string]any{"a": float64(1), "gb": "B"})
}

func TestGroupActionMatchNumReturnsEarly(t *testing.T) {
	recorder.reset()
	elapsed := groupFlow(xtypes.XMap{"nodeids": "fast,slow", "matchnum": float64(1)},
		step("fast", xtypes.XMap{"set": "F"}), step("slow", xtypes.XMap{"sleep": "2s"}),
	).run(t, nil)
	if elapsed > time.Second {
		t.Fatalf("group action waited %s, want early return after first match", elapsed)
	}
	assertInputs(t, "after", []any{map[string]any{"nodeId": "fast", "relationType": "success", "data": "F"}})
}

func TestGroupActionTimeout(t *testing.T) {
	recorder.reset()
	groupFlow(xtypes.XMap{"nodeids": "slow", "timeout": "50ms"}, step("slow", xtypes.XMap{"sleep": "2s"})).run(t, "input")
	assertCalls(t, "after", 0)
	assertInputs(t, "failed", []any{})
}

// ---------------------------------------------------------------------------
// 编译及配置校验

func TestReferencedNodeMustExist(t *testing.T) {
	_, err := forFlow(xtypes.XMap{"range": "1..2", "do": "missing"}).compile()
	if err == nil {
		t.Fatal("compile should fail for unknown do node")
	}
	_, err = groupFlow(xtypes.XMap{"nodeids": "group"}).compile()
	if err == nil {
		t.Fatal("compile should fail when group references itself")
	}
}

func TestInvalidConfig(t *testing.T) {
	for _, tc := range []struct {
		nodeType string
		ext      xtypes.XMap
	}{
		{SwitchNodeType, nil},
		{SwitchNodeType, xtypes.XMap{"cases": "not json"}},
		{SwitchNodeType, xtypes.XMap{"cases": []any{}}},
		{SwitchNodeType, xtypes.XMap{"cases": []any{map[string]any{"case": "msg.a > 1"}}}},
		{InclusiveNodeType, xtypes.XMap{"cases": []any{map[string]any{"case": "msg.a >", "then": "a"}}}},
		{ForNodeType, xtypes.XMap{}},
		{ForNodeType, xtypes.XMap{"do": "chain:sub"}},
		{ForNodeType, xtypes.XMap{"do": "x", "mode": float64(5)}},
		{ForNodeType, xtypes.XMap{"do": "x", "range": "1.."}},
		{WhileNodeType, xtypes.XMap{"do": "x"}},
		{WhileNodeType, xtypes.XMap{"do": "x", "condition": "true", "mode": "async"}},
		{WhileNodeType, xtypes.XMap{"do": "x", "condition": "true", "maxloops": "abc"}},
		{JoinNodeType, xtypes.XMap{"timeout": "abc"}},
		{GroupActionNodeType, xtypes.XMap{}},
		{GroupActionNodeType, xtypes.XMap{"nodeids": "a,a"}},
		{GroupActionNodeType, xtypes.XMap{"nodeids": "a", "matchnum": "x"}},
	} {
		desc, _ := glowflow.DefaultRegistry.GetDescriptor(tc.nodeType)
		_, err := desc.Factory.Build(node("n1", tc.nodeType, tc.ext))
		if !errors.Is(err, flowerrors.ErrInvalidNodeConfig) {
			t.Errorf("%s with %v: err = %v, want ErrInvalidNodeConfig", tc.nodeType, tc.ext, err)
		}
	}
}

func TestStripVarMarks(t *testing.T) {
	for src, want := range map[string]string{
		"${msg.a} > 1":               "msg.a > 1",
		`${msg.s} == "${keep}"`:      `msg.s == "${keep}"`,
		"msg.s == `${keep}` && ${x}": "msg.s == `${keep}` && x",
		"${unclosed":                 "${unclosed",
	} {
		if got := stripVarMarks(src); got != want {
			t.Errorf("stripVarMarks(%q) = %q, want %q", src, got, want)
		}
	}
}
