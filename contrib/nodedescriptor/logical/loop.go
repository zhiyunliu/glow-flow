package logical

import (
	"sync/atomic"

	glowflow "github.com/zhiyunliu/glow-flow"
)

// LoopState 循环节点(for/while)当前迭代的信息
type LoopState struct {
	Index int    // 迭代序号，从 0 开始
	Key   string // 遍历 map 时的键
	Item  any    // 当前迭代项
}

type loopKey struct{}

// loopControl 一次迭代的循环状态，通过上下文传递给循环体中的节点
type loopControl struct {
	state  LoopState
	broken atomic.Bool
}

// loopContext 将循环状态附加到流程上下文；Get/Set 等方法仍作用于原上下文
type loopContext struct {
	glowflow.Context
	ctl *loopControl
}

func (c *loopContext) Value(key any) any {
	if _, ok := key.(loopKey); ok {
		return c.ctl
	}
	return c.Context.Value(key)
}

// withLoop 派生迭代上下文：循环体节点收到的上下文派生自它，因此可找到最近一层循环
func withLoop(ctx glowflow.Context, state LoopState) (glowflow.Context, *loopControl) {
	ctl := &loopControl{state: state}
	return &loopContext{Context: ctx, ctl: ctl}, ctl
}

func loopFrom(ctx glowflow.Context) *loopControl {
	if ctx == nil {
		return nil
	}
	ctl, _ := ctx.Value(loopKey{}).(*loopControl)
	return ctl
}

// CurrentLoop 返回最近一层循环的迭代信息；不在循环体中时返回 false
func CurrentLoop(ctx glowflow.Context) (LoopState, bool) {
	if ctl := loopFrom(ctx); ctl != nil {
		return ctl.state, true
	}
	return LoopState{}, false
}

// BreakLoop 标记最近一层循环在本次迭代结束后退出；不在循环体中时返回 false
func BreakLoop(ctx glowflow.Context) bool {
	if ctl := loopFrom(ctx); ctl != nil {
		ctl.broken.Store(true)
		return true
	}
	return false
}
