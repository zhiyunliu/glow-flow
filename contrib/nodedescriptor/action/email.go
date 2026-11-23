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
  - host：SMTP 服务器地址，必填
  - port：SMTP 端口，默认 ssl=true 时 465，否则 25
  - ssl：是否使用 SSL/TLS 直连(通常为 465 端口)；为 false 时若服务器支持则自动 STARTTLS
  - username / password：认证账号及密码(授权码)，username 为空时不认证；
    认证只允许在加密连接(或 localhost)上进行
  - from：发件人，默认为 username，支持 "名称 <addr@example.com>" 格式
  - to / cc / bcc：收件人列表(数组或逗号分隔字符串)，支持 ${} 模板，to/cc/bcc 至少一个不为空
  - subject：主题，支持 ${} 模板
  - content：正文，支持 ${} 模板；为空时使用消息数据(非字符串数据 JSON 序列化)
  - contenttype：html 或 text，默认 html
  - timeout：超时时间，如 30s 或秒数，默认 30s

发送成功走 success 分支并原样传递消息；发送失败走 failure 分支。
*/
type nodeEmail struct {
	glowflow.Node
	host        string
	port        int
	ssl         bool
	username    string
	password    string
	from        *mail.Address
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

func (n *nodeEmail) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	m, err := n.render(ctx, msg)
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}

	sendCtx, cancel := withTimeout(ctx, n.timeout)
	defer cancel()
	if err := n.send(sendCtx, m); err != nil {
		return failureResult(ctx, n, msg, fmt.Errorf("send email via %s:%d: %w", n.host, n.port, err))
	}
	return passResult(msg), nil
}

func (n *nodeEmail) render(ctx glowflow.Context, msg *glowflow.Message) (*mailMessage, error) {
	env := templateEnv(ctx, msg)
	m := &mailMessage{from: n.from, contentType: n.contentType}

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

func (n *nodeEmail) send(ctx context.Context, m *mailMessage) (err error) {
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
	node := &nodeEmail{
		Node:     def,
		host:     strings.TrimSpace(ext.GetString("host")),
		ssl:      ext.GetBool("ssl"),
		username: strings.TrimSpace(ext.GetString("username")),
		password: ext.GetString("password"),
	}
	if node.host == "" {
		return nil, invalidConfig(def, "host is empty")
	}

	port, err := getInt64(ext, "port")
	if err != nil || port < 0 || port > 65535 {
		return nil, invalidConfig(def, "invalid port %q", ext.GetString("port"))
	}
	node.port = int(port)
	if node.port == 0 {
		node.port = 25
		if node.ssl {
			node.port = 465
		}
	}

	from := strings.TrimSpace(ext.GetString("from"))
	if from == "" {
		from = node.username
	}
	if node.from, err = mail.ParseAddress(from); err != nil {
		return nil, invalidConfig(def, "invalid from %q: %v", from, err)
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
