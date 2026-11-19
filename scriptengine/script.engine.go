package scriptengine

import (
	"fmt"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glue/config"
	"github.com/zhiyunliu/glue/log"
)

type Config struct {
	JsScript string
	FromVars map[string]any
}

type Engine interface {
	Proto() string
	Execute(ctx glowflow.Context, functionName string, argumentList ...any) (any, error)
	Stop()
}

type EngineResolver interface {
	Proto() string
	Resolve(name string,
		cfg config.Config) (Engine, error)
}

var engineResolvers = make(map[string]EngineResolver)

// Register 注册配置文件适配器
func Register(resolver EngineResolver) {
	proto := resolver.Proto()
	if _, ok := engineResolvers[proto]; ok {
		log.Warn(fmt.Errorf("jsengine: 重复注册:%s", proto))
	}
	engineResolvers[proto] = resolver
}

// Deregister 清理配置适配器
func Deregister(name string) {
	delete(engineResolvers, name)
}

// NewEngine 根据适配器名称及参数返回配置处理器
func NewEngine(proto string,
	cfg config.Config) (Engine, error) {

	resolver, ok := engineResolvers[proto]
	if !ok {
		return nil, fmt.Errorf("jsengine: 未知的协议类型:%s", proto)
	}
	return resolver.Resolve(proto, cfg)
}
