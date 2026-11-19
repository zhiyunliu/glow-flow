package lua

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	glua "github.com/yuin/gopher-lua"
	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/scriptengine/internal/gofunc"
)

const (
	ctxTypeName     = "glowflow.ctx"
	maxConvertDepth = 100
)

var errTooDeep = errors.New("value nesting too deep (cyclic reference?)")

// toLValue 将 Go 值转换为 Lua 值：
// map/slice/struct 转为 table（struct 按 json tag），函数包装为 Lua 函数，glowflow.Context 转为 ctx 对象，
// 其他无法表示的类型以 userdata 形式传递。
func toLValue(vm *glua.LState, v any) (glua.LValue, error) {
	return toLValueDepth(vm, v, 0)
}

func toLValueDepth(vm *glua.LState, v any, depth int) (glua.LValue, error) {
	if depth > maxConvertDepth {
		return nil, errTooDeep
	}

	switch t := v.(type) {
	case nil:
		return glua.LNil, nil
	case glua.LValue:
		return t, nil
	case glowflow.Context:
		return newCtxUserData(vm, t), nil
	case glua.LGFunction:
		return vm.NewFunction(t), nil
	case func(*glua.LState) int:
		return vm.NewFunction(t), nil
	case string:
		return glua.LString(t), nil
	case bool:
		return glua.LBool(t), nil
	case []byte:
		return glua.LString(t), nil
	case time.Time:
		return glua.LString(t.Format(time.RFC3339Nano)), nil
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return nil, err
		}
		return glua.LNumber(f), nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return glua.LNumber(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return glua.LNumber(rv.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return glua.LNumber(rv.Float()), nil
	case reflect.String:
		return glua.LString(rv.String()), nil
	case reflect.Bool:
		return glua.LBool(rv.Bool()), nil
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return glua.LNil, nil
		}
		if rv.Kind() == reflect.Pointer && rv.Elem().Kind() == reflect.Struct {
			return structToLValue(vm, v, depth)
		}
		return toLValueDepth(vm, rv.Elem().Interface(), depth+1)
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return glua.LNil, nil
		}
		tbl := vm.CreateTable(rv.Len(), 0)
		for i := 0; i < rv.Len(); i++ {
			item, err := toLValueDepth(vm, rv.Index(i).Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			tbl.RawSetInt(i+1, item)
		}
		return tbl, nil
	case reflect.Map:
		if rv.IsNil() {
			return glua.LNil, nil
		}
		tbl := vm.CreateTable(0, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			key, err := toLValueDepth(vm, iter.Key().Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			if key == glua.LNil {
				continue
			}
			value, err := toLValueDepth(vm, iter.Value().Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			tbl.RawSet(key, value)
		}
		return tbl, nil
	case reflect.Struct:
		return structToLValue(vm, v, depth)
	case reflect.Func:
		if rv.IsNil() {
			return glua.LNil, nil
		}
		return vm.NewFunction(wrapGoFunc(rv)), nil
	}

	ud := vm.NewUserData()
	ud.Value = v
	return ud, nil
}

// structToLValue 结构体按 json 编码规则转换为 table
func structToLValue(vm *glua.LState, v any, depth int) (glua.LValue, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("convert %T: %w", v, err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("convert %T: %w", v, err)
	}
	return toLValueDepth(vm, decoded, depth+1)
}

// fromLValue 将 Lua 值转换为 Go 值：
// 整数值的 number 转为 int64，其余为 float64；键为 1..n 的 table 转为 []any，其余转为 map[string]any；
// userdata 返回其中保存的 Go 值；函数等无法转换的值原样返回。
func fromLValue(lv glua.LValue) (any, error) {
	return fromLValueDepth(lv, 0)
}

func fromLValueDepth(lv glua.LValue, depth int) (any, error) {
	if depth > maxConvertDepth {
		return nil, errTooDeep
	}

	switch v := lv.(type) {
	case nil, *glua.LNilType:
		return nil, nil
	case glua.LBool:
		return bool(v), nil
	case glua.LNumber:
		return numberToGo(v), nil
	case glua.LString:
		return string(v), nil
	case *glua.LTable:
		return tableToGo(v, depth)
	case *glua.LUserData:
		return v.Value, nil
	}
	return lv, nil
}

func numberToGo(n glua.LNumber) any {
	f := float64(n)
	if f == math.Trunc(f) && math.Abs(f) <= 1<<53 {
		return int64(f)
	}
	return f
}

func tableToGo(tbl *glua.LTable, depth int) (any, error) {
	total := 0
	isArray := true
	tbl.ForEach(func(key glua.LValue, _ glua.LValue) {
		total++
		n, ok := key.(glua.LNumber)
		if !ok || float64(n) != math.Trunc(float64(n)) || n < 1 {
			isArray = false
		}
	})

	if total > 0 && isArray {
		// 键均为正整数且互不相同，若最大键不超过总数则恰为 1..total
		items := make([]any, total)
		for i := 1; i <= total; i++ {
			item := tbl.RawGetInt(i)
			if item == glua.LNil {
				isArray = false
				break
			}
			value, err := fromLValueDepth(item, depth+1)
			if err != nil {
				return nil, err
			}
			items[i-1] = value
		}
		if isArray {
			return items, nil
		}
	}

	result := make(map[string]any, total)
	var firstErr error
	tbl.ForEach(func(key glua.LValue, value glua.LValue) {
		if firstErr != nil {
			return
		}
		item, err := fromLValueDepth(value, depth+1)
		if err != nil {
			firstErr = err
			return
		}
		result[tableKeyString(key)] = item
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return result, nil
}

func tableKeyString(key glua.LValue) string {
	switch k := key.(type) {
	case glua.LString:
		return string(k)
	case glua.LNumber:
		switch n := numberToGo(k).(type) {
		case int64:
			return strconv.FormatInt(n, 10)
		case float64:
			return strconv.FormatFloat(n, 'g', -1, 64)
		}
	}
	return key.String()
}

// callGoFunc 以 Lua 栈上的参数调用 Go 函数，并将返回值压栈
func callGoFunc(vm *glua.LState, fn reflect.Value) int {
	args := make([]any, vm.GetTop())
	for i := range args {
		value, err := fromLValue(vm.Get(i + 1))
		if err != nil {
			vm.RaiseError("argument #%d: %s", i+1, err.Error())
			return 0
		}
		args[i] = value
	}

	results, err := gofunc.Call(fn, args)
	if err != nil {
		vm.RaiseError("%s", err.Error())
		return 0
	}
	for _, result := range results {
		lv, err := toLValue(vm, result)
		if err != nil {
			vm.RaiseError("%s", err.Error())
			return 0
		}
		vm.Push(lv)
	}
	return len(results)
}

func registerCtxType(vm *glua.LState) {
	mt := vm.NewTypeMetatable(ctxTypeName)
	vm.SetField(mt, "__index", vm.SetFuncs(vm.NewTable(), map[string]glua.LGFunction{
		"get":         ctxGet,
		"set":         ctxSet,
		"instance_id": ctxInstanceID,
	}))
}

func newCtxUserData(vm *glua.LState, ctx glowflow.Context) *glua.LUserData {
	ud := vm.NewUserData()
	ud.Value = ctx
	vm.SetMetatable(ud, vm.GetTypeMetatable(ctxTypeName))
	return ud
}

func checkCtx(vm *glua.LState) glowflow.Context {
	ud := vm.CheckUserData(1)
	ctx, ok := ud.Value.(glowflow.Context)
	if !ok {
		vm.ArgError(1, "ctx expected")
	}
	return ctx
}

// ctx:get(key)
func ctxGet(vm *glua.LState) int {
	ctx := checkCtx(vm)
	lv, err := toLValue(vm, ctx.Get(vm.CheckString(2)))
	if err != nil {
		vm.RaiseError("%s", err.Error())
		return 0
	}
	vm.Push(lv)
	return 1
}

// ctx:set(key, value)
func ctxSet(vm *glua.LState) int {
	ctx := checkCtx(vm)
	key := vm.CheckString(2)
	value, err := fromLValue(vm.Get(3))
	if err != nil {
		vm.RaiseError("%s", err.Error())
		return 0
	}
	ctx.Set(key, value)
	return 0
}

// ctx:instance_id()
func ctxInstanceID(vm *glua.LState) int {
	ctx := checkCtx(vm)
	vm.Push(glua.LString(ctx.GetInstanceID()))
	return 1
}
