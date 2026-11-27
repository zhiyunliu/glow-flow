package trace

import (
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/nodedescriptor/logical"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/models"
)

// NewRegistry 以 base 中已注册的节点为基础，创建记录执行轨迹的节点注册表。
// 节点的编译与执行逻辑不变，执行时若 context 中携带 Recorder 则记录轨迹。
func NewRegistry(base glowflow.NodeRegistry) glowflow.NodeRegistry {
	registry := glowflow.NewNodeRegistry()
	for _, desc := range base.List() {
		_ = registry.Register(&glowflow.NodeDescriptor{
			Type:          desc.Type,
			Factory:       &tracingFactory{base: desc.Factory},
			DefaultConfig: desc.DefaultConfig,
		})
	}
	return registry
}

type tracingFactory struct {
	base glowflow.NodeFactory
}

func (f *tracingFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	node, err := f.base.Build(def)
	if err != nil {
		return nil, err
	}
	traced := &tracedNode{CompiledNode: node}
	// 保留编译器与 dispatcher 依赖的接口
	if join, ok := node.(glowflow.JoinNode); ok {
		return &tracedJoinNode{tracedNode: traced, join: join}, nil
	}
	if referrer, ok := node.(glowflow.NodeReferrer); ok {
		return &tracedReferrerNode{tracedNode: traced, referrer: referrer}, nil
	}
	return traced, nil
}

type tracedNode struct {
	glowflow.CompiledNode
}

func (n *tracedNode) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	finish := n.begin(ctx)
	result, err := n.CompiledNode.Execute(ctx, msg)
	finish(result, err)
	return result, err
}

// begin 记录开始顺序，返回结束时调用的记录函数；未携带 Recorder 时不做记录
func (n *tracedNode) begin(ctx glowflow.Context) func(glowflow.ExecuteResult, error) {
	rec := FromContext(ctx)
	if rec == nil {
		return func(glowflow.ExecuteResult, error) {}
	}
	start := time.Now()
	ev := models.TraceEvent{Seq: rec.seq.Add(1), NodeID: n.Id(), NodeName: n.Name(), NodeType: n.Type()}
	if loop, ok := logical.CurrentLoop(ctx); ok {
		ev.LoopIndex = &loop.Index
	}
	return func(result glowflow.ExecuteResult, err error) {
		ev.CostMs = time.Since(start).Milliseconds()
		ev.Relation = result.RelationType
		if len(result.RelationTypes) > 1 {
			ev.Relations = result.RelationTypes
		}
		ev.End = result.End
		if result.Msg != nil {
			ev.Output = snapshot(result.Msg.Data)
		}
		if err == nil {
			err = result.Error
		}
		if err != nil {
			ev.Error = err.Error()
		}
		rec.add(ev)
	}
}

type tracedJoinNode struct {
	*tracedNode
	join glowflow.JoinNode
}

func (n *tracedJoinNode) JoinTimeout() time.Duration {
	return n.join.JoinTimeout()
}

func (n *tracedJoinNode) Join(ctx glowflow.Context, inputs []glowflow.JoinInput, timedOut bool) (glowflow.ExecuteResult, error) {
	finish := n.begin(ctx)
	result, err := n.join.Join(ctx, inputs, timedOut)
	finish(result, err)
	return result, err
}

type tracedReferrerNode struct {
	*tracedNode
	referrer glowflow.NodeReferrer
}

func (n *tracedReferrerNode) ReferencedNodes() []string {
	return n.referrer.ReferencedNodes()
}
