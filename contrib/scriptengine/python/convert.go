package python

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/scriptengine/internal/gofunc"
	"go.starlark.net/starlark"
)

const maxConvertDepth = 100

var errTooDeep = errors.New("value nesting too deep (cyclic reference?)")

// toStarlark 将 Go 值转换为 Starlark 值：
// map 转为 dict，slice/array 转为 list，struct 按 json tag 转为 dict，函数包装为内置函数，glowflow.Context 转为 ctx 对象。
func toStarlark(v any) (starlark.Value, error) {
	return toStarlarkDepth(v, 0)
}

func toStarlarkDepth(v any, depth int) (starlark.Value, error) {
	if depth > maxConvertDepth {
		return nil, errTooDeep
	}

	switch t := v.(type) {
	case nil:
		return starlark.None, nil
	case starlark.Value:
		return t, nil
	case glowflow.Context:
		return contextValue{ctx: t}, nil
	case string:
		return starlark.String(t), nil
	case bool:
		return starlark.Bool(t), nil
	case []byte:
		return starlark.Bytes(t), nil
	case time.Time:
		return starlark.String(t.Format(time.RFC3339Nano)), nil
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return starlark.MakeInt64(i), nil
		}
		f, err := t.Float64()
		if err != nil {
			return nil, err
		}
		return starlark.Float(f), nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt64(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return starlark.MakeUint64(rv.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return starlark.Float(rv.Float()), nil
	case reflect.String:
		return starlark.String(rv.String()), nil
	case reflect.Bool:
		return starlark.Bool(rv.Bool()), nil
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return starlark.None, nil
		}
		if rv.Kind() == reflect.Pointer && rv.Elem().Kind() == reflect.Struct {
			return structToStarlark(v, depth)
		}
		return toStarlarkDepth(rv.Elem().Interface(), depth+1)
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return starlark.None, nil
		}
		items := make([]starlark.Value, rv.Len())
		for i := range items {
			item, err := toStarlarkDepth(rv.Index(i).Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return starlark.NewList(items), nil
	case reflect.Map:
		if rv.IsNil() {
			return starlark.None, nil
		}
		dict := starlark.NewDict(rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			key, err := toStarlarkDepth(iter.Key().Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			value, err := toStarlarkDepth(iter.Value().Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			if err := dict.SetKey(key, value); err != nil {
				return nil, err
			}
		}
		return dict, nil
	case reflect.Struct:
		return structToStarlark(v, depth)
	case reflect.Func:
		if rv.IsNil() {
			return starlark.None, nil
		}
		return wrapGoFunc(goFuncName(rv), rv), nil
	}
	return nil, fmt.Errorf("unsupported type %T", v)
}

// structToStarlark 结构体按 json 编码规则转换为 dict
func structToStarlark(v any, depth int) (starlark.Value, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("convert %T: %w", v, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("convert %T: %w", v, err)
	}
	return toStarlarkDepth(decoded, depth+1)
}

// fromStarlark 将 Starlark 值转换为 Go 值：
// int 转为 int64（超出范围时为 *big.Int），list/tuple/set 转为 []any，dict 转为 map[string]any；
// 函数等无法转换的值原样返回。
func fromStarlark(v starlark.Value) (any, error) {
	return fromStarlarkDepth(v, 0)
}

func fromStarlarkDepth(v starlark.Value, depth int) (any, error) {
	if depth > maxConvertDepth {
		return nil, errTooDeep
	}

	switch t := v.(type) {
	case nil, starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(t), nil
	case starlark.Int:
		if i, ok := t.Int64(); ok {
			return i, nil
		}
		return t.BigInt(), nil
	case starlark.Float:
		return float64(t), nil
	case starlark.String:
		return string(t), nil
	case starlark.Bytes:
		return []byte(t), nil
	case *starlark.Dict:
		result := make(map[string]any, t.Len())
		for _, item := range t.Items() {
			value, err := fromStarlarkDepth(item[1], depth+1)
			if err != nil {
				return nil, err
			}
			result[dictKeyString(item[0])] = value
		}
		return result, nil
	case *starlark.List, starlark.Tuple, *starlark.Set:
		iter := starlark.Iterate(t)
		defer iter.Done()
		items := make([]any, 0, starlark.Len(t))
		var item starlark.Value
		for iter.Next(&item) {
			value, err := fromStarlarkDepth(item, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		return items, nil
	case contextValue:
		return t.ctx, nil
	}
	return v, nil
}

func dictKeyString(key starlark.Value) string {
	if s, ok := starlark.AsString(key); ok {
		return s
	}
	return key.String()
}

// wrapGoFunc 将任意 Go 函数包装为 Starlark 内置函数：无返回值返回 None，单返回值直接返回，多返回值返回 tuple
func wrapGoFunc(name string, fn reflect.Value) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if len(kwargs) > 0 {
			return nil, fmt.Errorf("%s: unexpected keyword arguments", b.Name())
		}
		goArgs := make([]any, len(args))
		for i, arg := range args {
			if c, ok := arg.(contextValue); ok {
				goArgs[i] = c.resolve(thread)
				continue
			}
			value, err := fromStarlark(arg)
			if err != nil {
				return nil, fmt.Errorf("%s: argument #%d: %w", b.Name(), i+1, err)
			}
			goArgs[i] = value
		}

		results, err := gofunc.Call(fn, goArgs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", b.Name(), err)
		}
		switch len(results) {
		case 0:
			return starlark.None, nil
		case 1:
			return toStarlark(results[0])
		}
		tuple := make(starlark.Tuple, len(results))
		for i, result := range results {
			if tuple[i], err = toStarlark(result); err != nil {
				return nil, err
			}
		}
		return tuple, nil
	})
}

func goFuncName(fn reflect.Value) string {
	if f := runtime.FuncForPC(fn.Pointer()); f != nil {
		name := f.Name()
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		return name
	}
	return "go_function"
}

// contextValue 脚本中的 ctx 对象；ctx 为 nil 时使用当前调用线程绑定的 glowflow.Context
type contextValue struct {
	ctx glowflow.Context
}

var _ starlark.HasAttrs = contextValue{}

func (c contextValue) String() string        { return "<ctx>" }
func (c contextValue) Type() string          { return "ctx" }
func (c contextValue) Freeze()               {}
func (c contextValue) Truth() starlark.Bool  { return starlark.True }
func (c contextValue) Hash() (uint32, error) { return 0, errors.New("unhashable type: ctx") }
func (c contextValue) AttrNames() []string   { return []string{"get", "instance_id", "set"} }

func (c contextValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "get":
		return starlark.NewBuiltin("ctx.get", c.get), nil
	case "set":
		return starlark.NewBuiltin("ctx.set", c.set), nil
	case "instance_id":
		return starlark.NewBuiltin("ctx.instance_id", c.instanceID), nil
	}
	return nil, nil
}

func (c contextValue) resolve(thread *starlark.Thread) glowflow.Context {
	if c.ctx != nil {
		return c.ctx
	}
	ctx, _ := thread.Local(ctxLocalKey).(glowflow.Context)
	return ctx
}

func (c contextValue) mustResolve(thread *starlark.Thread, fn string) (glowflow.Context, error) {
	ctx := c.resolve(thread)
	if ctx == nil {
		return nil, fmt.Errorf("%s: ctx is not available", fn)
	}
	return ctx, nil
}

// ctx.get(key)
func (c contextValue) get(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &key); err != nil {
		return nil, err
	}
	ctx, err := c.mustResolve(thread, b.Name())
	if err != nil {
		return nil, err
	}
	return toStarlark(ctx.Get(key))
}

// ctx.set(key, value)
func (c contextValue) set(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		key   string
		value starlark.Value
	)
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 2, &key, &value); err != nil {
		return nil, err
	}
	ctx, err := c.mustResolve(thread, b.Name())
	if err != nil {
		return nil, err
	}
	goValue, err := fromStarlark(value)
	if err != nil {
		return nil, err
	}
	ctx.Set(key, goValue)
	return starlark.None, nil
}

// ctx.instance_id()
func (c contextValue) instanceID(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
		return nil, err
	}
	ctx, err := c.mustResolve(thread, b.Name())
	if err != nil {
		return nil, err
	}
	return starlark.String(ctx.GetInstanceID()), nil
}
