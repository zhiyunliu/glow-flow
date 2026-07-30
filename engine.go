package glowflow

import (
	"context"
	"fmt"
	"sync"
)

type Engine struct {
	options *options
	mu      sync.RWMutex
	flows   map[string]*flowSeries
}

type flowSeries struct {
	versions      map[string]*flowRuntime
	latestVersion string
	paused        bool
}

type flowRuntime struct {
	definition FlowDefinition
	compiled   *CompiledFlow
}

// NewEngine creates a new engine with the given options.
func NewEngine(opts ...Option) *Engine {
	engine := &Engine{
		options: &options{},
		flows:   make(map[string]*flowSeries),
	}
	for _, opt := range opts {
		opt(engine.options)
	}
	if engine.options.registry == nil {
		engine.options.registry = NewRegistry()
	}
	if engine.options.compiler == nil {
		engine.options.compiler = NewFlowCompiler()
	}
	if engine.options.dispatcher == nil {
		engine.options.dispatcher = NewDispatcher()
	}
	return engine
}

func (e *Engine) Load(defs ...FlowDefinition) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, def := range defs {
		if err := e.loadLocked(def); err != nil {
			return err
		}
	}
	return nil
}

// Run runs the engine.
func (e *Engine) Run() error {
	return nil
}

func (e *Engine) Dispatch(flowID string, data any) error {
	e.mu.RLock()
	series, ok := e.flows[flowID]
	if !ok {
		e.mu.RUnlock()
		return fmt.Errorf("flow not loaded: %s", flowID)
	}
	version := series.latestVersion
	e.mu.RUnlock()
	return e.DispatchVersion(flowID, version, data)
}

func (e *Engine) DispatchVersion(flowID string, version string, data any) error {
	e.mu.RLock()
	series, ok := e.flows[flowID]
	if !ok {
		e.mu.RUnlock()
		return fmt.Errorf("flow not loaded: %s", flowID)
	}
	if series.paused {
		e.mu.RUnlock()
		return fmt.Errorf("flow paused: %s", flowID)
	}
	flow, ok := series.versions[version]
	if !ok {
		e.mu.RUnlock()
		return fmt.Errorf("flow version not loaded: %s@%s", flowID, version)
	}
	compiled := flow.compiled
	e.mu.RUnlock()

	return e.options.dispatcher.Dispatch(NewContext(context.Background()), compiled, data)
}

// Stop stops the engine.
func (e *Engine) Stop() error {
	return nil
}

func (e *Engine) Pause(flowID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	flow, ok := e.flows[flowID]
	if !ok {
		return fmt.Errorf("flow not loaded: %s", flowID)
	}
	flow.paused = true
	return nil
}

func (e *Engine) Resume(flowID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	flow, ok := e.flows[flowID]
	if !ok {
		return fmt.Errorf("flow not loaded: %s", flowID)
	}
	flow.paused = false
	return nil
}

func (e *Engine) Reload(def FlowDefinition) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.loadLocked(def)
}

func (e *Engine) loadLocked(def FlowDefinition) error {
	if def.ID == "" {
		return fmt.Errorf("flow id is empty")
	}
	if def.Version == "" {
		return fmt.Errorf("flow version is empty")
	}
	compiled, err := e.options.compiler.Compile(def, e.options.registry)
	if err != nil {
		return err
	}
	series, ok := e.flows[def.ID]
	if !ok {
		series = &flowSeries{versions: make(map[string]*flowRuntime)}
		e.flows[def.ID] = series
	}
	series.versions[def.Version] = &flowRuntime{definition: def, compiled: compiled}
	series.latestVersion = def.Version
	return nil
}
