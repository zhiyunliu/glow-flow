package action

import (
	"context"
	"fmt"
	"testing"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glue/config"
	"github.com/zhiyunliu/glue/global"
	"github.com/zhiyunliu/golibs/xtypes"
)

func useEmailServers(t *testing.T, servers map[string]*EmailServer) *[]string {
	t.Helper()
	var calls []string
	SetEmailServerProvider(EmailServerProviderFunc(func(_ context.Context, name string) (*EmailServer, error) {
		calls = append(calls, name)
		if s, ok := servers[name]; ok {
			return s, nil
		}
		return nil, fmt.Errorf("smtp server %q not found", name)
	}))
	t.Cleanup(func() { SetEmailServerProvider(nil) })
	return &calls
}

func TestEmailSendBySmtpName(t *testing.T) {
	port, captured := startFakeSMTP(t)
	calls := useEmailServers(t, map[string]*EmailServer{
		"test1": {SmtpName: "test1", Host: "127.0.0.1", Port: port, From: "运维 <ops@example.com>"},
	})
	// 引用配置名称时无需 host，内联的 host 被忽略
	node := buildNode(t, EmailNodeType, xtypes.XMap{
		"smtpname": "test1",
		"host":     "ignored.example.com",
		"to":       "buyer@example.com",
		"subject":  "hi",
		"timeout":  "5s",
	})
	msg := glowflow.NewMessage("msg-1", map[string]any{"a": 1})
	if result := execute(t, node, msg); result.RelationType != "success" {
		t.Fatalf("result = %+v", result)
	}
	var capture smtpCapture
	select {
	case capture = <-captured:
	case <-time.After(5 * time.Second):
		t.Fatal("smtp session not completed")
	}
	if capture.from != "ops@example.com" {
		t.Fatalf("mail from = %q, want server default from", capture.from)
	}
	if len(*calls) != 1 || (*calls)[0] != "test1" {
		t.Fatalf("provider calls = %v", *calls)
	}
}

func TestEmailSmtpNameFailures(t *testing.T) {
	useEmailServers(t, map[string]*EmailServer{
		"nohost":  {SmtpName: "nohost"},
		"nofrom":  {SmtpName: "nofrom", Host: "127.0.0.1", Port: 1, Username: "plain-user"},
		"badfrom": {SmtpName: "badfrom", Host: "127.0.0.1", Port: 1, From: "bad"},
	})
	for _, name := range []string{"missing", "nohost", "nofrom", "badfrom"} {
		node := buildNode(t, EmailNodeType, xtypes.XMap{"smtpname": name, "to": "b@example.com", "timeout": "2s"})
		msg := glowflow.NewMessage("msg-"+name, map[string]any{})
		assertFailure(t, execute(t, node, msg), msg)
	}

	// 节点配置的发件人优先于服务器配置；非法发件人在编译时报错
	assertInvalidConfig(t, EmailNodeType, xtypes.XMap{"smtpname": "nofrom", "to": "b@example.com", "from": "bad"})
	// 内联模式下账号不是邮箱地址时必须配置发件人
	assertInvalidConfig(t, EmailNodeType, xtypes.XMap{"host": "127.0.0.1", "username": "plain-user", "to": "b@example.com"})
	buildNode(t, EmailNodeType, xtypes.XMap{"host": "127.0.0.1", "username": "plain-user", "from": "a@example.com", "to": "b@example.com"})
}

func TestConfigEmailServerProvider(t *testing.T) {
	old := global.Config
	t.Cleanup(func() { global.Config = old })

	cfg := config.New(config.WithSource(config.NewStrSource(`{"emails":[
		{"smtpname":" test1 ","host":"smtp1.example.com","port":465,"ssl":true,"username":"a@example.com","password":"p1"},
		{"smtpname":"test2","host":"smtp2.example.com"}
	]}`)))
	if err := cfg.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	global.Config = cfg

	p := ConfigEmailServerProvider{}
	servers, err := p.ListEmailServers()
	if err != nil || len(servers) != 2 || servers[0].SmtpName != "test1" {
		t.Fatalf("servers = %+v, %v", servers, err)
	}
	s, err := p.GetEmailServer(context.Background(), "test1")
	if err != nil || s.Host != "smtp1.example.com" || s.Port != 465 || !s.SSL || s.Password != "p1" {
		t.Fatalf("test1 = %+v, %v", s, err)
	}
	if _, err := p.GetEmailServer(context.Background(), "test3"); err == nil {
		t.Fatal("test3 should not be found")
	}

	// 未配置 emails 节点时返回空列表
	empty := config.New(config.WithSource(config.NewStrSource(`{}`)))
	if err := empty.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	global.Config = empty
	if servers, err := p.ListEmailServers(); err != nil || len(servers) != 0 {
		t.Fatalf("servers = %+v, %v", servers, err)
	}
}
