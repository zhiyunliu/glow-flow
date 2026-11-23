package action

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/golibs/xtypes"
)

type dingtalkRequest struct {
	path  string
	query url.Values
	body  map[string]any
}

// fakeDingtalk 模拟钉钉机器人 webhook、gettoken 及工作通知接口
type fakeDingtalk struct {
	mu        sync.Mutex
	server    *httptest.Server
	requests  []dingtalkRequest
	tokenHits int
	token     string
	sendCode  int
}

func newFakeDingtalk(t *testing.T) *fakeDingtalk {
	f := &fakeDingtalk{token: "TOKEN-1"}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		req := dingtalkRequest{path: r.URL.Path, query: r.URL.Query()}
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&req.body)
		}
		f.requests = append(f.requests, req)

		switch r.URL.Path {
		case "/gettoken":
			f.tokenHits++
			if req.query.Get("appkey") != "key" || req.query.Get("appsecret") != "secret" {
				fmt.Fprint(w, `{"errcode":40089,"errmsg":"invalid appkey"}`)
				return
			}
			fmt.Fprintf(w, `{"errcode":0,"errmsg":"ok","access_token":%q,"expires_in":7200}`, f.token)
		default:
			fmt.Fprintf(w, `{"errcode":%d,"errmsg":"msg"}`, f.sendCode)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeDingtalk) sends(path string) []dingtalkRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []dingtalkRequest
	for _, req := range f.requests {
		if req.path == path {
			result = append(result, req)
		}
	}
	return result
}

func (f *fakeDingtalk) setResponse(sendCode int, token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendCode, f.token = sendCode, token
}

func (f *fakeDingtalk) tokenCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokenHits
}

func (f *fakeDingtalk) useOpenAPI(t *testing.T) {
	old := dingtalkOpenAPI
	dingtalkOpenAPI = f.server.URL
	t.Cleanup(func() { dingtalkOpenAPI = old })
}

func TestDingtalkGroupMarkdownWithSign(t *testing.T) {
	fake := newFakeDingtalk(t)
	node := buildNode(t, DingtalkNodeType, xtypes.XMap{
		"webhook":   fake.server.URL + "/robot/send?access_token=robot-token",
		"secret":    "SEC123",
		"msgtype":   "markdown",
		"title":     "订单${order_no}",
		"content":   "### 订单 ${order_no} 已支付",
		"atmobiles": "${mobile}",
		"atuserids": []any{"u1"},
	})

	msg := glowflow.NewMessage("msg-1", map[string]any{"order_no": "A1", "mobile": "13800000000"})
	if result := execute(t, node, msg); result.RelationType != "success" || result.Msg != msg {
		t.Fatalf("result = %+v", result)
	}

	sends := fake.sends("/robot/send")
	if len(sends) != 1 {
		t.Fatalf("robot sends = %d", len(sends))
	}
	req := sends[0]
	if req.query.Get("access_token") != "robot-token" || req.query.Get("sign") != dingtalkSign(req.query.Get("timestamp"), "SEC123") {
		t.Fatalf("query = %v", req.query)
	}
	want := map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]any{"title": "订单A1", "text": "### 订单 A1 已支付"},
		"at":       map[string]any{"atMobiles": []any{"13800000000"}, "atUserIds": []any{"u1"}},
	}
	if !reflect.DeepEqual(req.body, want) {
		t.Fatalf("body = %#v, want %#v", req.body, want)
	}
}

func TestDingtalkGroupTextAndImage(t *testing.T) {
	fake := newFakeDingtalk(t)
	webhook := fake.server.URL + "/robot/send?access_token=tk"

	text := buildNode(t, DingtalkNodeType, xtypes.XMap{"webhook": webhook, "atall": true})
	msg := glowflow.NewMessage("msg-1", "服务告警")
	execute(t, text, msg)

	image := buildNode(t, DingtalkNodeType, xtypes.XMap{"webhook": webhook, "msgtype": "image", "imageurl": "${data}"})
	execute(t, image, glowflow.NewMessage("msg-2", "https://img.example.com/1.png"))

	sends := fake.sends("/robot/send")
	if len(sends) != 2 {
		t.Fatalf("robot sends = %d", len(sends))
	}
	if _, signed := sends[0].query["sign"]; signed {
		t.Fatalf("unsigned robot should not send sign")
	}
	wantText := map[string]any{"msgtype": "text", "text": map[string]any{"content": "服务告警"}, "at": map[string]any{"isAtAll": true}}
	if !reflect.DeepEqual(sends[0].body, wantText) {
		t.Fatalf("text body = %#v", sends[0].body)
	}
	wantImage := map[string]any{"msgtype": "markdown", "markdown": map[string]any{"title": "测试节点", "text": "![测试节点](https://img.example.com/1.png)"}}
	if !reflect.DeepEqual(sends[1].body, wantImage) {
		t.Fatalf("image body = %#v", sends[1].body)
	}

	fake.setResponse(310000, "TOKEN-1")
	assertFailure(t, execute(t, text, msg), msg)
}

