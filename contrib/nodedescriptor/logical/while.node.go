package logical

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	glowflow "github.com/zhiyunliu/glow-flow"
)

const WhileNodeType = "while"

// 条件循环
/*
条件满足时重复执行循环体(节点类型 while)。

extparams 配置：
  - condition：循环条件，expr-lang 布尔表达式，必填，如 msg.count < 10；每次迭代前计算，
    计算时 msg 为当前消息(上一次迭代的输出)，loop.index 为即将执行的迭代序号
  - do：循环体起始节点ID，必填。该节点(及其后续节点)每次迭代执行一次，不作为流程起始节点
  - mode：结果处理方式，默认 2
    0 / ignore：输出原消息
    1 / merge：输出所有迭代中各分支输出数据组成的数组
    2 / replace：输出最后一次迭代的结果
  - maxloops：最大迭代次数，超过时走 failure 分支；默认 0 表示不限制

每次迭代以上一次迭代的输出(最后结束的分支的消息)作为输入，因此循环体需更新条件引用的字段；
循环体中执行 break 节点后在本次迭代结束时退出循环。

循环完成走 success 分支；条件计算出错、超过最大迭代次数或循环体执行出错时走 failure 分支(输出原消息)。
*/
type nodeWhile struct {
	glowflow.Node
	condition *vm.Program
	do        string
	mode      int
	maxLoops  int
}

var _ glowflow.NodeReferrer = (*nodeWhile)(nil)

func (n *nodeWhile) ReferencedNodes() []string {
	return []string{n.do}
}

func (n *nodeWhile) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	current := msg
	var merged []any
	for i := 0; ; i++ {
		if err := ctx.Err(); err != nil {
			return failureResult(ctx, n, msg, err)
		}
		env := exprEnv(ctx, current)
		env[envKeyLoop] = loopEnv(LoopState{Index: i})
		out, err := expr.Run(n.condition, env)
		if err != nil {
			return failureResult(ctx, n, msg, fmt.Errorf("condition: %w", err))
		}
		if !isTrue(out) {
			break
		}
		if n.maxLoops > 0 && i >= n.maxLoops {
			return failureResult(ctx, n, msg, fmt.Errorf("exceeded maxloops %d", n.maxLoops))
		}

		loopCtx, loop := withLoop(ctx, LoopState{Index: i, Item: messageData(current)})
		result, err := glowflow.ExecuteNode(loopCtx, n.do, current)
		if err == nil {
			err = result.Err
		}
		if err != nil {
			return failureResult(ctx, n, msg, fmt.Errorf("iteration %d: %w", i, err))
		}
		if n.mode == loopModeMerge {
			merged = append(merged, branchData(result)...)
		}
		if last := result.LastMsg(); last != nil {
			current = last
		}
		if loop.broken.Load() {
			break
		}
	}

	switch n.mode {
	case loopModeMerge:
		if merged == nil {
			merged = []any{}
		}
		return successResult(newMessage(msg, merged)), nil
	case loopModeReplace:
		return successResult(current), nil
	default:
		return successResult(msg), nil
	}
}

type whileFactory struct{}

func (f *whileFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	node := &nodeWhile{Node: def}
	var err error
	if node.do, err = getDoNode(def); err != nil {
		return nil, err
	}
	if node.mode, err = getLoopMode(def, loopModeReplace, false); err != nil {
		return nil, err
	}
	if node.maxLoops, err = getInt(def, "maxloops", 0); err != nil {
		return nil, err
	}
	src := strings.TrimSpace(def.ExtParams.GetString("condition"))
	if src == "" {
		return nil, invalidConfig(def, "condition is empty")
	}
	if node.condition, err = compileExpr(src); err != nil {
		return nil, invalidConfig(def, "condition: %v", err)
	}
	return node, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    WhileNodeType,
		Factory: &whileFactory{},
	})
}
