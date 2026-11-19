package python

import (
	"fmt"

	scriptengine "github.com/zhiyunliu/glow-flow/scriptengine"
	"github.com/zhiyunliu/glue/config"
)

const (
	Proto = "python"
)

type engineResolver struct {
}

func (s *engineResolver) Proto() string {
	return Proto
}

func (s *engineResolver) Resolve(name string,
	cfg config.Config) (scriptengine.Engine, error) {
	pyConfig := &Config{}
	err := cfg.ScanTo(pyConfig)
	if err != nil {
		return nil, fmt.Errorf("读取pythonengine配置[%s]错误%w", cfg.Path(), err)
	}
	return NewEngine(pyConfig)
}

func init() {
	scriptengine.Register(&engineResolver{})
}
