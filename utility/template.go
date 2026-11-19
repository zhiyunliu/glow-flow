package utility

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/zhiyunliu/golibs/xreflect"
)

type Template interface {
	Parse() error
	Execute(data map[string]any) (any, error)
	ExecuteCallback(callback func() map[string]any) (any, error)
	// HasVar 是否有变量
	HasVar() bool
}

// 支持选项：
//   - WithIncludeFunc(fn IncludeFunc): 设置自定义 include 函数
//
// include 函数使用示例：
//   - ${include("/path/to/file.txt")}: 包含文件内容（使用绝对路径）
//   - ${upper(include("/path/to/file.txt"))}: 包含文件内容并转为大写
//   - ${include("/path/to/file.txt") + suffix}: 包含文件内容并拼接后缀
func NewTemplate(tmpl any) (Template, error) {
	if v, ok := tmpl.(string); ok {
		v := strings.TrimSpace(v)
		// 如果包含变量但不是单个表达式，使用 MixedTemplate
		if CheckHasVar(v) {
			return NewExprTemplate(v)
		}
	}
	return &AnyTemplate{Tmpl: tmpl}, nil
}

// ExprTemplate 模板变量支持 这种方式 ${xx},使用expr表达式计算
type ExprTemplate struct {
	Tmpl    string
	Program *vm.Program
}

// 定义正则表达式，用于匹配形如 ${...} 的占位符
var re = regexp.MustCompile(`\$\{([^}]*)\}`)

// NewExprTemplate 创建表达式模板（向后兼容）
func NewExprTemplate(tmpl string) (*ExprTemplate, error) {
	// 使用字符串构建器来处理模板字符串
	var sb strings.Builder
	inQuotes := false // 标记是否在双引号内

	for i := 0; i < len(tmpl); i++ {
		switch tmpl[i] {
		case '"':
			// 翻转 inQuotes 标志
			inQuotes = !inQuotes
			sb.WriteByte(tmpl[i])
		case '\\':
			// 处理转义字符
			if i+1 < len(tmpl) {
				sb.WriteByte(tmpl[i])
				i++
				sb.WriteByte(tmpl[i])
			}
		default:
			if !inQuotes && i+1 < len(tmpl) && tmpl[i] == '$' && tmpl[i+1] == '{' {
				// 如果不在双引号内且遇到${，尝试匹配并替换
				loc := re.FindStringIndex(tmpl[i:])
				if loc != nil {
					// 找到匹配的 ${...}
					start, end := loc[0], loc[1]
					sb.WriteString(tmpl[i : i+start])         // 写入 ${ 前的内容
					sb.WriteString(tmpl[i+start+2 : i+end-1]) // 替换为 $1
					i += end - 1                              // 跳过已处理的部分
					continue
				}
			}
			// 如果在双引号内或未找到匹配项，直接写入字符
			sb.WriteByte(tmpl[i])
		}
	}

	// 替换后的模板字符串
	tmpl = sb.String()

	// 创建 ExprTemplate 实例
	t := &ExprTemplate{Tmpl: tmpl}

	// 调用 Parse 方法解析模板
	if err := t.Parse(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *ExprTemplate) Parse() error {
	if program, err := expr.Compile(t.Tmpl, expr.AllowUndefinedVariables()); err != nil {
		return err
	} else {
		t.Program = program
	}
	return nil
}

// buildEnv 构建包含 include 函数的环境
func (t *ExprTemplate) buildEnv(data map[string]any) map[string]any {
	env := make(map[string]any)

	// 复制原始数据
	for k, v := range data {
		env[k] = v
	}

	// 添加 include 函数（只要 config 不为 nil 就添加，支持绝对路径）
	env["include"] = t.includeFunc()
	env["fileExists"] = t.fileExistsFunc()
	return env
}

// includeFunc 返回 include 函数实现
func (t *ExprTemplate) includeFunc() func(string) string {
	return func(path string) string {
		// 默认实现：读取本地文件
		content, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return string(content)
	}
}

// fileExistsFunc 返回 fileExists 函数实现
func (t *ExprTemplate) fileExistsFunc() func(string) bool {
	return func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
}

func (t *ExprTemplate) Execute(data map[string]any) (any, error) {
	if t.Program != nil {
		// 构建包含 include 函数的环境
		env := t.buildEnv(data)
		var vm vm.VM
		return vm.Run(t.Program, env)
	}
	return nil, nil
}

func (t *ExprTemplate) ExecuteCallback(loadDataFunc func() map[string]any) (any, error) {
	var data map[string]any
	if loadDataFunc != nil {
		data = loadDataFunc()
	}
	return t.Execute(data)
}

func (t *ExprTemplate) HasVar() bool {
	return true
}

// ExecuteAsString 执行模板并返回字符串结果
func (t *ExprTemplate) ExecuteAsString(data map[string]any) string {
	result, err := t.Execute(data)
	if err != nil {
		return ""
	}
	if result == nil {
		return ""
	}
	return xreflect.GetString(result)
}

type AnyTemplate struct {
	Tmpl any
}

func (t *AnyTemplate) Parse() error {
	return nil
}

func (t *AnyTemplate) Execute(data map[string]any) (any, error) {
	return t.Tmpl, nil
}

// ExecuteFn 执行模板函数
func (t *AnyTemplate) ExecuteCallback(loadDataFunc func() map[string]any) (any, error) {
	return t.Tmpl, nil
}

func (t *AnyTemplate) HasVar() bool {
	return false
}

// MixedTemplate 支持混合字符串和变量的模板，格式如 aa/${xxx}
type MixedTemplate struct {
	Tmpl      string
	variables []struct {
		start int
		end   int
		expr  string // 保存原始表达式字符串，用于动态编译
	}
	hasVars bool // 是否包含变量
}

// NewMixedTemplate 创建混合模板（向后兼容）
func NewMixedTemplate(tmpl string) (*MixedTemplate, error) {
	t := &MixedTemplate{Tmpl: tmpl}
	if err := t.Parse(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *MixedTemplate) Parse() error {
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

func (t *MixedTemplate) Execute(data map[string]any) (any, error) {
	return t.execute(data)
}

// buildEnv 构建包含 include 函数的环境
func (t *MixedTemplate) buildEnv(data map[string]any) map[string]any {
	env := make(map[string]any)
	for k, v := range data {
		env[k] = v
	}
	return env
}

func (t *MixedTemplate) execute(data map[string]any) (string, error) {
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

func (t *MixedTemplate) ExecuteCallback(loadDataFunc func() map[string]any) (any, error) {
	var data map[string]any
	if loadDataFunc != nil {
		data = loadDataFunc()
	}
	return t.execute(data)
}

func (t *MixedTemplate) HasVar() bool {
	return t.hasVars
}
