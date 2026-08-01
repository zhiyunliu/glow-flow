package glowflow

import (
	"context"
	"fmt"

	cmap "github.com/orcaman/concurrent-map/v2"
)

type Engine struct {
	options *options
	chains  cmap.ConcurrentMap[string, *chainSeries]
}

type chainSeries struct {
	versions      map[string]*chainRuntime
	latestVersion string
	paused        bool
}

type chainRuntime struct {
	definition ChainDefinition
	compiled   *CompiledChain
}

// NewEngine creates a new engine with the given options.
func NewEngine(opts ...Option) *Engine {
	engine := &Engine{
		options: &options{},
		chains:  cmap.New[*chainSeries](),
	}
	for _, opt := range opts {
		opt(engine.options)
	}
	if engine.options.registry == nil {
		engine.options.registry = NewRegistry()
	}
	if engine.options.compiler == nil {
		engine.options.compiler = NewChainCompiler()
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

func (e *Engine) Dispatch(chainID string, data any) (string, error) {
	series, ok := e.chains.Get(chainID)
	if !ok {
		return "", fmt.Errorf("flow not loaded: %s", chainID)
	}
	if series.paused {
		return "", fmt.Errorf("flow paused: %s", chainID)
	}
	version := series.latestVersion
	flow, ok := series.versions[version]
	if !ok {
		return "", fmt.Errorf("flow version not loaded: %s@%s", chainID, version)
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

func (e *Engine) Pause(chainID string) error {

	flow, ok := e.chains.Get(chainID)
	if !ok {
		return fmt.Errorf("chain not loaded: %s", chainID)
	}
	flow.paused = true
	return nil
}

func (e *Engine) Resume(chainID string) error {
	flow, ok := e.chains.Get(chainID)
	if !ok {
		return fmt.Errorf("chain not loaded: %s", chainID)
	}
	flow.paused = false
	return nil
}

func (e *Engine) Reload(def ChainDefinition) error {
	return e.loadLocked(def)
}

func (e *Engine) loadLocked(def ChainDefinition) error {
	if def.ID == "" {
		return fmt.Errorf("chain id is empty")
	}
	if def.Version == "" {
		return fmt.Errorf("chain version is empty")
	}
	compiled, err := e.options.compiler.Compile(def, e.options.registry)
	if err != nil {
		return err
	}
	series, ok := e.chains.Get(def.ID)
	if !ok {
		series = &chainSeries{versions: make(map[string]*chainRuntime)}
		e.chains.Set(def.ID, series)
	}
	series.versions[def.Version] = &chainRuntime{definition: def, compiled: compiled}
	series.latestVersion = def.Version
	return nil
}
