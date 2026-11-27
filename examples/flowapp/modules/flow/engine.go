package flow

import (
	"context"
	"sort"
	"sync"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/repository/jsonfile"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/models"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/modules/flow/trace"
	"github.com/zhiyunliu/glue/log"
)

// flowEngine 流程引擎实例：从 jsonfile 仓储(工作目录下的 glowflow/*.json)加载流程定义
type flowEngine struct {
	repo   glowflow.DataRepository
	engine *glowflow.Engine

	mu     sync.RWMutex
	chains map[string]models.ChainInfo
}

var std *flowEngine

// Init 服务启动时创建引擎并加载全部流程。
// 仓储读取失败时阻止启动；个别流程编译失败只记录日志，修正后可调用 /flow/reload 重新加载。
func Init(ctx context.Context, logger log.Logger) error {
	repo := jsonfile.NewRepository()
	eng := &flowEngine{
		repo: repo,
		// 包装内置节点注册表以记录执行轨迹；流程编译、执行与默认注册表一致
		engine: glowflow.NewEngine(
			glowflow.WithDataRepository(repo),
			glowflow.WithRegistry(trace.NewRegistry(glowflow.DefaultRegistry)),
		),
		chains: make(map[string]models.ChainInfo),
	}
	result, err := eng.reload(ctx)
	if err != nil {
		return err
	}
	for chainID, msg := range result.Failed {
		logger.Errorf("load flow %s failed: %s", chainID, msg)
	}
	logger.Infof("loaded %d flows: %v", len(result.Loaded), result.Loaded)
	std = eng
	return nil
}

// reload 从仓储重新加载全部流程定义
func (e *flowEngine) reload(ctx context.Context) (models.ReloadResult, error) {
	result := models.ReloadResult{Loaded: []string{}}
	defs, err := e.repo.LoadChainDefinitions(ctx)
	if err != nil {
		return result, err
	}
	for _, def := range defs {
		if err := e.engine.Load(*def); err != nil {
			if result.Failed == nil {
				result.Failed = make(map[string]string)
			}
			result.Failed[def.ID] = err.Error()
			continue
		}
		e.mu.Lock()
		e.chains[def.ID] = models.ChainInfo{ID: def.ID, Name: def.Metadata.Name, Version: def.Version, Nodes: len(def.Nodes)}
		e.mu.Unlock()
		result.Loaded = append(result.Loaded, def.ID)
	}
	sort.Strings(result.Loaded)
	return result, nil
}

func (e *flowEngine) loaded(chainID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.chains[chainID]
	return ok
}

func (e *flowEngine) list() []models.ChainInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	list := make([]models.ChainInfo, 0, len(e.chains))
	for _, info := range e.chains {
		list = append(list, info)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}
