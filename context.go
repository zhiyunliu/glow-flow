package glowflow

import (
	"context"
	"time"
)

type Context interface {
	context.Context
	GetInstanceID() string
	Get(key string) any
	Set(key string, value any)
}

func NewContext(ctx context.Context, instanceID string) Context {
	return &contextImpl{instanceID: instanceID, ctx: ctx}
}

// WithTimeout 派生带超时的流程上下文，保留 parent 中的值及执行作用域
func WithTimeout(parent Context, timeout time.Duration) (Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	return NewContext(ctx, parent.GetInstanceID()), cancel
}

// WithCancel 派生可取消的流程上下文，保留 parent 中的值及执行作用域
func WithCancel(parent Context) (Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	return NewContext(ctx, parent.GetInstanceID()), cancel
}

type contextImpl struct {
	instanceID string
	ctx        context.Context
}

func (c *contextImpl) GetInstanceID() string {
	return c.instanceID
}

func (c *contextImpl) Get(key string) any {
	return c.ctx.Value(key)
}
func (c *contextImpl) Set(key string, value any) {
	c.ctx = context.WithValue(c.ctx, key, value)
}

func (c *contextImpl) Value(key any) any {
	return c.ctx.Value(key)
}

func (c *contextImpl) Deadline() (deadline time.Time, ok bool) {
	return c.ctx.Deadline()
}

func (c *contextImpl) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *contextImpl) Err() error {
	return c.ctx.Err()
}
