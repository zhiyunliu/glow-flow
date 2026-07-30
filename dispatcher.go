package glowflow

import (
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

type Dispatcher interface {
	Dispatch(ctx Context, flow *CompiledFlow, data any) (instanceID string, err error)
}

type dispatcher struct {
	asnycGroup errgroup.Group
}

func NewDispatcher() Dispatcher {
	return &dispatcher{}
}

func (d *dispatcher) Dispatch(ctx Context, flow *CompiledFlow, data any) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("context is nil")
	}
	if flow == nil {
		return "", fmt.Errorf("compiled flow is nil")
	}
	instanceID, err := newInstanceID()
	if err != nil {
		return "", err
	}
	ctx.Set("instance_id", instanceID)

	for _, node := range flow.StartNodes {
		d.asnycGroup.Go(d.asyncCall(ctx, flow, node, data))
	}

	return instanceID, nil
}

func (d *dispatcher) dispatchNode(ctx Context, flow *CompiledFlow, node CompiledNode, data any) error {
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
		d.asnycGroup.Go(d.asyncCall(ctx, flow, nextNode, result.Data))
	}
	return nil
}

func (d *dispatcher) asyncCall(ctx Context, flow *CompiledFlow, node CompiledNode, data any) func() error {
	return func() error {
		return d.dispatchNode(ctx, flow, node, data)
	}
}

func newInstanceID() (string, error) {
	return uuid.New().String(), nil
}
