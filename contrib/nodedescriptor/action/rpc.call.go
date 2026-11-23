package action

import (
	"fmt"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glue"
	"github.com/zhiyunliu/glue/xrpc"
)

const RpcCallNodeType = "rpccall"

/*
rpc远程调用(节点类型 rpccall)
使用
obj:=glue.RPC(clientname)
res,err:= obj.Request(ctx,service,msg.data)

extparams 配置：
  - clientname：glue rpc 客户端配置名称，默认 default
  - service：服务地址，必填，支持 ${} 模板，例如 grpc://order-service/api/order/query
  - method：请求方法，默认由 rpc 客户端决定(POST)
  - headers：请求头(对象)，值支持 ${} 模板
  - timeout：超时时间，如 5s 或秒数

响应状态码为 2xx 时走 success 分支，响应内容(JSON 自动解析，否则为字符串)作为下游消息数据；
请求失败或状态码非 2xx 时走 failure 分支，并原样传递消息。
*/
type nodeRpcCall struct {
	glowflow.Node
	remoteCall
}

// rpcClientResolver 获取 glue rpc 客户端，测试时可替换
var rpcClientResolver = func(name string) xrpc.Client {
	return glue.RPC(name)
}

func (n *nodeRpcCall) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	reqCtx, cancel, req, err := n.prepare(ctx, msg)
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}
	defer cancel()

	opts := []xrpc.RequestOption{xrpc.WithMethod(n.method)}
	if len(req.headers) > 0 {
		opts = append(opts, xrpc.WithHeaders(req.headers))
	}

	body, err := rpcClientResolver(n.clientName).Request(reqCtx, req.service, messageData(msg), opts...)
	if err != nil {
		return failureResult(ctx, n, msg, fmt.Errorf("rpc %s: %w", req.service, err))
	}
	data, err := decodeResponse(body.GetStatus(), body.GetResult())
	if err != nil {
		return failureResult(ctx, n, msg, fmt.Errorf("rpc %s: %w", req.service, err))
	}
	return successResult(msg, data), nil
}

type rpcCallFactory struct{}

func (f *rpcCallFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	call, err := newRemoteCall(def, "")
	if err != nil {
		return nil, err
	}
	return &nodeRpcCall{
		Node:       def,
		remoteCall: call,
	}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    RpcCallNodeType,
		Factory: &rpcCallFactory{},
	})
}
