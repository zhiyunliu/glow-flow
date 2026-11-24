package filter

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	glowflow "github.com/zhiyunliu/glow-flow"
)

const ExprFilterNodeType = "exprfilter"

// 表达式过滤器
/*
使用 expr-lang 表达式(https://expr-lang.org)对消息进行布尔判定(节点类型 exprfilter)

extparams 配置：
  - expr：布尔表达式，必填，如 msg.temperature > 50 && msg.status == "ok"；
    兼容 ${msg.temperature} > 50 写法(引号外的 ${} 标记会被去除)

表达式中可用的变量：
  - msg：消息数据(结构体按 json tag 转换为 map)
  - msgid：消息ID
  - ts：消息时间戳(秒)
  - instanceid：流程实例ID

结果为 true 走 true 分支，false 或非布尔结果走 false 分支，消息原样传递；
表达式执行出错(如对 nil 做比较运算)走 failure 分支。

节点配置示例：

	{
	  "id": "f1",
	  "type": "exprfilter",
	  "name": "温度过滤",
	  "extparams": {"expr": "msg.temperature > 50"}
	}
*/
type nodeExprFilter struct {
	glowflow.Node
	program *vm.Program
}

func (n *nodeExprFilter) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	out, err := expr.Run(n.program, exprEnv(ctx, msg))
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}
	return boolResult(msg, out), nil
}

type exprFilterFactory struct{}

func (f *exprFilterFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	src, err := getScript(def, "expr")
	if err != nil {
		return nil, err
	}
	program, err := compileFilterExpr(src)
	if err != nil {
		return nil, invalidConfig(def, "%v", err)
	}
	return &nodeExprFilter{Node: def, program: program}, nil
}

// compileFilterExpr 编译表达式；变量在运行时才确定，因此允许未定义变量(求值为 nil)
func compileFilterExpr(src string) (*vm.Program, error) {
	program, err := expr.Compile(stripVarMarks(strings.TrimSpace(src)), expr.AllowUndefinedVariables())
	if err != nil {
		return nil, fmt.Errorf("compile expr: %w", err)
	}
	return program, nil
}

// stripVarMarks 去除引号外的 ${...} 标记，如 ${msg.a} > 1 转换为 msg.a > 1。
// 与模板写法保持兼容；${} 内不支持嵌套 }。
func stripVarMarks(src string) string {
	if !strings.Contains(src, "${") {
		return src
	}
	var sb strings.Builder
	var quote byte
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case quote != 0:
			sb.WriteByte(c)
			if c == '\\' && quote != '`' && i+1 < len(src) {
				i++
				sb.WriteByte(src[i])
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
			sb.WriteByte(c)
		case c == '$' && i+1 < len(src) && src[i+1] == '{':
			end := strings.IndexByte(src[i+2:], '}')
			if end < 0 {
				sb.WriteString(src[i:])
				return sb.String()
			}
			sb.WriteString(src[i+2 : i+2+end])
			i += 2 + end
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    ExprFilterNodeType,
		Factory: &exprFilterFactory{},
	})
}
