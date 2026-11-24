package logical

import (
	"fmt"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
)

const JoinNodeType = "join"

// 汇聚节点
/*
等待所有可能到达本节点的上游分支结束后，合并各分支消息并只执行一次后续节点(节点类型 join)。

由 dispatcher 负责收集：当前执行作用域中不再有可到达本节点的执行中节点时即汇聚，
因此 switch/inclusive 等条件分支未走到的路径不会阻塞汇聚。

extparams 配置：
  - timeout：最长等待时间，如 30s 或秒数，默认 30s；超时后以已收到的消息走 failure 分支，之后到达的消息被丢弃
  - mergetomap：true 时合并为对象(map 数据的字段合并到顶层，其他数据以来源节点ID为键)；
    默认 false，输出数组 [{"nodeId": 来源节点ID, "data": 数据}, ...]，按到达先后排列

汇聚成功走 success 分支。
*/
type nodeJoin struct {
	glowflow.Node
	timeout    time.Duration
	mergeToMap bool
}

var _ glowflow.JoinNode = (*nodeJoin)(nil)

func (n *nodeJoin) JoinTimeout() time.Duration {
	return n.timeout
}

func (n *nodeJoin) Join(ctx glowflow.Context, inputs []glowflow.JoinInput, timedOut bool) (glowflow.ExecuteResult, error) {
	items := make([]mergeInput, 0, len(inputs))
	var first *glowflow.Message
	for _, in := range inputs {
		if first == nil && in.Msg != nil {
			first = in.Msg
		}
		items = append(items, mergeInput{nodeID: in.FromID, data: messageData(in.Msg)})
	}
	msg := newMessage(first, mergeResults(items, n.mergeToMap))
	if timedOut {
		return failureResult(ctx, n, msg, fmt.Errorf("join timeout(%s): %d branch(es) arrived", n.timeout, len(inputs)))
	}
	return successResult(msg), nil
}

// Execute 不经 dispatcher 汇聚直接执行时，视为只有一个分支
func (n *nodeJoin) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	return n.Join(ctx, []glowflow.JoinInput{{Msg: msg}}, false)
}

type joinFactory struct{}

func (f *joinFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	timeout, err := getDuration(def, "timeout", defaultWaitTimeout)
	if err != nil {
		return nil, err
	}
	return &nodeJoin{Node: def, timeout: timeout, mergeToMap: def.ExtParams.GetBool("mergetomap")}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    JoinNodeType,
		Factory: &joinFactory{},
	})
}
