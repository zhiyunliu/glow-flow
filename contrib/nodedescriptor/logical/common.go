package logical

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
	"github.com/zhiyunliu/glow-flow/errors"
	"github.com/zhiyunliu/golibs/xtypes"
)

// 表达式中可用的变量
const (
	envKeyMsg        = "msg"        // 消息数据
	envKeyMsgID      = "msgid"      // 消息ID
	envKeyTimestamp  = "ts"         // 消息时间戳(秒)
	envKeyInstanceID = "instanceid" // 流程实例ID
	envKeyLoop       = "loop"       // 当前循环信息：loop.index、loop.key、loop.item(不在循环中时为 nil)
)

// 合并结果中的字段名
const (
	resultKeyNodeID   = "nodeId"
	resultKeyRelation = "relationType"
	resultKeyData     = "data"
	resultKeyError    = "err"
)

const defaultWaitTimeout = 30 * time.Second

// failureResult 记录错误日志并走 failure 分支。
// 返回的 error 为 nil：dispatcher 遇到 error 会中断所在分支，执行异常应交由流程中配置的 failure 分支处理。
func failureResult(ctx glowflow.Context, node glowflow.Node, msg *glowflow.Message, err error) (glowflow.ExecuteResult, error) {
	var logCtx context.Context = context.Background()
	if ctx != nil {
		logCtx = ctx
	}
	glowflow.GetLogger(logCtx).Errorf("node[%s:%s] execute error: %v", node.Type(), node.Id(), err)
	return glowflow.ExecuteResult{RelationType: relationtype.Failure, Msg: msg}, nil
}

func successResult(msg *glowflow.Message) glowflow.ExecuteResult {
	return glowflow.ExecuteResult{RelationType: relationtype.Success, Msg: msg}
}

func invalidConfig(def glowflow.NodeDefinition, format string, args ...any) error {
	return fmt.Errorf("%w: node[%s:%s] %s", errors.ErrInvalidNodeConfig, def.NodeType, def.ID, fmt.Sprintf(format, args...))
}

func newMessage(src *glowflow.Message, data any) *glowflow.Message {
	msgID := ""
	if src != nil {
		msgID = src.MsgId
	}
	return glowflow.NewMessage(msgID, data)
}

func messageData(msg *glowflow.Message) any {
	if msg == nil {
		return nil
	}
	return msg.Data
}

// ---------------------------------------------------------------------------
// 配置读取

