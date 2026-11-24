package glowflow

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"
)

// ErrNoRuntime 节点不在 dispatcher 中执行(如单独调用 Execute)时无法使用流程运行时功能
var ErrNoRuntime = errors.New("flow runtime is not available in context")

// BranchEnd 一个分支的最终结果：分支末端节点(没有后续节点、End 或执行出错)的执行结果
type BranchEnd struct {
	NodeID       string
	RelationType string
	Msg          *Message
	Err          error
}

// SubflowResult ExecuteNode 的执行结果
type SubflowResult struct {
	Ends []BranchEnd // 按结束先后排列的各分支结果
	Err  error       // 首个节点执行错误
}

// LastMsg 返回最后一个正常结束的分支的消息；没有时返回 nil
func (r *SubflowResult) LastMsg() *Message {
	for i := len(r.Ends) - 1; i >= 0; i-- {
		if r.Ends[i].Err == nil {
			return r.Ends[i].Msg
		}
	}
	return nil
}

type ExecuteOption func(*executeOptions)

type executeOptions struct {
	onlyNode bool
	async    bool
}

// OnlyNode 只执行指定节点本身，不继续执行其后续节点
func OnlyNode() ExecuteOption {
	return func(o *executeOptions) { o.onlyNode = true }
}

// Async 异步执行，不等待结果(返回空结果)；所在流程仍会等待其执行完成，执行错误仅记录日志
func Async() ExecuteOption {
	return func(o *executeOptions) { o.async = true }
}

// ExecuteNode 在独立的执行作用域中从 nodeID 节点开始执行(默认继续执行其后续节点)，并等待所有分支结束。
// 用于逻辑节点执行流程中的其他节点，如 for/while 的循环体、groupaction 的节点组；
// ctx 须为节点 Execute 收到的上下文或其派生，被执行的节点收到的上下文派生自 ctx，因此可通过 ctx 向其传递值。
func ExecuteNode(ctx Context, nodeID string, msg *Message, opts ...ExecuteOption) (*SubflowResult, error) {
	parent := scopeFrom(ctx)
	if parent == nil {
		return nil, ErrNoRuntime
	}
	node, ok := parent.flow.Nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}
	var o executeOptions
	for _, opt := range opts {
		opt(&o)
	}

	child := newScope(parent.flow, o)
	if !o.async {
		return child.run(ctx, []CompiledNode{node}, msg), nil
	}
	// 调用方节点执行中，parent 的计数大于 0，此时 Add 是安全的
	parent.wg.Add(1)
	go func() {
		defer parent.wg.Done()
		if result := child.run(ctx, []CompiledNode{node}, msg); result.Err != nil {
			GetLogger(ctx).Errorf("async execute node %s: %v", nodeID, result.Err)
		}
	}()
	return &SubflowResult{}, nil
}

// ---------------------------------------------------------------------------
// 执行作用域

type scopeKey struct{}

// scopeContext 将执行作用域附加到流程上下文
type scopeContext struct {
	Context
	scope *scope
}

func (c *scopeContext) Value(key any) any {
	if _, ok := key.(scopeKey); ok {
		return c.scope
	}
	return c.Context.Value(key)
}

func scopeFrom(ctx Context) *scope {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(scopeKey{}).(*scope)
	return s
}

// scope 一次(子)流程执行：跟踪执行中的节点、分支结果及汇聚节点状态。
// dispatch 为整个流程创建根作用域；ExecuteNode 创建子作用域，子作用域中的汇聚、分支结果相互独立。
type scope struct {
	flow     *CompiledChain
	onlyNode bool
	ctx      Context

	wg sync.WaitGroup

	mu       sync.Mutex
	inflight map[string]int // 节点ID -> 执行中(含等待汇聚)的任务数
	joins    map[string]*joinState
	ends     []BranchEnd
	err      error
}

type joinState struct {
	node      JoinNode
	ancestors map[string]struct{}
	inputs    []JoinInput
	pending   bool // 已收到消息，等待汇聚
	fired     bool
	timer     *time.Timer
}

func newScope(flow *CompiledChain, o executeOptions) *scope {
	return &scope{
		flow:     flow,
		onlyNode: o.onlyNode,
		inflight: make(map[string]int),
		joins:    make(map[string]*joinState),
	}
}

// run 执行起始节点及其后续节点，并等待所有分支结束
func (s *scope) run(ctx Context, nodes []CompiledNode, msg *Message) *SubflowResult {
	s.ctx = &scopeContext{Context: ctx, scope: s}
	s.mu.Lock()
	for _, node := range nodes {
		s.submitLocked(node, "", msg)
	}
	s.checkJoinsLocked()
	s.mu.Unlock()

	s.wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	return &SubflowResult{Ends: s.ends, Err: s.err}
}

