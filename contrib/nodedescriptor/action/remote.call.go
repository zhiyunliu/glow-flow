package action

import (
	"context"
	"strings"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/utility"
)

const defaultClientName = "default"

// remoteCall http/rpc 调用节点的公共配置：
//   - clientname：glue 客户端配置名称，默认 default
//   - service：请求地址，必填，支持 ${} 模板
//   - method：请求方法
//   - headers：请求头(对象)，值支持 ${} 模板
//   - timeout：超时时间，如 5s 或秒数，默认不限制(受流程 context 控制)
type remoteCall struct {
	clientName string
	service    utility.Template
	method     string
	headers    map[string]utility.Template
	timeout    time.Duration
}

// remoteRequest 渲染后的单次请求参数
type remoteRequest struct {
	service string
	headers map[string]string
}

func newRemoteCall(def glowflow.NodeDefinition, defaultMethod string) (remoteCall, error) {
	ext := def.ExtParams
	call := remoteCall{
		clientName: strings.TrimSpace(ext.GetString("clientname")),
		method:     strings.ToUpper(strings.TrimSpace(ext.GetString("method"))),
	}
	if call.clientName == "" {
		call.clientName = defaultClientName
	}
	if call.method == "" {
		call.method = defaultMethod
	}

	service := strings.TrimSpace(ext.GetString("service"))
	if service == "" {
		return call, invalidConfig(def, "service is empty")
	}
	var err error
	if call.service, err = compileTemplate(def, "service", service); err != nil {
		return call, err
	}

	headers, err := getStringMap(ext, "headers")
	if err != nil {
		return call, invalidConfig(def, "%v", err)
	}
	if call.headers, err = compileTemplateMap(def, "headers", headers); err != nil {
		return call, err
	}

	if call.timeout, err = getDuration(ext, "timeout", 0); err != nil {
		return call, invalidConfig(def, "%v", err)
	}
	return call, nil
}

// prepare 渲染请求参数并创建带超时的 context
func (c *remoteCall) prepare(ctx glowflow.Context, msg *glowflow.Message) (context.Context, context.CancelFunc, *remoteRequest, error) {
	env := templateEnv(ctx, msg)
	service, err := renderTemplate(c.service, env)
	if err != nil {
		return nil, nil, nil, err
	}
	headers, err := renderMap(c.headers, env)
	if err != nil {
		return nil, nil, nil, err
	}
	reqCtx, cancel := withTimeout(ctx, c.timeout)
	return reqCtx, cancel, &remoteRequest{service: service, headers: headers}, nil
}
