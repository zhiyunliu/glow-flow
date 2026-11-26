package action

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/utility"
)

const EmailNodeType = "email"

const defaultEmailTimeout = 30 * time.Second

// 执行邮件消息
/*
通过 SMTP 发送邮件(节点类型 email)

extparams 配置：
  - smtpname：SMTP 服务器配置名称，执行时通过 EmailServerProvider 获取服务器参数
    (默认读取 glue 配置 emails 数组中 smtpname 相同的一项，见 EmailServer)；
    设置后忽略下方 host/port/ssl/username/password
  - host：SMTP 服务器地址，smtpname 为空时必填
  - port：SMTP 端口，默认 ssl=true 时 465，否则 25
  - ssl：是否使用 SSL/TLS 直连(通常为 465 端口)；为 false 时若服务器支持则自动 STARTTLS
  - username / password：认证账号及密码(授权码)，username 为空时不认证；
    认证只允许在加密连接(或 localhost)上进行
  - from：发件人，支持 "名称 <addr@example.com>" 格式；为空时使用服务器配置的 from，再为空使用 username
  - to / cc / bcc：收件人列表(数组或逗号分隔字符串)，支持 ${} 模板，to/cc/bcc 至少一个不为空
  - subject：主题，支持 ${} 模板
  - content：正文，支持 ${} 模板；为空时使用消息数据(非字符串数据 JSON 序列化)
  - contenttype：html 或 text，默认 html
  - timeout：超时时间，如 30s 或秒数，默认 30s

发送成功走 success 分支并原样传递消息；发送失败走 failure 分支。
*/
type nodeEmail struct {
	glowflow.Node
	smtpName    string        // 引用的 SMTP 服务器配置名称，非空时执行时获取服务器参数
	server      *smtpServer   // smtpname 为空时节点内联的服务器参数
	from        *mail.Address // 节点配置的发件人，为空时使用服务器的发件人
	to          []utility.Template
	cc          []utility.Template
	bcc         []utility.Template
	subject     utility.Template
	content     utility.Template
	contentType string
	timeout     time.Duration
}

// mailMessage 渲染后的邮件内容
type mailMessage struct {
	from        *mail.Address
	to, cc, bcc []*mail.Address
	subject     string
	body        string
	contentType string
}

// smtpServer 解析后的 SMTP 连接参数
type smtpServer struct {
	host     string
	port     int
	ssl      bool
	username string
	password string
	from     *mail.Address // 服务器默认发件人，可能为空
}

func newSMTPServer(host string, port int64, ssl bool, username, password, from string) (*smtpServer, error) {
	s := &smtpServer{host: strings.TrimSpace(host), ssl: ssl, username: strings.TrimSpace(username), password: password}
	if s.host == "" {
		return nil, fmt.Errorf("host is empty")
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}
	s.port = int(port)
	if s.port == 0 {
		s.port = 25
		if s.ssl {
			s.port = 465
		}
	}
	if from = strings.TrimSpace(from); from != "" {
		addr, err := mail.ParseAddress(from)
		if err != nil {
			return nil, fmt.Errorf("invalid from %q: %v", from, err)
		}
		s.from = addr
	} else if addr, err := mail.ParseAddress(s.username); err == nil {
		// 账号为邮箱地址时作为默认发件人
		s.from = addr
	}
	return s, nil
}

func (n *nodeEmail) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	m, err := n.render(ctx, msg)
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}

	sendCtx, cancel := withTimeout(ctx, n.timeout)
	defer cancel()
	server, err := n.resolveServer(sendCtx)
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}
	if m.from = n.from; m.from == nil {
		m.from = server.from
	}
	if m.from == nil {
		return failureResult(ctx, n, msg, fmt.Errorf("from is empty"))
	}
	if err := server.send(sendCtx, m); err != nil {
		return failureResult(ctx, n, msg, fmt.Errorf("send email via %s:%d: %w", server.host, server.port, err))
	}
	return passResult(msg), nil
}

// resolveServer 获取 SMTP 服务器参数：引用配置名称时每次执行从 EmailServerProvider 获取，以使用最新配置
func (n *nodeEmail) resolveServer(ctx context.Context) (*smtpServer, error) {
	if n.smtpName == "" {
		return n.server, nil
	}
	conf, err := GetEmailServerProvider().GetEmailServer(ctx, n.smtpName)
	if err != nil {
		return nil, fmt.Errorf("get smtp server %s: %w", n.smtpName, err)
	}
	if conf == nil {
		return nil, fmt.Errorf("smtp server %q not found", n.smtpName)
	}
	server, err := newSMTPServer(conf.Host, int64(conf.Port), conf.SSL, conf.Username, conf.Password, conf.From)
	if err != nil {
		return nil, fmt.Errorf("smtp server %s: %w", n.smtpName, err)
	}
	return server, nil
}

func (n *nodeEmail) render(ctx glowflow.Context, msg *glowflow.Message) (*mailMessage, error) {
	env := templateEnv(ctx, msg)
	m := &mailMessage{contentType: n.contentType}

	var err error
	if m.to, err = renderAddresses(n.to, env); err != nil {
		return nil, fmt.Errorf("to: %w", err)
	}
	if m.cc, err = renderAddresses(n.cc, env); err != nil {
		return nil, fmt.Errorf("cc: %w", err)
	}
	if m.bcc, err = renderAddresses(n.bcc, env); err != nil {
		return nil, fmt.Errorf("bcc: %w", err)
	}
	if len(m.to)+len(m.cc)+len(m.bcc) == 0 {
		return nil, fmt.Errorf("no recipients")
	}

	if m.subject, err = renderTemplate(n.subject, env); err != nil {
		return nil, fmt.Errorf("subject: %w", err)
	}
	if n.content != nil {
		m.body, err = renderTemplate(n.content, env)
	} else {
		m.body, err = payloadString(messageData(msg))
	}
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	return m, nil
}

