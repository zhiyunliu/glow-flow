package config

import (
	"github.com/zhiyunliu/glue/config"
)

const (
	defaultFulfillChain = "order_fulfill"
	defaultRunTimeout   = 30
)

// Config 样例业务配置，对应配置文件中的 flowapp 节点
type Config struct {
	FulfillChain string `json:"fulfill_chain"` // 消费下单事件时执行的流程
	RunTimeout   int    `json:"run_timeout"`   // 单次流程执行的最长时间(秒)，超时后未完成的节点收到取消信号
}

var Sys = &Config{
	FulfillChain: defaultFulfillChain,
	RunTimeout:   defaultRunTimeout,
}

// Load 从 glue 配置的 flowapp 节点加载业务配置，未配置的字段保留默认值
func Load(cfg config.Config) error {
	node := cfg.Get("flowapp")
	if node == nil {
		return nil
	}
	if err := node.ScanTo(Sys); err != nil {
		return err
	}
	if Sys.FulfillChain == "" {
		Sys.FulfillChain = defaultFulfillChain
	}
	if Sys.RunTimeout <= 0 {
		Sys.RunTimeout = defaultRunTimeout
	}
	return nil
}
