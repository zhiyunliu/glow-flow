package action

import glowflow "github.com/zhiyunliu/glow-flow"

type nodeRpcCall struct {
}

func (n *nodeRpcCall) Call[T any](ctx glowflow.Context, method string, args ...any) {

}
