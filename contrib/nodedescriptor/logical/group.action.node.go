package logical

import (
	"fmt"
	"strings"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
)

const GroupActionNodeType = "groupaction"

// 节点组
/*
并行执行一组节点(只执行节点本身，不执行其后续节点)，按匹配结果路由(节点类型 groupaction)。

extparams 配置：
  - nodeids：节点ID列表，必填，数组或逗号分隔字符串；这些节点不作为流程起始节点
  - matchrelationtype：判定为匹配的结果关系类型，默认 success
  - matchnum：匹配的节点数达到该值即走 success 分支(不再等待其余节点)；默认 0 表示全部节点都需匹配
  - timeout：最长等待时间，如 30s 或秒数，默认 30s
  - mergetomap：true 时合并为对象(map 数据的字段合并到顶层，其他数据以节点ID为键)；
    默认 false，输出数组 [{"nodeId", "relationType", "data", "err"}, ...]，按 nodeids 顺序排列

输出消息为已完成节点的合并结果：匹配数达到 matchnum 走 success 分支；
全部完成仍未达到时走 failure 分支；超时走 failure 分支。
提前结束或超时后，仍在执行的节点会收到取消信号(ctx.Done)，其结果被忽略。
*/
type nodeGroupAction struct {
	glowflow.Node
	nodeIDs       []string
	matchRelation string
	matchNum      int
	timeout       time.Duration
	mergeToMap    bool
}

var _ glowflow.NodeReferrer = (*nodeGroupAction)(nil)

func (n *nodeGroupAction) ReferencedNodes() []string {
	return n.nodeIDs
}

type groupOutcome struct {
	index int
	end   glowflow.BranchEnd
}

func (n *nodeGroupAction) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	groupCtx, cancel := glowflow.WithTimeout(ctx, n.timeout)
	defer cancel()

	outcomes := make(chan groupOutcome, len(n.nodeIDs))
	for i, nodeID := range n.nodeIDs {
		go func() {
			end := glowflow.BranchEnd{NodeID: nodeID}
			result, err := glowflow.ExecuteNode(groupCtx, nodeID, msg, glowflow.OnlyNode())
			switch {
			case err != nil:
				end.Err = err
			case len(result.Ends) > 0:
				end = result.Ends[0]
			}
			outcomes <- groupOutcome{index: i, end: end}
		}()
	}

	ends := make([]*glowflow.BranchEnd, len(n.nodeIDs))
	matched, completed := 0, 0
	for completed < len(n.nodeIDs) && matched < n.matchNum {
		select {
		case o := <-outcomes:
			completed++
			ends[o.index] = &o.end
			if o.end.Err == nil && o.end.RelationType == n.matchRelation {
				matched++
			}
		case <-groupCtx.Done():
			return failureResult(ctx, n, n.merge(msg, ends), fmt.Errorf(
				"group action timeout(%s): %d/%d nodes completed, matched %d/%d: %w",
				n.timeout, completed, len(n.nodeIDs), matched, n.matchNum, groupCtx.Err()))
		}
	}

	output := n.merge(msg, ends)
	if matched >= n.matchNum {
		return successResult(output), nil
	}
	return glowflow.ExecuteResult{RelationType: relationtype.Failure, Msg: output}, nil
}

// merge 按 nodeids 顺序合并已完成节点的结果
func (n *nodeGroupAction) merge(msg *glowflow.Message, ends []*glowflow.BranchEnd) *glowflow.Message {
	items := make([]mergeInput, 0, len(ends))
	for i, end := range ends {
		if end == nil {
			continue
		}
		items = append(items, mergeInput{nodeID: n.nodeIDs[i], relation: end.RelationType, data: messageData(end.Msg), err: end.Err})
	}
	return newMessage(msg, mergeResults(items, n.mergeToMap))
}

type groupActionFactory struct{}

func (f *groupActionFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	node := &nodeGroupAction{
		Node:          def,
		nodeIDs:       getStrings(def.ExtParams, "nodeids"),
		matchRelation: strings.TrimSpace(def.ExtParams.GetString("matchrelationtype")),
		mergeToMap:    def.ExtParams.GetBool("mergetomap"),
	}
	if len(node.nodeIDs) == 0 {
		return nil, invalidConfig(def, "nodeids is empty")
	}
	seen := make(map[string]struct{}, len(node.nodeIDs))
	for _, id := range node.nodeIDs {
		if _, ok := seen[id]; ok {
			return nil, invalidConfig(def, "duplicate node id %q in nodeids", id)
		}
		seen[id] = struct{}{}
	}
	if node.matchRelation == "" {
		node.matchRelation = relationtype.Success
	}

	var err error
	if node.matchNum, err = getInt(def, "matchnum", 0); err != nil {
		return nil, err
	}
	if node.matchNum <= 0 || node.matchNum > len(node.nodeIDs) {
		node.matchNum = len(node.nodeIDs)
	}
	if node.timeout, err = getDuration(def, "timeout", defaultWaitTimeout); err != nil {
		return nil, err
	}
	if node.timeout <= 0 {
		node.timeout = defaultWaitTimeout
	}
	return node, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    GroupActionNodeType,
		Factory: &groupActionFactory{},
	})
}
