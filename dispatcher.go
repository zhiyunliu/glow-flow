package glowflow

import (
	"fmt"

	"github.com/google/uuid"
)

type Dispatcher interface {
	Dispatch(ctx Context, flow *CompiledChain, msg *Message) (err error)
}

type dispatcher struct{}

var (
	DefaultDispatcher Dispatcher = NewDispatcher()
)

func NewDispatcher() Dispatcher {
	return &dispatcher{}
}

// Dispatch 从起始节点开始并行执行流程，等待所有分支结束后返回首个节点执行错误。
// 节点返回 error(或 ExecuteResult.Error)时结束所在分支，不影响其他分支。
func (d *dispatcher) Dispatch(ctx Context, flow *CompiledChain, msg *Message) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if flow == nil {
		return fmt.Errorf("compiled chain is nil")
	}
	return newScope(flow, executeOptions{}).run(ctx, flow.StartNodes, msg).Err
}

func newInstanceID() (string, error) {
	return uuid.New().String(), nil
}
