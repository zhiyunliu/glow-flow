package filter

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
	"github.com/zhiyunliu/glow-flow/errors"
)

const (
	// FilterFuncName 脚本过滤器(js/lua)中包装用户脚本的函数名
	FilterFuncName = "Filter"
	// defaultScriptTimeout 脚本单次执行的默认超时时间，防止死循环阻塞流程
	defaultScriptTimeout = 2 * time.Second
)

// 表达式过滤器中可用的变量
const (
	envKeyMsg        = "msg"        // 消息数据
	envKeyMsgID      = "msgid"      // 消息ID
	envKeyTimestamp  = "ts"         // 消息时间戳(秒)
	envKeyInstanceID = "instanceid" // 流程实例ID
)

// boolResult 结果为 true 时走 true 分支，否则(含非 bool 结果)走 false 分支，消息原样传递
func boolResult(msg *glowflow.Message, out any) glowflow.ExecuteResult {
	relation := relationtype.False
	if pass, ok := out.(bool); ok && pass {
		relation = relationtype.True
	}
	return glowflow.ExecuteResult{RelationType: relation, Msg: msg}
}

// failureResult 记录错误日志并走 failure 分支。
// 返回的 error 为 nil：dispatcher 遇到 error 会中断流程，执行异常应交由流程中配置的 failure 分支处理。
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

// getScript 读取必填的脚本内容
func getScript(def glowflow.NodeDefinition, key string) (string, error) {
	script := def.ExtParams.GetString(key)
	if strings.TrimSpace(script) == "" {
		return "", invalidConfig(def, "%s is empty", key)
	}
	return script, nil
}

// getTimeout 读取脚本超时时间：支持 Go duration 格式(如 500ms)或秒数(如 3)；未配置时使用默认值，<=0 表示不限制
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
	return time.Duration(time.Duration(secs) * time.Second), nil
}

// scriptData 将消息数据转换为 JSON 形态的普通值(map[string]any/[]any/基础类型)：
//   - 结构体按 json tag 暴露字段，使表达式与各脚本引擎中的字段名保持一致
//   - map/slice 生成副本：并行分支共享同一个消息，脚本中修改 msg 不应影响其他分支及下游节点
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

// exprEnv 构造表达式过滤器的变量
func exprEnv(ctx glowflow.Context, msg *glowflow.Message) map[string]any {
	env := map[string]any{envKeyMsg: scriptData(msg)}
	if msg != nil {
		env[envKeyMsgID] = msg.MsgId
		env[envKeyTimestamp] = msg.Timestamp
	}
	if ctx != nil {
		env[envKeyInstanceID] = ctx.GetInstanceID()
	}
	return env
}

// msgID 脚本过滤器函数的第二个参数
func msgID(msg *glowflow.Message) string {
	if msg == nil {
		return ""
	}
	return msg.MsgId
}
