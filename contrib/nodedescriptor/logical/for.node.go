package logical

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	glowflow "github.com/zhiyunliu/glow-flow"
)

const ForNodeType = "for"

// 遍历循环
/*
遍历数组/map，对每一项执行循环体(节点类型 for)。

extparams 配置：
  - range：遍历对象的 expr-lang 表达式，如 msg.items、1..5；为空时遍历消息数据本身。
    支持数组/切片(按顺序)、map 或对象(按键排序)
  - do：循环体起始节点ID，必填。该节点(及其后续节点)每次迭代执行一次，不作为流程起始节点
  - mode：结果处理方式，默认 0
    0 / ignore：忽略循环体结果，输出原消息
    1 / merge：输出所有迭代中各分支输出数据组成的数组
    2 / replace：首次迭代输入为迭代项，之后每次迭代以上一次的输出为输入，输出最后一次迭代的结果
    3 / async：异步执行循环体，不等待结果，输出原消息(流程仍会等待其执行完成)

除 replace 模式的后续迭代外，循环体的输入消息数据为当前迭代项。
循环体中可通过 loop 变量(表达式)或 CurrentLoop 获取 index、key(遍历 map 时)、item；
循环体中执行 break 节点后在本次迭代结束时退出循环。

循环完成走 success 分支；range 计算失败、不可遍历或循环体执行出错时走 failure 分支(输出原消息)。
*/
type nodeFor struct {
	glowflow.Node
	rangeExpr *vm.Program
	do        string
	mode      int
}

var _ glowflow.NodeReferrer = (*nodeFor)(nil)

func (n *nodeFor) ReferencedNodes() []string {
	return []string{n.do}
}

func (n *nodeFor) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	data := messageData(msg)
	if n.rangeExpr != nil {
		out, err := expr.Run(n.rangeExpr, exprEnv(ctx, msg))
		if err != nil {
			return failureResult(ctx, n, msg, fmt.Errorf("range: %w", err))
		}
		data = out
	}
	items, err := loopItems(data)
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}

	current := msg
	var merged []any
	for i, item := range items {
		if err := ctx.Err(); err != nil {
			return failureResult(ctx, n, msg, err)
		}
		input := current
		if n.mode != loopModeReplace || i == 0 {
			input = newMessage(msg, item.value)
		}
		loopCtx, loop := withLoop(ctx, LoopState{Index: i, Key: item.key, Item: item.value})

		if n.mode == loopModeAsync {
			if _, err := glowflow.ExecuteNode(loopCtx, n.do, input, glowflow.Async()); err != nil {
				return failureResult(ctx, n, msg, err)
			}
			continue
		}

		result, err := glowflow.ExecuteNode(loopCtx, n.do, input)
		if err == nil {
			err = result.Err
		}
		if err != nil {
			return failureResult(ctx, n, msg, fmt.Errorf("iteration %d: %w", i, err))
		}
		switch n.mode {
		case loopModeMerge:
			merged = append(merged, branchData(result)...)
		case loopModeReplace:
			if last := result.LastMsg(); last != nil {
				current = last
			}
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

type forFactory struct{}

func (f *forFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	node := &nodeFor{Node: def}
	var err error
	if node.do, err = getDoNode(def); err != nil {
		return nil, err
	}
	if node.mode, err = getLoopMode(def, loopModeIgnore, true); err != nil {
		return nil, err
	}
	if src := strings.TrimSpace(def.ExtParams.GetString("range")); src != "" {
		if node.rangeExpr, err = compileExpr(src); err != nil {
			return nil, invalidConfig(def, "range: %v", err)
		}
	}
	return node, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    ForNodeType,
		Factory: &forFactory{},
	})
}
