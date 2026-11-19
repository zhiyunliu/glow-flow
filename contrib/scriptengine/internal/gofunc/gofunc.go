// Package gofunc 提供脚本引擎调用任意 Go 函数的公共逻辑：
// 将脚本侧转换得到的 Go 值按函数签名适配为实参，并处理末尾的 error 返回值。
package gofunc

import (
	"encoding/json"
	"fmt"
	"reflect"
)

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// Call 以 args 调用 fn。
//   - 缺少的参数以零值补齐，多余的参数返回错误；支持可变参数函数。
//   - 若最后一个返回值为 error 且不为 nil，则返回该错误；否则从结果中去掉该 error。
//   - fn 内部的 panic 会被转换为错误。
func Call(fn reflect.Value, args []any) (results []any, err error) {
	defer func() {
		if obj := recover(); obj != nil {
			err = fmt.Errorf("%v", obj)
		}
	}()

	fnType := fn.Type()
	numIn := fnType.NumIn()
	variadic := fnType.IsVariadic()
	fixed := numIn
	if variadic {
		fixed = numIn - 1
	}
	if !variadic && len(args) > numIn {
		return nil, fmt.Errorf("too many arguments: want %d, got %d", numIn, len(args))
	}

	in := make([]reflect.Value, 0, max(len(args), fixed))
	for i, arg := range args {
		var paramType reflect.Type
		if variadic && i >= fixed {
			paramType = fnType.In(numIn - 1).Elem()
		} else {
			paramType = fnType.In(i)
		}
		value, err := ConvertTo(arg, paramType)
		if err != nil {
			return nil, fmt.Errorf("argument #%d: %w", i+1, err)
		}
		in = append(in, value)
	}
	for i := len(args); i < fixed; i++ {
		in = append(in, reflect.Zero(fnType.In(i)))
	}

	out := fn.Call(in)
	if n := len(out); n > 0 && fnType.Out(n-1) == errorType {
		if !out[n-1].IsNil() {
			return nil, out[n-1].Interface().(error)
		}
		out = out[:n-1]
	}

	results = make([]any, len(out))
	for i := range out {
		results[i] = out[i].Interface()
	}
	return results, nil
}

// ConvertTo 将 value 转换为 target 类型。
// 可直接赋值时直接使用；数值/字符串/布尔之间按同类转换；其余情况通过 JSON 编解码适配（如 map 转结构体）。
func ConvertTo(value any, target reflect.Type) (reflect.Value, error) {
	if value == nil {
		return reflect.Zero(target), nil
	}
	rv := reflect.ValueOf(value)
	if rv.Type().AssignableTo(target) {
		return rv, nil
	}
	if sameKindFamily(rv.Kind(), target.Kind()) {
		return rv.Convert(target), nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("cannot convert %T to %s: %w", value, target, err)
	}
	ptr := reflect.New(target)
	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return reflect.Value{}, fmt.Errorf("cannot convert %T to %s: %w", value, target, err)
	}
	return ptr.Elem(), nil
}

func sameKindFamily(from reflect.Kind, to reflect.Kind) bool {
	switch {
	case isNumber(from) && isNumber(to):
		return true
	case from == reflect.String && to == reflect.String:
		return true
	case from == reflect.Bool && to == reflect.Bool:
		return true
	}
	return false
}

func isNumber(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}
