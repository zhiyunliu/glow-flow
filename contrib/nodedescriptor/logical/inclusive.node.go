package logical

import (
	"fmt"

	"github.com/expr-lang/expr"
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
)

const InclusiveNodeType = "inclusive"

// 包容网关
/*
计算所有条件，同时走所有满足条件的分支(节点类型 inclusive)；都不满足时走 default 分支。
各分支并行执行，可在下游使用 join 汇聚(未走到的分支不会阻塞 join)。

extparams 配置与 switch 相同：
  - cases：[{"case": "msg.temperature > 50", "then": "alarm"}, {"case": "msg.humidity > 80", "then": "dehumidify"}]

表达式中可用的变量：msg、msgid、ts、instanceid、loop。
任一条件计算出错时走 failure 分支；消息原样传递。
*/
type nodeInclusive struct {
	glowflow.Node
	cases []caseProgram
}

func (n *nodeInclusive) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	env := exprEnv(ctx, msg)
	var relations []string
	for _, c := range n.cases {
		out, err := expr.Run(c.program, env)
		if err != nil {
			return failureResult(ctx, n, msg, fmt.Errorf("case %q: %w", c.expr, err))
		}
		if isTrue(out) {
			relations = append(relations, c.relationType)
		}
	}
	if len(relations) == 0 {
		return glowflow.ExecuteResult{RelationType: relationtype.Default, Msg: msg}, nil
	}
	return glowflow.ExecuteResult{RelationType: relations[0], RelationTypes: relations, Msg: msg}, nil
}

type inclusiveFactory struct{}

func (f *inclusiveFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	cases, err := getCases(def)
	if err != nil {
		return nil, err
	}
	return &nodeInclusive{Node: def, cases: cases}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    InclusiveNodeType,
		Factory: &inclusiveFactory{},
	})
}
