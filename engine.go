package glowflow

import (
	"context"
	"fmt"

	cmap "github.com/orcaman/concurrent-map/v2"
)

type Engine struct {
	options *options
	flows   cmap.ConcurrentMap[string, *flowSeries]
}

type flowSeries struct {
	versions      map[string]*flowRuntime
	latestVersion string
	paused        bool
}

type flowRuntime struct {
	definition ChainDefinition
	compiled   *CompiledFlow
}

// NewEngine creates a new engine with the given options.
func NewEngine(opts ...Option) *Engine {
	engine := &Engine{
		options: &options{},
		flows:   cmap.New[*flowSeries](),
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

func (e *Engine) Load(defs ...ChainDefinition) error {
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

func (e *Engine) Dispatch(flowID string, data any) (string, error) {
	series, ok := e.flows.Get(flowID)
	if !ok {
		return "", fmt.Errorf("flow not loaded: %s", flowID)
	}
	if series.paused {
		return "", fmt.Errorf("flow paused: %s", flowID)
	}
	version := series.latestVersion
	flow, ok := series.versions[version]
	if !ok {
		return "", fmt.Errorf("flow version not loaded: %s@%s", flowID, version)
	}
	compiled := flow.compiled

	instanceID, err := newInstanceID()
	if err != nil {
		return "", err
	}

	err = e.options.dispatcher.Dispatch(NewContext(context.Background(), instanceID), compiled, data)
	if err != nil {
		return "", err
	}
	return instanceID, nil
}

// Stop stops the engine.
func (e *Engine) Stop() error {
	return nil
}

func (e *Engine) Pause(flowID string) error {

	flow, ok := e.flows.Get(flowID)
	if !ok {
		return fmt.Errorf("flow not loaded: %s", flowID)
	}
	flow.paused = true
	return nil
}

func (e *Engine) Resume(flowID string) error {
	flow, ok := e.flows.Get(flowID)
	if !ok {
		return fmt.Errorf("flow not loaded: %s", flowID)
	}
	flow.paused = false
	return nil
}

func (e *Engine) Reload(def ChainDefinition) error {
	return e.loadLocked(def)
}

func (e *Engine) loadLocked(def ChainDefinition) error {
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
	series, ok := e.flows.Get(def.ID)
	if !ok {
		series = &flowSeries{versions: make(map[string]*flowRuntime)}
		e.flows.Set(def.ID, series)
	}
	series.versions[def.Version] = &flowRuntime{definition: def, compiled: compiled}
	series.latestVersion = def.Version
	return nil
}
