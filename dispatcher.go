package glowflow

import (
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

type Dispatcher interface {
	Dispatch(ctx Context, flow *CompiledChain, data any) (err error)
}

type dispatcher struct{}

var (
	DefaultDispatcher Dispatcher = NewDispatcher()
)

func NewDispatcher() Dispatcher {
	return &dispatcher{}
}

func (d *dispatcher) Dispatch(ctx Context, flow *CompiledChain, data any) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if flow == nil {
		return fmt.Errorf("compiled chain is nil")
	}
	var asyncGroup errgroup.Group

	for _, node := range flow.StartNodes {
		asyncGroup.Go(d.asyncCall(&asyncGroup, ctx, flow, node, data))
	}

	return asyncGroup.Wait()
}

func (d *dispatcher) dispatchNode(asyncGroup *errgroup.Group, ctx Context, flow *CompiledChain, node CompiledNode, data any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := node.Execute(ctx, data)
	if err != nil {
		return fmt.Errorf("execute node %s: %w", node.Id(), err)
	}
	if result.Error != nil {
		return fmt.Errorf("execute node %s: %w", node.Id(), result.Error)
	}

	nextNodes := flow.NextNodes(node.Id(), result.RelationType)
	for _, nextNode := range nextNodes {
		asyncGroup.Go(d.asyncCall(asyncGroup, ctx, flow, nextNode, result.Data))
	}
	return nil
}

func (d *dispatcher) asyncCall(asyncGroup *errgroup.Group, ctx Context, flow *CompiledChain, node CompiledNode, data any) func() error {
	return func() error {
		return d.dispatchNode(asyncGroup, ctx, flow, node, data)
	}
}

func newInstanceID() (string, error) {
	return uuid.New().String(), nil
}