func renderAddresses(tpls []utility.Template, env map[string]any) ([]*mail.Address, error) {
	items, err := renderList(tpls, env)
	if err != nil {
		return nil, err
	}
	addrs := make([]*mail.Address, 0, len(items))
	for _, item := range items {
		addr, err := mail.ParseAddress(item)
		if err != nil {
			return nil, fmt.Errorf("invalid address %q: %w", item, err)
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

func (n *smtpServer) send(ctx context.Context, m *mailMessage) (err error) {
	addr := net.JoinHostPort(n.host, strconv.Itoa(n.port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	// net/smtp 不支持 context：通过截止时间及取消时关闭连接来中断阻塞的读写
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	tlsConfig := &tls.Config{ServerName: n.host}
	if n.ssl {
		conn = tls.Client(conn, tlsConfig)
	}
	client, err := smtp.NewClient(conn, n.host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer client.Close()

	if !n.ssl {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return err
			}
		}
	}
	if n.username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp server does not support AUTH")
		}
		if err := client.Auth(smtp.PlainAuth("", n.username, n.password, n.host)); err != nil {
			return err
		}
	}

	if err := client.Mail(m.from.Address); err != nil {
		return err
	}
	for _, list := range [][]*mail.Address{m.to, m.cc, m.bcc} {
		for _, rcpt := range list {
			if err := client.Rcpt(rcpt.Address); err != nil {
				return fmt.Errorf("rcpt %s: %w", rcpt.Address, err)
			}
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(m.bytes()); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// bytes 生成 MIME 邮件内容；密送人不写入邮件头
func (m *mailMessage) bytes() []byte {
	var buf bytes.Buffer
	writeHeader := func(name, value string) {
		buf.WriteString(name)
		buf.WriteString(": ")
		buf.WriteString(value)
		buf.WriteString("\r\n")
	}
	writeHeader("From", m.from.String())
	if len(m.to) > 0 {
		writeHeader("To", joinAddresses(m.to))
	}
	if len(m.cc) > 0 {
		writeHeader("Cc", joinAddresses(m.cc))
	}
	writeHeader("Subject", mime.BEncoding.Encode("UTF-8", sanitizeHeader(m.subject)))
	writeHeader("Date", time.Now().Format(time.RFC1123Z))
	writeHeader("MIME-Version", "1.0")
	writeHeader("Content-Type", m.contentType+"; charset=UTF-8")
	writeHeader("Content-Transfer-Encoding", "base64")
	buf.WriteString("\r\n")

	encoded := base64.StdEncoding.EncodeToString([]byte(m.body))
	for len(encoded) > 76 {
		buf.WriteString(encoded[:76])
		buf.WriteString("\r\n")
		encoded = encoded[76:]
	}
	buf.WriteString(encoded)
	buf.WriteString("\r\n")
	return buf.Bytes()
}

func joinAddresses(addrs []*mail.Address) string {
	items := make([]string, len(addrs))
	for i, addr := range addrs {
		items[i] = addr.String()
	}
	return strings.Join(items, ", ")
}

// sanitizeHeader 去除换行，防止邮件头注入
func sanitizeHeader(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}

type emailFactory struct{}

func (f *emailFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	ext := def.ExtParams
	node := &nodeEmail{Node: def, smtpName: strings.TrimSpace(ext.GetString("smtpname"))}

	var err error
	if from := strings.TrimSpace(ext.GetString("from")); from != "" {
		if node.from, err = mail.ParseAddress(from); err != nil {
			return nil, invalidConfig(def, "invalid from %q: %v", from, err)
		}
	}
	if node.smtpName == "" {
		// 节点内联服务器参数
		port, err := getInt64(ext, "port")
		if err != nil || port < 0 || port > 65535 {
			return nil, invalidConfig(def, "invalid port %q", ext.GetString("port"))
		}
		node.server, err = newSMTPServer(ext.GetString("host"), port, ext.GetBool("ssl"),
			ext.GetString("username"), ext.GetString("password"), "")
		if err != nil {
			return nil, invalidConfig(def, "%v", err)
		}
		if node.from == nil && node.server.from == nil {
			return nil, invalidConfig(def, "from is empty")
		}
	}

	switch contentType := strings.ToLower(strings.TrimSpace(ext.GetString("contenttype"))); contentType {
	case "", "html", "text/html":
		node.contentType = "text/html"
	case "text", "plain", "text/plain":
		node.contentType = "text/plain"
	default:
		return nil, invalidConfig(def, "unsupported contenttype %q", contentType)
	}

	if node.to, err = compileTemplates(def, "to", getStrings(ext, "to")); err != nil {
		return nil, err
	}
	if node.cc, err = compileTemplates(def, "cc", getStrings(ext, "cc")); err != nil {
		return nil, err
	}
	if node.bcc, err = compileTemplates(def, "bcc", getStrings(ext, "bcc")); err != nil {
		return nil, err
	}
	if len(node.to)+len(node.cc)+len(node.bcc) == 0 {
		return nil, invalidConfig(def, "no recipients")
	}

	if node.subject, err = compileTemplate(def, "subject", ext.GetString("subject")); err != nil {
		return nil, err
	}
	if content := ext.GetString("content"); strings.TrimSpace(content) != "" {
		if node.content, err = compileTemplate(def, "content", content); err != nil {
			return nil, err
		}
	}
	if node.timeout, err = getDuration(ext, "timeout", defaultEmailTimeout); err != nil {
		return nil, invalidConfig(def, "%v", err)
	}
	return node, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    EmailNodeType,
		Factory: &emailFactory{},
	})
}
