package lua

import (
	"fmt"

	scriptengine "github.com/zhiyunliu/glow-flow/scriptengine"
	"github.com/zhiyunliu/glue/config"
)

const (
	Proto = "lua"
)

type engineResolver struct {
}

func (s *engineResolver) Proto() string {
	return Proto
}

func (s *engineResolver) Resolve(name string,
	cfg config.Config) (scriptengine.Engine, error) {
	luaConfig := &Config{}
	err := cfg.ScanTo(luaConfig)
	if err != nil {
		return nil, fmt.Errorf("读取luaengine配置[%s]错误%w", cfg.Path(), err)
	}
	return NewEngine(luaConfig)
}

func init() {
	scriptengine.Register(&engineResolver{})
}
