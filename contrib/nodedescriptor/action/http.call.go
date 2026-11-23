package action

import (
	"fmt"
	"net/http"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glue"
	"github.com/zhiyunliu/glue/xhttp"
)

const HttpCallNodeType = "httpcall"

/*
http请求调用(节点类型 httpcall)
使用
obj:=glue.Http(clientname)
res,err:= obj.Request(ctx,service,msg.data)

service支持：http,https,xhttp，例如 http://host/api/orders/${order_no}

extparams 配置：
  - clientname：glue http 客户端配置名称，默认 default
  - service：请求地址，必填，支持 ${} 模板
  - method：请求方法，默认 POST；GET/HEAD/DELETE 请求不发送请求体，参数请通过 service 模板拼接
  - headers：请求头(对象)，值支持 ${} 模板
  - timeout：超时时间，如 5s 或秒数

响应状态码为 2xx 时走 success 分支，响应内容(JSON 自动解析，否则为字符串)作为下游消息数据；
请求失败或状态码非 2xx 时走 failure 分支，并原样传递消息。
*/
type nodeHttpCall struct {
	glowflow.Node
	remoteCall
}

// httpClientResolver 获取 glue http 客户端，测试时可替换
var httpClientResolver = func(name string) xhttp.Client {
	return glue.Http(name)
}

func (n *nodeHttpCall) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	reqCtx, cancel, req, err := n.prepare(ctx, msg)
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}
	defer cancel()

	opts := []xhttp.RequestOption{xhttp.WithMethod(n.method)}
	if len(req.headers) > 0 {
		opts = append(opts, xhttp.WithHeaders(req.headers))
	}

	var input any = messageData(msg)
	if input == nil || !httpMethodHasBody(n.method) {
		input = ""
	}

	body, err := httpClientResolver(n.clientName).Request(reqCtx, req.service, input, opts...)
	if err != nil {
		return failureResult(ctx, n, msg, fmt.Errorf("http %s %s: %w", n.method, req.service, err))
	}
	data, err := decodeResponse(body.GetStatus(), body.GetResult())
	if err != nil {
		return failureResult(ctx, n, msg, fmt.Errorf("http %s %s: %w", n.method, req.service, err))
	}
	return successResult(msg, data), nil
}

func httpMethodHasBody(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		return false
	}
	return true
}

type httpCallFactory struct{}

func (f *httpCallFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	call, err := newRemoteCall(def, http.MethodPost)
	if err != nil {
		return nil, err
	}
	return &nodeHttpCall{
		Node:       def,
		remoteCall: call,
	}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    HttpCallNodeType,
		Factory: &httpCallFactory{},
	})
}
