package action

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"reflect"
	"strings"
	"testing"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/golibs/xtypes"
)

type smtpCapture struct {
	from  string
	rcpts []string
	data  string
}

// startFakeSMTP 启动一个只处理单次会话、不支持 STARTTLS/AUTH 的 SMTP 服务
func startFakeSMTP(t *testing.T) (int, <-chan smtpCapture) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ch := make(chan smtpCapture, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		r := bufio.NewReader(conn)
		reply := func(line string) { fmt.Fprintf(conn, "%s\r\n", line) }
		address := func(line string) string {
			return line[strings.Index(line, "<")+1 : strings.Index(line, ">")]
		}

		var capture smtpCapture
		reply("220 localhost ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				reply("250 localhost")
			case strings.HasPrefix(upper, "MAIL FROM:"):
				capture.from = address(line)
				reply("250 OK")
			case strings.HasPrefix(upper, "RCPT TO:"):
				capture.rcpts = append(capture.rcpts, address(line))
				reply("250 OK")
			case upper == "DATA":
				reply("354 go ahead")
				var data strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				capture.data = data.String()
				reply("250 OK")
			case upper == "QUIT":
				reply("221 bye")
				ch <- capture
				return
			default:
				reply("502 unsupported")
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, ch
}

func parseMail(t *testing.T, raw string) (*mail.Message, string) {
	t.Helper()
	m, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse mail: %v\n%s", err, raw)
	}
	encoded, _ := bufio.NewReader(m.Body).ReadString(0)
	body, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(encoded, "\r\n", ""))
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return m, string(body)
}

func TestEmailSend(t *testing.T) {
	port, captured := startFakeSMTP(t)
	node := buildNode(t, EmailNodeType, xtypes.XMap{
		"host":    "127.0.0.1",
		"port":    float64(port),
		"from":    "Glow Flow <noreply@example.com>",
		"to":      []any{"${email}", "ops@example.com"},
		"cc":      "lead@example.com",
		"bcc":     "audit@example.com",
		"subject": "订单${order_no}已支付",
		"content": "<p>订单 ${order_no}</p>",
		"timeout": "5s",
	})

	msg := glowflow.NewMessage("msg-1", map[string]any{"order_no": "A1", "email": "buyer@example.com"})
	if result := execute(t, node, msg); result.RelationType != "success" || result.Msg != msg {
		t.Fatalf("result = %+v", result)
	}

	var capture smtpCapture
	select {
	case capture = <-captured:
	case <-time.After(5 * time.Second):
		t.Fatal("smtp session not completed")
	}
	if capture.from != "noreply@example.com" {
		t.Fatalf("mail from = %q", capture.from)
	}
	wantRcpts := []string{"buyer@example.com", "ops@example.com", "lead@example.com", "audit@example.com"}
	if !reflect.DeepEqual(capture.rcpts, wantRcpts) {
		t.Fatalf("rcpts = %v, want %v", capture.rcpts, wantRcpts)
	}

	m, body := parseMail(t, capture.data)
	subject, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if err != nil || subject != "订单A1已支付" {
		t.Fatalf("subject = %q, %v", subject, err)
	}
	if got := m.Header.Get("To"); got != "<buyer@example.com>, <ops@example.com>" {
		t.Fatalf("To = %q", got)
	}
	if m.Header.Get("Cc") != "<lead@example.com>" || m.Header.Get("Bcc") != "" || strings.Contains(capture.data, "audit@") {
		t.Fatalf("cc/bcc headers wrong:\n%s", capture.data)
	}
	if m.Header.Get("Content-Type") != "text/html; charset=UTF-8" || body != "<p>订单 A1</p>" {
		t.Fatalf("content-type/body = %q / %q", m.Header.Get("Content-Type"), body)
	}
}

func TestEmailMessageHeaderInjection(t *testing.T) {
	from, _ := mail.ParseAddress("noreply@example.com")
	to, _ := mail.ParseAddress("buyer@example.com")
	m := &mailMessage{
		from: from, to: []*mail.Address{to}, contentType: "text/plain",
		subject: "hi\r\nBcc: evil@example.com", body: strings.Repeat("x", 200),
	}
	raw := string(m.bytes())
	if strings.Contains(raw, "\r\nBcc:") {
		t.Fatalf("subject injected a header:\n%s", raw)
	}
	parsed, body := parseMail(t, raw)
	if parsed.Header.Get("Subject") != "hi  Bcc: evil@example.com" || body != strings.Repeat("x", 200) {
		t.Fatalf("subject/body = %q / %q", parsed.Header.Get("Subject"), body)
	}
	for _, line := range strings.Split(raw, "\r\n") {
		if len(line) > 78 {
			t.Fatalf("line too long (%d): %q", len(line), line)
		}
	}
}

func TestEmailFailures(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	base := xtypes.XMap{"host": "127.0.0.1", "port": float64(closedPort), "from": "a@example.com", "to": "${email}", "timeout": "2s"}
	node := buildNode(t, EmailNodeType, base)

	refused := glowflow.NewMessage("msg-1", map[string]any{"email": "b@example.com"})
	assertFailure(t, execute(t, node, refused), refused)

	invalid := glowflow.NewMessage("msg-2", map[string]any{"email": "not-an-email"})
	assertFailure(t, execute(t, node, invalid), invalid)

	empty := glowflow.NewMessage("msg-3", map[string]any{})
	assertFailure(t, execute(t, node, empty), empty)

	for _, change := range []xtypes.XMap{
		{"host": ""},
		{"to": nil},
		{"from": "bad"},
		{"port": "abc"},
		{"contenttype": "pdf"},
		{"timeout": "x"},
	} {
		ext := xtypes.XMap{}
		for k, v := range base {
			ext[k] = v
		}
		for k, v := range change {
			ext[k] = v
		}
		assertInvalidConfig(t, EmailNodeType, ext)
	}
}
