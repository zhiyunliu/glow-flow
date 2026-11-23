package action

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	glowflow "github.com/zhiyunliu/glow-flow"
	gluectx "github.com/zhiyunliu/glue/context"
	"github.com/zhiyunliu/glue/xhttp"
	"github.com/zhiyunliu/glue/xrpc"
	"github.com/zhiyunliu/golibs/xtypes"
)

type fakeBody struct {
	status int32
	result []byte
}

func (b fakeBody) GetStatus() int32             { return b.status }
func (b fakeBody) GetHeader() map[string]string { return nil }
func (b fakeBody) GetResult() []byte            { return b.result }

type remoteCallRecord struct {
	client      string
	service     string
	input       any
	method      string
	headers     map[string]string
	hasDeadline bool
}

// fakeRemote 同时实现 xhttp.Client 与 xrpc.Client
type fakeRemote struct {
	mu     sync.Mutex
	name   string
	calls  []remoteCallRecord
	status int32
	result string
	err    error
}

func (f *fakeRemote) record(ctx context.Context, service string, input any, method string, headers map[string]string) (fakeBody, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, hasDeadline := ctx.Deadline()
	f.calls = append(f.calls, remoteCallRecord{
		client: f.name, service: service, input: input, method: method, headers: headers, hasDeadline: hasDeadline,
	})
	return fakeBody{status: f.status, result: []byte(f.result)}, f.err
}

func (f *fakeRemote) lastCall(t *testing.T) remoteCallRecord {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("no remote call recorded")
	}
	return f.calls[len(f.calls)-1]
}

type fakeHTTPClient struct{ *fakeRemote }

func (c fakeHTTPClient) Swap(gluectx.Context, string, ...xhttp.RequestOption) (xhttp.Body, error) {
	return nil, errors.New("not implemented")
}

func (c fakeHTTPClient) Request(ctx context.Context, service string, input any, opts ...xhttp.RequestOption) (xhttp.Body, error) {
	o := &xhttp.Options{}
	for _, opt := range opts {
		opt(o)
	}
	return c.record(ctx, service, input, o.Method, o.Header)
}

type fakeRPCClient struct{ *fakeRemote }

func (c fakeRPCClient) Swap(gluectx.Context, string, ...xrpc.RequestOption) (xrpc.Body, error) {
	return nil, errors.New("not implemented")
}

func (c fakeRPCClient) Request(ctx context.Context, service string, input any, opts ...xrpc.RequestOption) (xrpc.Body, error) {
	o := &xrpc.Options{}
	for _, opt := range opts {
		opt(o)
	}
	return c.record(ctx, service, input, o.Method, o.Header)
}

func useFakeHTTP(t *testing.T, fake *fakeRemote) {
	old := httpClientResolver
	httpClientResolver = func(name string) xhttp.Client {
		fake.name = name
		return fakeHTTPClient{fake}
	}
	t.Cleanup(func() { httpClientResolver = old })
}

func useFakeRPC(t *testing.T, fake *fakeRemote) {
	old := rpcClientResolver
	rpcClientResolver = func(name string) xrpc.Client {
		fake.name = name
		return fakeRPCClient{fake}
	}
	t.Cleanup(func() { rpcClientResolver = old })
}

func orderMessage() *glowflow.Message {
	return glowflow.NewMessage("msg-1", map[string]any{"order_no": "A1", "tenant": "t1", "status": "paid"})
}

func TestHttpCallSuccess(t *testing.T) {
	fake := &fakeRemote{status: 200, result: `{"ok":true,"id":7}`}
	useFakeHTTP(t, fake)
	node := buildNode(t, HttpCallNodeType, xtypes.XMap{
		"clientname": "order",
		"service":    "http://order-api/orders/${order_no}",
		"headers":    map[string]any{"X-Tenant": "${tenant}"},
		"timeout":    "5s",
	})

	msg := orderMessage()
	result := execute(t, node, msg)
	assertSuccessData(t, result, msg, map[string]any{"ok": true, "id": float64(7)})

	call := fake.lastCall(t)
	if call.client != "order" || call.service != "http://order-api/orders/A1" || call.method != "POST" {
		t.Fatalf("call = %+v", call)
	}
	if call.headers["X-Tenant"] != "t1" || !call.hasDeadline {
		t.Fatalf("headers/deadline = %v/%v", call.headers, call.hasDeadline)
	}
	if !reflect.DeepEqual(call.input, msg.Data) {
		t.Fatalf("input = %#v, want message data", call.input)
	}
	if msg.Data.(map[string]any)["order_no"] != "A1" {
		t.Fatalf("input message should not be modified")
	}
}

func TestHttpCallGetSendsNoBody(t *testing.T) {
	fake := &fakeRemote{status: 200, result: "pong"}
	useFakeHTTP(t, fake)
	node := buildNode(t, HttpCallNodeType, xtypes.XMap{"service": "http://api/ping?id=${order_no}", "method": "get"})

	msg := orderMessage()
	assertSuccessData(t, execute(t, node, msg), msg, "pong")
	call := fake.lastCall(t)
	if call.client != defaultClientName || call.method != "GET" || call.input != "" || call.hasDeadline {
		t.Fatalf("call = %+v", call)
	}
	if call.service != "http://api/ping?id=A1" {
		t.Fatalf("service = %q", call.service)
	}
}

func TestHttpCallFailures(t *testing.T) {
	fake := &fakeRemote{status: 500, result: "oops"}
	useFakeHTTP(t, fake)
	node := buildNode(t, HttpCallNodeType, xtypes.XMap{"service": "http://api/orders"})

	msg := orderMessage()
	assertFailure(t, execute(t, node, msg), msg)

	fake.status, fake.err = 0, errors.New("connection refused")
	assertFailure(t, execute(t, node, msg), msg)

	assertInvalidConfig(t, HttpCallNodeType, xtypes.XMap{})
	assertInvalidConfig(t, HttpCallNodeType, xtypes.XMap{"service": "http://api", "timeout": "abc"})
	assertInvalidConfig(t, HttpCallNodeType, xtypes.XMap{"service": "http://api", "headers": []any{"x"}})
}

func TestRpcCall(t *testing.T) {
	fake := &fakeRemote{status: 200, result: `[1,2]`}
	useFakeRPC(t, fake)
	node := buildNode(t, RpcCallNodeType, xtypes.XMap{
		"clientname": "rpc",
		"service":    "grpc://order-service/order/${status}",
		"headers":    `{"X-Tenant":"${tenant}"}`,
	})

	msg := orderMessage()
	assertSuccessData(t, execute(t, node, msg), msg, []any{float64(1), float64(2)})
	call := fake.lastCall(t)
	if call.client != "rpc" || call.service != "grpc://order-service/order/paid" || call.method != "" {
		t.Fatalf("call = %+v", call)
	}
	if call.headers["X-Tenant"] != "t1" || !reflect.DeepEqual(call.input, msg.Data) {
		t.Fatalf("headers/input = %v/%#v", call.headers, call.input)
	}

	fake.status = 404
	assertFailure(t, execute(t, node, msg), msg)
	assertInvalidConfig(t, RpcCallNodeType, xtypes.XMap{"service": " "})
}
