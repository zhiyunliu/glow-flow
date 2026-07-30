package glowflow

import "fmt"

type Dispatcher interface {
	Dispatch(ctx Context, flow *CompiledFlow, data any) error
}

type dispatcher struct{}

func NewDispatcher() Dispatcher {
	return &dispatcher{}
}

func (d *dispatcher) Dispatch(ctx Context, flow *CompiledFlow, data any) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if flow == nil {
		return fmt.Errorf("compiled flow is nil")
	}
	for _, node := range flow.StartNodes {
		if err := d.dispatchNode(ctx, flow, node, data); err != nil {
			return err
		}
	}
	return nil
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
	for _, nextNode := range flow.NextNodes(node.Id(), result.RelationType) {
		if err := d.dispatchNode(ctx, flow, nextNode, result.Data); err != nil {
			return err
		}
	}
	return nil
}
