package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
	"github.com/zhiyunliu/glow-flow/errors"
)

// defaultScriptTimeout 脚本单次执行的默认超时时间，防止死循环阻塞流程
const defaultScriptTimeout = 2 * time.Second

// 表达式/模板中可用的变量
const (
	envKeyMsg        = "msg"        // 消息数据
	envKeyMsgID      = "msgid"      // 消息ID
	envKeyTimestamp  = "ts"         // 消息时间戳(秒)
	envKeyInstanceID = "instanceid" // 流程实例ID
)

// successResult 以转换结果构造新消息走 success 分支；并行分支共享同一个消息，因此不修改入参 msg
func successResult(msg *glowflow.Message, data any) glowflow.ExecuteResult {
	msgID := ""
	if msg != nil {
		msgID = msg.MsgId
	}
	return glowflow.ExecuteResult{RelationType: relationtype.Success, Msg: glowflow.NewMessage(msgID, data)}
}

// failureResult 记录错误日志并走 failure 分支，消息原样传递。
// 返回的 error 为 nil：dispatcher 遇到 error 会中断所在分支，执行异常应交由流程中配置的 failure 分支处理。
func failureResult(ctx glowflow.Context, node glowflow.Node, msg *glowflow.Message, err error) (glowflow.ExecuteResult, error) {
	var logCtx context.Context = context.Background()
	if ctx != nil {
		logCtx = ctx
	}
	glowflow.GetLogger(logCtx).Errorf("node[%s:%s] execute error: %v", node.Type(), node.Id(), err)
	return glowflow.ExecuteResult{RelationType: relationtype.Failure, Msg: msg}, nil
}

func invalidConfig(def glowflow.NodeDefinition, format string, args ...any) error {
	return fmt.Errorf("%w: node[%s:%s] %s", errors.ErrInvalidNodeConfig, def.NodeType, def.ID, fmt.Sprintf(format, args...))
}

// getRequired 读取必填的字符串配置
func getRequired(def glowflow.NodeDefinition, key string) (string, error) {
	val := def.ExtParams.GetString(key)
	if strings.TrimSpace(val) == "" {
		return "", invalidConfig(def, "%s is empty", key)
	}
	return val, nil
}

// getTimeout 读取脚本超时时间：支持 Go duration 格式(如 500ms)或整数秒(如 3)；未配置时使用默认值，<=0 表示不限制
func getTimeout(def glowflow.NodeDefinition, key string) (time.Duration, error) {
	val := strings.TrimSpace(def.ExtParams.GetString(key))
	if val == "" {
		return defaultScriptTimeout, nil
	}
	if d, err := time.ParseDuration(val); err == nil {
		return d, nil
	}
	secs, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, invalidConfig(def, "invalid %s %q", key, val)
	}
	return time.Duration(secs) * time.Second, nil
}

// getMapping 读取 字段名 -> 表达式 的映射，支持对象或 JSON 字符串；表达式须为字符串、数字或布尔值
func getMapping(def glowflow.NodeDefinition, key string) (map[string]string, error) {
	raw, ok := def.ExtParams.Get(key)
	if !ok || raw == nil {
		return nil, nil
	}
	var payload []byte
	if s, ok := raw.(string); ok {
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		payload = []byte(s)
	} else {
		var err error
		if payload, err = json.Marshal(raw); err != nil {
			return nil, invalidConfig(def, "invalid %s: %v", key, err)
		}
	}
	var items map[string]any
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, invalidConfig(def, "invalid %s: want object: %v", key, err)
	}
	mapping := make(map[string]string, len(items))
	for field, val := range items {
		switch v := val.(type) {
		case string:
			mapping[field] = v
		case float64, bool:
			mapping[field] = fmt.Sprint(v)
		default:
			return nil, invalidConfig(def, "invalid %s.%s: expression must be a string, got %T", key, field, val)
		}
	}
	return mapping, nil
}

// ---------------------------------------------------------------------------
// 表达式

// compileExpr 编译 expr-lang 表达式；变量在运行时才确定，因此允许未定义变量(求值为 nil)。
// 兼容 ${msg.a} + 1 写法：引号外的 ${} 标记会被去除。
func compileExpr(src string) (*vm.Program, error) {
	program, err := expr.Compile(stripVarMarks(strings.TrimSpace(src)), expr.AllowUndefinedVariables())
	if err != nil {
		return nil, fmt.Errorf("compile expr %q: %w", src, err)
	}
	return program, nil
}

// stripVarMarks 去除引号外的 ${...} 标记；${} 内不支持嵌套 }
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

// env 构造表达式/模板变量
func env(ctx glowflow.Context, msg *glowflow.Message) map[string]any {
	vars := map[string]any{envKeyMsg: scriptData(msg)}
	if msg != nil {
		vars[envKeyMsgID] = msg.MsgId
		vars[envKeyTimestamp] = msg.Timestamp
	}
	if ctx != nil {
		vars[envKeyInstanceID] = ctx.GetInstanceID()
	}
	return vars
}

// scriptData 将消息数据转换为 JSON 形态的普通值(map[string]any/[]any/基础类型)：
//   - 结构体按 json tag 暴露字段，使表达式、模板与脚本中的字段名保持一致
//   - map/slice 生成副本：并行分支共享同一个消息，脚本中修改 msg 不应影响其他分支
//
// 无法 JSON 序列化的数据原样返回。
func scriptData(msg *glowflow.Message) any {
	if msg == nil {
		return nil
	}
	data := msg.Data
	switch v := data.(type) {
	case nil:
		return nil
	case []byte:
		return string(v)
	case json.Number:
		return v
	}
	switch reflect.ValueOf(data).Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return data
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return data
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return data
	}
	return out
}
