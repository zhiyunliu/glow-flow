package goja

import (
	"fmt"

	scriptengine "github.com/zhiyunliu/glow-flow/scriptengine"
	"github.com/zhiyunliu/glue/config"
)

const (
	Proto = "goja"
)

type engineResolver struct {
}

func (s *engineResolver) Proto() string {
	return Proto
}

func (s *engineResolver) Resolve(name string,
	cfg config.Config) (scriptengine.Engine, error) {
	jsconfig := &Config{}
	err := cfg.ScanTo(jsconfig)
	if err != nil {
		return nil, fmt.Errorf("读取jsengine配置[%s]错误%w", cfg.Path(), err)
	}
	return NewEngine(jsconfig)
}

func init() {
	scriptengine.Register(&engineResolver{})
}
