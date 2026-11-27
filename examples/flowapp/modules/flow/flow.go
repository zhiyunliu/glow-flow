package flow

import (
	"context"
	"time"

	"github.com/zhiyunliu/glow-flow/examples/flowapp/config"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/constants/subcode"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/models"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/modules/flow/trace"
	"github.com/zhiyunliu/glow-flow/examples/flowapp/pkg/bizerr"
	"github.com/zhiyunliu/glue/log"
)

// Flower 流程执行：执行流程并返回结果与节点执行轨迹，管理已加载的流程
type Flower interface {
	// Run 同步执行流程，流程全部分支结束(或超时)后返回
	Run(ctx context.Context, chainID string, data any) (models.RunResult, error)
	// List 已加载的流程
	List(ctx context.Context) ([]models.ChainInfo, error)
	// Reload 从仓储重新加载全部流程定义
	Reload(ctx context.Context) (models.ReloadResult, error)
}

type FlowerImpl struct {
	logger log.Logger
}

func NewFlower(logger log.Logger) Flower {
	return &FlowerImpl{logger: logger}
}

func (svc *FlowerImpl) Run(ctx context.Context, chainID string, data any) (models.RunResult, error) {
	result := models.RunResult{ChainID: chainID, Results: []any{}, Trace: []models.TraceEvent{}}
	eng, err := current()
	if err != nil {
		return result, err
	}
	if !eng.loaded(chainID) {
		return result, bizerr.NotFound(subcode.ChainNotFound, "流程不存在或未加载：%s", chainID)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(config.Sys.RunTimeout)*time.Second)
	defer cancel()
	rec := trace.NewRecorder()
	start := time.Now()
	instanceID, err := eng.engine.Dispatch(trace.WithRecorder(ctx, rec), chainID, data)

	result.InstanceID = instanceID
	result.CostMs = time.Since(start).Milliseconds()
	result.Results = rec.Results()
	result.Trace = rec.Events()
	if err != nil {
		// 执行错误随结果返回，便于结合轨迹定位
		svc.logger.Errorf("run flow %s failed, err:%+v", chainID, err)
		result.Error = err.Error()
	}
	return result, nil
}

func (svc *FlowerImpl) List(ctx context.Context) ([]models.ChainInfo, error) {
	eng, err := current()
	if err != nil {
		return nil, err
	}
	return eng.list(), nil
}

func (svc *FlowerImpl) Reload(ctx context.Context) (models.ReloadResult, error) {
	eng, err := current()
	if err != nil {
		return models.ReloadResult{}, err
	}
	result, err := eng.reload(ctx)
	if err != nil {
		svc.logger.Errorf("reload flows failed, err:%+v", err)
		return result, err
	}
	for chainID, msg := range result.Failed {
		svc.logger.Errorf("load flow %s failed: %s", chainID, msg)
	}
	return result, nil
}

func current() (*flowEngine, error) {
	if std == nil {
		return nil, bizerr.Unavailable(subcode.EngineNotInit, "流程引擎未初始化")
	}
	return std, nil
}
