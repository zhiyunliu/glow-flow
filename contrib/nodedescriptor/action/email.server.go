package action

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/zhiyunliu/glue/global"
)

// EmailServerConfigKey glue 配置中 SMTP 服务器列表的节点名称
const EmailServerConfigKey = "emails"

// EmailServer SMTP 服务器配置，默认对应 glue 配置 emails 数组中的一项：
//
//	"emails": [
//	  {"smtpname":"test1","host":"smtp.example.com","port":465,"ssl":true,
//	   "username":"noreply@example.com","password":"***","from":"通知 <noreply@example.com>"}
//	]
type EmailServer struct {
	SmtpName string `json:"smtpname"` // 配置名称，邮件节点 extparams.smtpname 引用该值
	Host     string `json:"host"`     // SMTP 服务器地址
	Port     int    `json:"port"`     // 端口，默认 ssl=true 时 465，否则 25
	SSL      bool   `json:"ssl"`      // 是否 SSL/TLS 直连；为 false 时若服务器支持则自动 STARTTLS
	Username string `json:"username"` // 认证账号，为空时不认证
	Password string `json:"password"` // 密码/授权码
	From     string `json:"from"`     // 默认发件人，为空时使用 username
}

// EmailServerProvider 按名称获取 SMTP 服务器配置，可通过 SetEmailServerProvider 更换数据源
type EmailServerProvider interface {
	GetEmailServer(ctx context.Context, name string) (*EmailServer, error)
}

// EmailServerProviderFunc 函数形式的 EmailServerProvider
type EmailServerProviderFunc func(ctx context.Context, name string) (*EmailServer, error)

func (f EmailServerProviderFunc) GetEmailServer(ctx context.Context, name string) (*EmailServer, error) {
	return f(ctx, name)
}

var (
	emailServerMu       sync.RWMutex
	emailServerProvider EmailServerProvider = ConfigEmailServerProvider{}
)

// SetEmailServerProvider 更换 SMTP 服务器配置的数据源(默认读取 glue 配置 emails 节点)；传 nil 恢复默认
func SetEmailServerProvider(p EmailServerProvider) {
	if p == nil {
		p = ConfigEmailServerProvider{}
	}
	emailServerMu.Lock()
	emailServerProvider = p
	emailServerMu.Unlock()
}

// GetEmailServerProvider 当前使用的 SMTP 服务器配置数据源
func GetEmailServerProvider() EmailServerProvider {
	emailServerMu.RLock()
	defer emailServerMu.RUnlock()
	return emailServerProvider
}

// ConfigEmailServerProvider 从 glue 配置读取 SMTP 服务器列表，每次读取最新配置
type ConfigEmailServerProvider struct {
	Key string // 配置节点名称，默认 emails
}

// ListEmailServers 返回配置中的全部 SMTP 服务器；未配置时返回空列表
func (p ConfigEmailServerProvider) ListEmailServers() ([]EmailServer, error) {
	key := p.Key
	if key == "" {
		key = EmailServerConfigKey
	}
	if global.Config == nil {
		return nil, fmt.Errorf("config is not loaded")
	}
	val := global.Config.Value(key)
	if !val.Exists() {
		return []EmailServer{}, nil
	}
	var servers []EmailServer
	if err := val.ScanTo(&servers); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", key, err)
	}
	for i := range servers {
		servers[i].SmtpName = strings.TrimSpace(servers[i].SmtpName)
	}
	return servers, nil
}

func (p ConfigEmailServerProvider) GetEmailServer(_ context.Context, name string) (*EmailServer, error) {
	servers, err := p.ListEmailServers()
	if err != nil {
		return nil, err
	}
	for i := range servers {
		if servers[i].SmtpName == name {
			return &servers[i], nil
		}
	}
	return nil, fmt.Errorf("smtp server %q not found", name)
}
