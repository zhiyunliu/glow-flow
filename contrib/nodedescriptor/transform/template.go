package transform

import (
	"bytes"
	"encoding/json"
	"strings"
	"text/template"

	glowflow "github.com/zhiyunliu/glow-flow"
)

const TemplateNodeType = "template"

// 模板转换
/*
使用 Go text/template 将消息渲染为字符串(节点类型 template)

extparams 配置：
  - template：模板内容，必填，如 订单{{ .msg.order_no }}已支付，金额{{ .msg.amount }}

模板中可用的变量：.msg(消息数据，结构体按 json tag 转换为 map)、.msgid、.ts、.instanceid。
模板函数：除 text/template 内置函数外，还提供
  - escape：转义为 JSON 字符串内容(\、"、换行、回车、制表符)，如 {"name": "{{ .msg.name | escape }}"}
  - json：序列化为 JSON，如 {{ json .msg }}

渲染成功走 success 分支，输出的消息数据为渲染后的字符串(不修改原消息)；渲染出错走 failure 分支，消息原样传递。
出于安全考虑，不支持从文件(file:)加载模板。
*/
type nodeTemplate struct {
	glowflow.Node
	tpl *template.Template
}

func (n *nodeTemplate) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	var buf bytes.Buffer
	if err := n.tpl.Execute(&buf, env(ctx, msg)); err != nil {
		return failureResult(ctx, n, msg, err)
	}
	return successResult(msg, buf.String()), nil
}

var templateEscaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\n", `\n`,
	"\r", `\r`,
	"\t", `\t`,
)

var templateFuncs = template.FuncMap{
	"escape": func(s string) string {
		return templateEscaper.Replace(s)
	},
	"json": func(v any) (string, error) {
		raw, err := json.Marshal(v)
		return string(raw), err
	},
}

type templateFactory struct{}

func (f *templateFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	content, err := getRequired(def, "template")
	if err != nil {
		return nil, err
	}
	tpl, err := template.New(def.ID).Funcs(templateFuncs).Parse(content)
	if err != nil {
		return nil, invalidConfig(def, "parse template: %v", err)
	}
	return &nodeTemplate{Node: def, tpl: tpl}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    TemplateNodeType,
		Factory: &templateFactory{},
	})
}