func TestDingtalkWorkNoticeToUsers(t *testing.T) {
	fake := newFakeDingtalk(t)
	fake.useOpenAPI(t)

	userIDs := []any{"${user}"}
	for i := 0; i < 120; i++ {
		userIDs = append(userIDs, fmt.Sprintf("u%03d", i))
	}
	node := buildNode(t, DingtalkNodeType, xtypes.XMap{
		"target":    "user",
		"appkey":    "key",
		"appsecret": "secret",
		"agentid":   float64(123), // JSON 解析得到的数字
		"userids":   userIDs,
		"content":   "审批 ${order_no} 待处理",
	})

	msg := glowflow.NewMessage("msg-1", map[string]any{"order_no": "A1", "user": "boss"})
	execute(t, node, msg)
	execute(t, node, msg)

	const sendPath = "/topapi/message/corpconversation/asyncsend_v2"
	sends := fake.sends(sendPath)
	if fake.tokenCount() != 1 || len(sends) != 4 {
		t.Fatalf("token hits = %d, sends = %d; want 1 and 4 (121 users in 2 batches, twice)", fake.tokenCount(), len(sends))
	}
	first := sends[0]
	if first.query.Get("access_token") != "TOKEN-1" || first.body["agent_id"] != float64(123) {
		t.Fatalf("first send = %+v", first)
	}
	users := strings.Split(first.body["userid_list"].(string), ",")
	if len(users) != dingtalkMaxUsers || users[0] != "boss" {
		t.Fatalf("first batch users = %d, first = %q", len(users), users[0])
	}
	if got := len(strings.Split(sends[1].body["userid_list"].(string), ",")); got != 21 {
		t.Fatalf("second batch users = %d, want 21", got)
	}
	wantMsg := map[string]any{"msgtype": "text", "text": map[string]any{"content": "审批 A1 待处理"}}
	if !reflect.DeepEqual(first.body["msg"], wantMsg) {
		t.Fatalf("msg = %#v", first.body["msg"])
	}

	// 发送返回业务错误后清除缓存的 access_token
	fake.setResponse(40014, "TOKEN-1")
	assertFailure(t, execute(t, node, msg), msg)
	fake.setResponse(0, "TOKEN-2")
	execute(t, node, msg)
	if fake.tokenCount() != 2 {
		t.Fatalf("token hits = %d, want 2 after token reset", fake.tokenCount())
	}
	if last := fake.sends(sendPath); last[len(last)-1].query.Get("access_token") != "TOKEN-2" {
		t.Fatalf("last send should use refreshed token")
	}
}

func TestDingtalkErrorsDoNotLeakSecrets(t *testing.T) {
	old := dingtalkOpenAPI
	dingtalkOpenAPI = "http://127.0.0.1:1"
	t.Cleanup(func() { dingtalkOpenAPI = old })

	node := buildNode(t, DingtalkNodeType, xtypes.XMap{
		"target": "user", "appkey": "key", "appsecret": "TOP-SECRET", "agentid": 1, "userids": "u1", "timeout": "2s",
	}).(*nodeDingtalk)

	_, err := node.accessToken(newTestContext())
	if err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(err.Error(), "TOP-SECRET") {
		t.Fatalf("error leaks appsecret: %v", err)
	}
}

func TestDingtalkInvalidConfig(t *testing.T) {
	for _, ext := range []xtypes.XMap{
		{},
		{"webhook": "not-a-url"},
		{"webhook": "http://x/robot", "msgtype": "video"},
		{"webhook": "http://x/robot", "msgtype": "image"},
		{"webhook": "http://x/robot", "target": "channel"},
		{"target": "user", "appkey": "k", "appsecret": "s", "userids": "u1"},
		{"target": "user", "appkey": "k", "appsecret": "s", "agentid": 1},
		{"target": "user", "agentid": 1, "userids": "u1"},
	} {
		assertInvalidConfig(t, DingtalkNodeType, ext)
	}
}
