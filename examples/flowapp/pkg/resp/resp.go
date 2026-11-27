// Package resp 统一响应：成功返回 {"code":0,"message":"成功","data":...}；
// 失败返回业务错误，由 glue 输出 {"code":...,"sub_code":...,"message":...} 及对应的 HTTP 状态码。
package resp

import (
	"github.com/zhiyunliu/glow-flow/examples/flowapp/constants/respcode"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/constants/subcode"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/pkg/bizerr"
)

// Result 成功响应体
type Result struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Success 成功响应
func Success(data ...any) any {
	result := Result{Code: respcode.Success, Message: "成功"}
	if len(data) > 0 {
		result.Data = data[0]
	}
	return result
}

// Error 失败响应：业务错误原样返回，其他错误转换为系统错误
func Error(err error) any {
	if err == nil {
		return Success()
	}
	if xerr, ok := bizerr.IsBiz(err); ok {
		return xerr
	}
	return bizerr.System(err)
}

// BindError 参数绑定或校验失败
func BindError(err error) any {
	return bizerr.BadRequest(subcode.ParamInvalid, "参数错误：%s", err.Error())
}
