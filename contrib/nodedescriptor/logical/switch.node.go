package logical

import (
	"github.com/expr-lang/expr"
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
)

const SwitchNodeType = "switch"

// 排他网关
/*
按顺序计算条件，走第一个满足条件的分支(节点类型 switch)；都不满足时走 default 分支。

extparams 配置：
  - cases：条件列表，必填，数组或 JSON 字符串，如
    [{"case": "msg.temperature > 50", "then": "hot"}, {"case": "msg.temperature < 0", "then": "cold"}]
    case 为 expr-lang 布尔表达式，then 为满足条件时走的连接类型(connections.type)

表达式中可用的变量：msg、msgid、ts、instanceid、loop(在循环体中时，含 index/key/item)。
条件计算出错(如对 nil 做比较)时跳过该条件；消息原样传递。
*/
type nodeSwitch struct {
	glowflow.Node
	cases []caseProgram
}

func (n *nodeSwitch) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	env := exprEnv(ctx, msg)
	for _, c := range n.cases {
		out, err := expr.Run(c.program, env)
		if err != nil {
			glowflow.GetLogger(ctx).Debugf("node[%s:%s] case %q skipped: %v", n.Type(), n.Id(), c.expr, err)
			continue
		}
		if isTrue(out) {
			return glowflow.ExecuteResult{RelationType: c.relationType, Msg: msg}, nil
		}
	}
	return glowflow.ExecuteResult{RelationType: relationtype.Default, Msg: msg}, nil
}

type switchFactory struct{}

func (f *switchFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	cases, err := getCases(def)
	if err != nil {
		return nil, err
	}
	return &nodeSwitch{Node: def, cases: cases}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    SwitchNodeType,
		Factory: &switchFactory{},
	})
}
