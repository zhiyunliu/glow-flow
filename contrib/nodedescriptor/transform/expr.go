package transform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	glowflow "github.com/zhiyunliu/glow-flow"
)

const ExprTransformNodeType = "exprtransform"

// 表达式转换
/*
使用 expr-lang 表达式(https://expr-lang.org)转换消息数据(节点类型 exprtransform)

extparams 配置(二选一，同时配置时 expr 优先)：
  - expr：单个表达式，结果作为新的消息数据，如 msg.items[0] 或 {"total": msg.price * msg.qty}
  - mapping：字段名 -> 表达式 的映射(对象或 JSON 字符串)，各表达式的结果组成对象作为新的消息数据，
    如 {"name": "upper(msg.name)", "total": "msg.price * msg.qty", "id": "msgid"}

表达式中可用的变量：msg(消息数据，结构体按 json tag 转换为 map)、msgid、ts、instanceid；
兼容 ${msg.price} * 2 写法(引号外的 ${} 标记会被去除)。

转换成功走 success 分支并输出新消息(不修改原消息)；表达式执行出错走 failure 分支，消息原样传递。
*/
type nodeExpr struct {
	glowflow.Node
	program *vm.Program
	fields  []exprField
}

type exprField struct {
	name    string
	program *vm.Program
}

func (n *nodeExpr) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	vars := env(ctx, msg)
	if n.program != nil {
		out, err := expr.Run(n.program, vars)
		if err != nil {
			return failureResult(ctx, n, msg, err)
		}
		return successResult(msg, out), nil
	}

	result := make(map[string]any, len(n.fields))
	for _, field := range n.fields {
		out, err := expr.Run(field.program, vars)
		if err != nil {
			return failureResult(ctx, n, msg, fmt.Errorf("mapping.%s: %w", field.name, err))
		}
		result[field.name] = out
	}
	return successResult(msg, result), nil
}

type exprFactory struct{}

func (f *exprFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	node := &nodeExpr{Node: def}
	if src := def.ExtParams.GetString("expr"); strings.TrimSpace(src) != "" {
		program, err := compileExpr(src)
		if err != nil {
			return nil, invalidConfig(def, "%v", err)
		}
		node.program = program
		return node, nil
	}

	mapping, err := getMapping(def, "mapping")
	if err != nil {
		return nil, err
	}
	if len(mapping) == 0 {
		return nil, invalidConfig(def, "expr and mapping are both empty")
	}
	// 按字段名排序，使执行及报错顺序确定
	names := make([]string, 0, len(mapping))
	for name := range mapping {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		program, err := compileExpr(mapping[name])
		if err != nil {
			return nil, invalidConfig(def, "mapping.%s: %v", name, err)
		}
		node.fields = append(node.fields, exprField{name: name, program: program})
	}
	return node, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    ExprTransformNodeType,
		Factory: &exprFactory{},
	})
}
