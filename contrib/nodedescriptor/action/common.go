package action

import (
	"bytes"
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
	"github.com/zhiyunliu/glow-flow/utility"
	"github.com/zhiyunliu/golibs/xtypes"
)

// 模板(${...})中可用的保留变量；消息数据为 map/struct 时，其字段同时以顶层变量的形式提供
const (
	tplKeyData       = "data"       // 完整的消息数据
	tplKeyMsgID      = "msgid"      // 消息ID
	tplKeyInstanceID = "instanceid" // 流程实例ID
)

// successResult 走 success 分支，并以 data 构造新的下游消息。
// 并行分支共享同一个 msg，因此节点不应直接修改入参 msg。
func successResult(msg *glowflow.Message, data any) glowflow.ExecuteResult {
	msgID := ""
	if msg != nil {
		msgID = msg.MsgId
	}
	return glowflow.ExecuteResult{
		RelationType: relationtype.Success,
		Msg:          glowflow.NewMessage(msgID, data),
	}
}

// passResult 走 success 分支，原样传递消息
func passResult(msg *glowflow.Message) glowflow.ExecuteResult {
	return glowflow.ExecuteResult{
		RelationType: relationtype.Success,
		Msg:          msg,
	}
}

// failureResult 记录错误日志并走 failure 分支。
// 返回的 error 为 nil：dispatcher 遇到 error 会中断流程，业务失败应交由流程中配置的 failure 分支处理。
func failureResult(ctx glowflow.Context, node glowflow.Node, msg *glowflow.Message, err error) (glowflow.ExecuteResult, error) {
	glowflow.GetLogger(stdContext(ctx)).Errorf("node[%s:%s] execute error: %v", node.Type(), node.Id(), err)
	return glowflow.ExecuteResult{
		RelationType: relationtype.Failure,
		Msg:          msg,
	}, nil
}

func invalidConfig(def glowflow.NodeDefinition, format string, args ...any) error {
	return fmt.Errorf("%w: node[%s:%s] %s", errors.ErrInvalidNodeConfig, def.NodeType, def.ID, fmt.Sprintf(format, args...))
}

func stdContext(ctx glowflow.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// withTimeout timeout<=0 时不设置超时
func withTimeout(ctx glowflow.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return stdContext(ctx), func() {}
	}
	return context.WithTimeout(stdContext(ctx), timeout)
}

func messageData(msg *glowflow.Message) any {
	if msg == nil {
		return nil
	}
	return msg.Data
}

// ---------------------------------------------------------------------------
// 配置读取

// getDuration 支持 Go duration 格式(如 1m30s)或秒数(如 30)
func getDuration(ext xtypes.XMap, key string, def time.Duration) (time.Duration, error) {
	val := strings.TrimSpace(ext.GetString(key))
	if val == "" {
		return def, nil
	}
	if d, err := time.ParseDuration(val); err == nil {
		return d, nil
	}
	secs, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %s=%q", key, val)
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// getInt64 支持整数、字符串及 JSON 解析得到的 float64(XMap.GetInt64 不支持 float64)；未配置时返回 0
func getInt64(ext xtypes.XMap, key string) (int64, error) {
	val := strings.TrimSpace(ext.GetString(key))
	if val == "" {
		return 0, nil
	}
	num, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %s=%q", key, val)
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
	return compactStrings(items)
}

func compactStrings(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

// getStringMap 支持 JSON 对象或 JSON 字符串
func getStringMap(ext xtypes.XMap, key string) (map[string]string, error) {
	val, ok := ext.Get(key)
	if !ok || val == nil {
		return nil, nil
	}
	if s, ok := val.(string); ok {
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", key, err)
		}
		val = m
	}

	rv := reflect.ValueOf(val)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("invalid %s: want object, got %T", key, val)
	}
	result := make(map[string]string, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		result[iter.Key().String()] = xtypes.GetString(iter.Value().Interface())
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// 模板

func compileTemplate(def glowflow.NodeDefinition, key string, src string) (utility.Template, error) {
	tpl, err := utility.NewTemplate(src)
	if err != nil {
		return nil, invalidConfig(def, "%s template error: %v", key, err)
	}
	return tpl, nil
}

func compileTemplates(def glowflow.NodeDefinition, key string, items []string) ([]utility.Template, error) {
	tpls := make([]utility.Template, 0, len(items))
	for _, item := range items {
		tpl, err := compileTemplate(def, key, item)
		if err != nil {
			return nil, err
		}
		tpls = append(tpls, tpl)
	}
	return tpls, nil
}

func compileTemplateMap(def glowflow.NodeDefinition, key string, items map[string]string) (map[string]utility.Template, error) {
	tpls := make(map[string]utility.Template, len(items))
	for k, v := range items {
		tpl, err := compileTemplate(def, key+"."+k, v)
		if err != nil {
			return nil, err
		}
		tpls[k] = tpl
	}
	return tpls, nil
}

func renderTemplate(tpl utility.Template, env map[string]any) (string, error) {
	if tpl == nil {
		return "", nil
	}
	val, err := tpl.Execute(env)
	if err != nil {
		return "", err
	}
	return xtypes.GetString(val), nil
}

// renderList 渲染后的每一项再按逗号拆分，以便模板变量展开为多个值
func renderList(tpls []utility.Template, env map[string]any) ([]string, error) {
	var items []string
	for _, tpl := range tpls {
		val, err := renderTemplate(tpl, env)
		if err != nil {
			return nil, err
		}
		items = append(items, strings.Split(val, ",")...)
	}
	return compactStrings(items), nil
}

func renderMap(tpls map[string]utility.Template, env map[string]any) (map[string]string, error) {
	if len(tpls) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(tpls))
	for k, tpl := range tpls {
		val, err := renderTemplate(tpl, env)
		if err != nil {
			return nil, err
		}
		result[k] = val
	}
	return result, nil
}

// templateEnv 构造模板变量：消息数据的字段 + data/msgid/instanceid 保留变量
func templateEnv(ctx glowflow.Context, msg *glowflow.Message) map[string]any {
	env := make(map[string]any)
	data := messageData(msg)
	for k, v := range toMap(data) {
		env[k] = v
	}
	env[tplKeyData] = data
	if msg != nil {
		env[tplKeyMsgID] = msg.MsgId
	}
	if ctx != nil {
		env[tplKeyInstanceID] = ctx.GetInstanceID()
	}
	return env
}

func toMap(data any) map[string]any {
	switch v := data.(type) {
	case nil:
		return nil
	case map[string]any:
		return v
	case xtypes.XMap:
		return v
	}
	rv := reflect.ValueOf(data)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Map && rv.Kind() != reflect.Struct {
		return nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// ---------------------------------------------------------------------------
// 数据编解码

// payloadString string/[]byte 原样返回，其他类型 JSON 序列化
func payloadString(data any) (string, error) {
	switch v := data.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// decodeResult 合法 JSON 解析为 map/slice/基础类型，否则返回去除首尾空白的字符串
func decodeResult(raw []byte) any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	if json.Valid(trimmed) {
		var v any
		if err := json.Unmarshal(trimmed, &v); err == nil {
			return v
		}
	}
	return string(trimmed)
}

// decodeResponse 2xx 时解析响应内容，否则返回错误
func decodeResponse(status int32, result []byte) (any, error) {
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", status, truncate(result, 512))
	}
	return decodeResult(result), nil
}

func truncate(raw []byte, size int) string {
	if len(raw) <= size {
		return string(raw)
	}
	return string(raw[:size]) + "..."
}
