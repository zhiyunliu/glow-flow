package utility

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/zhiyunliu/golibs/xreflect"
)

type Template interface {
	Parse() error
	// Execute executes the template with the provided data map.
	Execute(data map[string]any) (any, error)
	// ExecuteCallback executes the template with a callback function that provides the data map.
	ExecuteCallback(callback func() map[string]any) (any, error)
	// HasVar returns true if the template contains variables, otherwise false.
	HasVar() bool
}

func NewTemplate(tmpl string) (Template, error) {
	v := strings.TrimSpace(tmpl)
	return newTemplate(v)
}

// templateImpl 支持混合字符串和变量的模板，格式如 aa/${xxx}
type templateImpl struct {
	Tmpl      string
	variables []struct {
		start int
		end   int
		expr  string // 保存原始表达式字符串，用于动态编译
	}
	hasVars bool // 是否包含变量
}

// NewMixedTemplate 创建混合模板（向后兼容）
func newTemplate(tmpl string) (*templateImpl, error) {
	t := &templateImpl{Tmpl: tmpl}
	if err := t.Parse(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *templateImpl) Parse() error {
	// 先检查是否包含${}变量
	if !strings.Contains(t.Tmpl, VarPrefix) {
		t.hasVars = false
		return nil
	}

	t.hasVars = true
	tmpl := t.Tmpl
	start := 0

	for {
		idx := strings.Index(tmpl[start:], VarPrefix)
		if idx == -1 {
			break
		}

		varStart := start + idx
		endIdx := strings.Index(tmpl[varStart+2:], VarSuffix)
		if endIdx == -1 {
			break
		}

		varEnd := varStart + 2 + endIdx
		varName := tmpl[varStart+2 : varEnd]

		// 保存原始表达式字符串，在执行时动态编译
		t.variables = append(t.variables, struct {
			start int
			end   int
			expr  string
		}{
			start: varStart,
			end:   varEnd + 1,
			expr:  varName,
		})

		start = varEnd + 1
	}

	return nil
}

func (t *templateImpl) Execute(data map[string]any) (any, error) {
	return t.execute(data)
}

// buildEnv 构建包含 include 函数的环境
func (t *templateImpl) buildEnv(data map[string]any) map[string]any {
	env := make(map[string]any)
	for k, v := range data {
		env[k] = v
	}
	return env
}

func (t *templateImpl) execute(data map[string]any) (string, error) {
	// 如果没有变量，直接返回原始字符串
	if !t.hasVars {
		return t.Tmpl, nil
	}

	if len(t.variables) == 0 {
		return t.Tmpl, nil
	}

	// 构建包含 include 函数的环境
	env := t.buildEnv(data)

	var sb strings.Builder
	lastPos := 0
	vmInstance := &vm.VM{}

	for _, v := range t.variables {
		sb.WriteString(t.Tmpl[lastPos:v.start])

		// 动态编译表达式，使用环境变量
		program, err := expr.Compile(v.expr, expr.Env(env), expr.AllowUndefinedVariables())
		if err != nil {
			return "", fmt.Errorf("failed to compile expression '%s': %v", v.expr, err)
		}

		val, err := vmInstance.Run(program, env)
		if err != nil {
			return "", err
		}
		sb.WriteString(xreflect.GetString(val))
		lastPos = v.end
	}
	sb.WriteString(t.Tmpl[lastPos:])
	return sb.String(), nil
}

func (t *templateImpl) ExecuteCallback(loadDataFunc func() map[string]any) (any, error) {
	var data map[string]any
	if loadDataFunc != nil {
		data = loadDataFunc()
	}
	return t.execute(data)
}

func (t *templateImpl) HasVar() bool {
	return t.hasVars
}