func (s *scope) submitLocked(node CompiledNode, fromID string, msg *Message) {
	if join, ok := node.(JoinNode); ok {
		s.arriveLocked(join, fromID, msg)
		return
	}
	s.spawnLocked(node, func(ctx Context) (ExecuteResult, error) {
		return node.Execute(ctx, msg)
	})
}

func (s *scope) spawnLocked(node CompiledNode, exec func(ctx Context) (ExecuteResult, error)) {
	s.inflight[node.Id()]++
	s.wg.Add(1)
	go func() {
		result, err := s.execute(exec)
		s.complete(node, result, err)
	}()
}

func (s *scope) execute(exec func(ctx Context) (ExecuteResult, error)) (result ExecuteResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	if err := s.ctx.Err(); err != nil {
		return ExecuteResult{}, err
	}
	result, err = exec(s.ctx)
	if err == nil && result.Error != nil {
		err = result.Error
	}
	return result, err
}

// complete 节点执行结束：出错或没有后续节点时记录分支结果，否则提交后续节点
func (s *scope) complete(node CompiledNode, result ExecuteResult, err error) {
	defer s.wg.Done()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseLocked(node.Id())

	if err != nil {
		err = fmt.Errorf("execute node %s: %w", node.Id(), err)
		if s.err == nil {
			s.err = err
		}
		s.ends = append(s.ends, BranchEnd{NodeID: node.Id(), RelationType: relationOf(result), Msg: result.Msg, Err: err})
	} else {
		var next []CompiledNode
		if !s.onlyNode {
			next = s.flow.nextNodesOf(node.Id(), result)
		}
		if len(next) == 0 {
			s.ends = append(s.ends, BranchEnd{NodeID: node.Id(), RelationType: relationOf(result), Msg: result.Msg})
		}
		for _, nextNode := range next {
			s.submitLocked(nextNode, node.Id(), result.Msg)
		}
	}
	s.checkJoinsLocked()
}

func (s *scope) releaseLocked(nodeID string) {
	if s.inflight[nodeID]--; s.inflight[nodeID] <= 0 {
		delete(s.inflight, nodeID)
	}
}

// arriveLocked 收集到达汇聚节点的消息；首条消息到达后汇聚节点视为执行中，以保持作用域存活并阻塞其下游的汇聚节点
func (s *scope) arriveLocked(node JoinNode, fromID string, msg *Message) {
	st := s.joins[node.Id()]
	if st == nil {
		st = &joinState{node: node, ancestors: s.flow.ancestors(node.Id())}
		s.joins[node.Id()] = st
	}
	if st.fired {
		GetLogger(s.ctx).Warnf("join node %s already fired, message from %s dropped", node.Id(), fromID)
		return
	}
	st.inputs = append(st.inputs, JoinInput{FromID: fromID, Msg: msg})
	if st.pending {
		return
	}
	st.pending = true
	s.inflight[node.Id()]++
	s.wg.Add(1)
	if timeout := node.JoinTimeout(); timeout > 0 {
		st.timer = time.AfterFunc(timeout, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if st.pending {
				s.fireLocked(st, true)
			}
		})
	}
}

// checkJoinsLocked 触发不再有执行中上游节点的汇聚节点
func (s *scope) checkJoinsLocked() {
	for _, st := range s.joins {
		if st.pending && !s.upstreamRunningLocked(st) {
			s.fireLocked(st, false)
		}
	}
}

func (s *scope) upstreamRunningLocked(st *joinState) bool {
	for nodeID := range s.inflight {
		if _, ok := st.ancestors[nodeID]; ok {
			return true
		}
	}
	return false
}

func (s *scope) fireLocked(st *joinState, timedOut bool) {
	st.pending = false
	st.fired = true
	if st.timer != nil {
		st.timer.Stop()
	}
	node, inputs := st.node, st.inputs
	st.inputs = nil
	s.spawnLocked(node, func(ctx Context) (ExecuteResult, error) {
		return node.Join(ctx, inputs, timedOut)
	})
	// 先提交汇聚任务再释放等待占位，保证计数不会提前归零
	s.releaseLocked(node.Id())
	s.wg.Done()
}

func relationOf(result ExecuteResult) string {
	if result.RelationType == "" && len(result.RelationTypes) > 0 {
		return result.RelationTypes[0]
	}
	return result.RelationType
}