// getDuration 支持 Go duration 格式(如 1m30s)或整数秒(如 30)；未配置时返回默认值
func getDuration(def glowflow.NodeDefinition, key string, defValue time.Duration) (time.Duration, error) {
	val := strings.TrimSpace(def.ExtParams.GetString(key))
	if val == "" {
		return defValue, nil
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

// getInt 支持整数、字符串及 JSON 解析得到的 float64
func getInt(def glowflow.NodeDefinition, key string, defValue int) (int, error) {
	val := strings.TrimSpace(def.ExtParams.GetString(key))
	if val == "" {
		return defValue, nil
	}
	num, err := strconv.Atoi(val)
	if err != nil {
		return 0, invalidConfig(def, "invalid %s %q", key, val)
	}
	return num, nil
}

// getStrings 支持数组或逗号分隔的字符串，忽略空项
func getStrings(ext xtypes.XMap, key string) []string {
	val, ok := ext.Get(key)
	if !ok || val == nil {
		return nil
	}
	var items []string
	switch v := val.(type) {
	case []string:
		items = v
	case []any:
		for _, item := range v {
			items = append(items, fmt.Sprint(item))
		}
	default:
		items = strings.Split(ext.GetString(key), ",")
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

// getDoNode 读取循环体节点ID；子流程(chain:xxx)请使用 subchain 节点
func getDoNode(def glowflow.NodeDefinition) (string, error) {
	do := strings.TrimSpace(def.ExtParams.GetString("do"))
	if do == "" {
		return "", invalidConfig(def, "do is empty")
	}
	if strings.HasPrefix(do, "chain:") {
		return "", invalidConfig(def, "do does not support sub chain %q, use node id", do)
	}
	return do, nil
}

// ---------------------------------------------------------------------------
// 表达式

// compileExpr 编译 expr-lang 表达式；变量在运行时才确定，因此允许未定义变量(求值为 nil)。
// 兼容 ${msg.a} > 1 写法：引号外的 ${} 标记会被去除。
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

func isTrue(out any) bool {
	pass, ok := out.(bool)
	return ok && pass
}

// exprEnv 构造表达式变量
func exprEnv(ctx glowflow.Context, msg *glowflow.Message) map[string]any {
	env := map[string]any{envKeyMsg: plainData(messageData(msg)), envKeyLoop: nil}
	if msg != nil {
		env[envKeyMsgID] = msg.MsgId
		env[envKeyTimestamp] = msg.Timestamp
	}
	if ctx != nil {
		env[envKeyInstanceID] = ctx.GetInstanceID()
		if state, ok := CurrentLoop(ctx); ok {
			env[envKeyLoop] = loopEnv(state)
		}
	}
	return env
}

func loopEnv(state LoopState) map[string]any {
	return map[string]any{"index": state.Index, "key": state.Key, "item": plainData(state.Item)}
}

// plainData 将数据转换为 JSON 形态的普通值(map[string]any/[]any/基础类型)：
// 结构体按 json tag 暴露字段；map/slice 生成副本。无法 JSON 序列化的数据原样返回。
func plainData(data any) any {
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

// ---------------------------------------------------------------------------
// 条件分支(switch/inclusive)

type caseProgram struct {
	expr         string
	relationType string
	program      *vm.Program
}

// getCases 读取 cases 配置：[{"case": "msg.a > 1", "then": "case1"}, ...]，支持数组或 JSON 字符串
func getCases(def glowflow.NodeDefinition) ([]caseProgram, error) {
	raw, ok := def.ExtParams.Get("cases")
	if !ok || raw == nil {
		return nil, invalidConfig(def, "cases is empty")
	}
	var payload []byte
	if s, ok := raw.(string); ok {
		payload = []byte(s)
	} else {
		var err error
		if payload, err = json.Marshal(raw); err != nil {
			return nil, invalidConfig(def, "invalid cases: %v", err)
		}
	}
	var items []struct {
		Case string `json:"case"`
		Then string `json:"then"`
	}
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, invalidConfig(def, "invalid cases: %v", err)
	}
	if len(items) == 0 {
		return nil, invalidConfig(def, "cases is empty")
	}

	cases := make([]caseProgram, 0, len(items))
	for i, item := range items {
		item.Case, item.Then = strings.TrimSpace(item.Case), strings.TrimSpace(item.Then)
		if item.Case == "" || item.Then == "" {
			return nil, invalidConfig(def, "cases[%d]: case and then are required", i)
		}
		program, err := compileExpr(item.Case)
		if err != nil {
			return nil, invalidConfig(def, "cases[%d]: %v", i, err)
		}
		cases = append(cases, caseProgram{expr: item.Case, relationType: item.Then, program: program})
	}
	return cases, nil
}

// ---------------------------------------------------------------------------
// 循环(for/while)

// 循环结果处理方式
const (
	loopModeIgnore  = 0 // 忽略循环体结果，输出原消息
	loopModeMerge   = 1 // 合并每次迭代各分支的输出数据为数组
	loopModeReplace = 2 // 每次迭代的输出作为下一次迭代的输入，输出最后一次迭代的结果
	loopModeAsync   = 3 // 异步执行循环体，不等待结果，输出原消息(仅 for)
)

var loopModeNames = map[string]int{
	"ignore": loopModeIgnore, "merge": loopModeMerge, "replace": loopModeReplace, "async": loopModeAsync,
}

func getLoopMode(def glowflow.NodeDefinition, defValue int, allowAsync bool) (int, error) {
	val := strings.ToLower(strings.TrimSpace(def.ExtParams.GetString("mode")))
	mode, ok := loopModeNames[val]
	if !ok {
		var err error
		if mode, err = getInt(def, "mode", defValue); err != nil {
			return 0, err
		}
	}
	maxMode := loopModeReplace
	if allowAsync {
		maxMode = loopModeAsync
	}
	if mode < loopModeIgnore || mode > maxMode {
		return 0, invalidConfig(def, "unsupported mode %q", val)
	}
	return mode, nil
}

// branchData 收集各分支正常结束时的输出数据
func branchData(result *glowflow.SubflowResult) []any {
	var items []any
	for _, end := range result.Ends {
		if end.Err == nil && end.Msg != nil {
			items = append(items, end.Msg.Data)
		}
	}
	return items
}

type loopItem struct {
	key   string
	value any
}

// loopItems 将 range 结果转换为迭代项：数组/切片按顺序，map 按键排序
func loopItems(data any) ([]loopItem, error) {
	rv := reflect.ValueOf(data)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, fmt.Errorf("range is nil")
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		items := make([]loopItem, rv.Len())
		for i := range items {
			items[i] = loopItem{value: rv.Index(i).Interface()}
		}
		return items, nil
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("range map key must be string, got %s", rv.Type().Key())
		}
		keys := make([]string, 0, rv.Len())
		for _, key := range rv.MapKeys() {
			keys = append(keys, key.String())
		}
		sort.Strings(keys)
		items := make([]loopItem, len(keys))
		for i, key := range keys {
			items[i] = loopItem{key: key, value: rv.MapIndex(reflect.ValueOf(key).Convert(rv.Type().Key())).Interface()}
		}
		return items, nil
	case reflect.Struct:
		// 结构体按 json 字段遍历
		if m, ok := plainData(rv.Interface()).(map[string]any); ok {
			return loopItems(m)
		}
	}
	return nil, fmt.Errorf("range must be array, slice or map, got %T", data)
}

// ---------------------------------------------------------------------------
// 结果合并(join/groupaction)

type mergeInput struct {
	nodeID   string
	relation string
	data     any
	err      error
}

// mergeResults mergeToMap=true 时合并为对象：map 数据的字段合并到顶层，其他数据以节点ID为键；
// 否则输出数组，每项为 {"nodeId", "relationType", "data", "err"}
func mergeResults(inputs []mergeInput, mergeToMap bool) any {
	if mergeToMap {
		merged := make(map[string]any)
		for _, in := range inputs {
			if in.err != nil {
				continue
			}
			if m, ok := plainData(in.data).(map[string]any); ok {
				for k, v := range m {
					merged[k] = v
				}
			} else {
				merged[in.nodeID] = in.data
			}
		}
		return merged
	}

	list := make([]any, 0, len(inputs))
	for _, in := range inputs {
		item := map[string]any{resultKeyNodeID: in.nodeID, resultKeyData: in.data}
		if in.relation != "" {
			item[resultKeyRelation] = in.relation
		}
		if in.err != nil {
			item[resultKeyError] = in.err.Error()
		}
		list = append(list, item)
	}
	return list
}
