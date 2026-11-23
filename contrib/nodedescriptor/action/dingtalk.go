package action

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/utility"
)

const DingtalkNodeType = "dingtalk"

const (
	dingtalkTargetGroup = "group"
	dingtalkTargetUser  = "user"

	dingtalkMsgText     = "text"
	dingtalkMsgMarkdown = "markdown"
	dingtalkMsgImage    = "image"

	defaultDingtalkTimeout = 10 * time.Second
	// dingtalkMaxUsers 工作通知单次最多发送的用户数
	dingtalkMaxUsers = 100
	// dingtalkTokenEarlyExpire access_token 提前过期时间，避免临界点失效
	dingtalkTokenEarlyExpire = 5 * time.Minute
)

var (
	// dingtalkOpenAPI 钉钉开放平台地址，测试时可替换
	dingtalkOpenAPI = "https://oapi.dingtalk.com"
	// dingtalkHTTPClient 钉钉请求使用的 http 客户端，测试时可替换
	dingtalkHTTPClient = &http.Client{}
)

// 执行钉钉消息
/*
钉钉的配置通过glowflow.NodeDefinition中读取(节点类型 dingtalk)
完成钉钉的文本，图片，md内容的发送，指定到群和指定到人

extparams 配置：
  - target：group(群自定义机器人，默认) 或 user(企业内部应用工作通知，指定到人)
  - msgtype：text / markdown / image，默认 text
  - title：标题(markdown/image 消息使用)，支持 ${} 模板，默认为节点名称
  - content：消息内容，支持 ${} 模板；text/markdown 消息为空时使用消息数据(非字符串数据 JSON 序列化)，
    image 消息中作为图片下方的说明文字(可选)
  - imageurl：图片地址(image 消息必填)，支持 ${} 模板；以 Markdown 图片方式发送
  - timeout：超时时间，如 10s 或秒数，默认 10s

指定到群(target=group)：
  - webhook：机器人 Webhook 地址(含 access_token)，必填
  - secret：机器人加签密钥(安全设置为"加签"时必填)
  - atmobiles / atuserids：需要 @ 的群成员手机号 / userId(数组或逗号分隔字符串)，支持 ${} 模板；
    text 消息需在 content 中包含 "@手机号" 才会高亮显示
  - atall：是否 @所有人

指定到人(target=user)：
  - appkey / appsecret：企业内部应用凭证，必填(access_token 自动获取并缓存)
  - agentid：应用 AgentId，必填
  - userids：接收人 userId 列表(数组或逗号分隔字符串)，支持 ${} 模板，必填；超过 100 人时分批发送

钉钉返回 errcode=0 时走 success 分支并原样传递消息；否则走 failure 分支。
*/
type nodeDingtalk struct {
	glowflow.Node
	target   string
	msgType  string
	title    utility.Template
	content  utility.Template
	imageURL utility.Template
	timeout  time.Duration

	// target=group
	webhook   *url.URL
	secret    string
	atMobiles []utility.Template
	atUserIDs []utility.Template
	atAll     bool

	// target=user
	appKey    string
	appSecret string
	agentID   int64
	userIDs   []utility.Template

	tokenMu     sync.Mutex
	token       string
	tokenExpire time.Time
}

func (n *nodeDingtalk) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	env := templateEnv(ctx, msg)
	body, err := n.buildMessage(env, msg)
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}

	reqCtx, cancel := withTimeout(ctx, n.timeout)
	defer cancel()
	if n.target == dingtalkTargetUser {
		err = n.sendToUsers(reqCtx, env, body)
	} else {
		err = n.sendToGroup(reqCtx, env, body)
	}
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}
	return passResult(msg), nil
}

// buildMessage 生成钉钉消息体(msgtype + 内容)，群机器人与工作通知格式相同
func (n *nodeDingtalk) buildMessage(env map[string]any, msg *glowflow.Message) (map[string]any, error) {
	title, err := renderTemplate(n.title, env)
	if err != nil {
		return nil, fmt.Errorf("title: %w", err)
	}
	if title = strings.TrimSpace(title); title == "" {
		title = n.Name()
	}
	content, err := renderTemplate(n.content, env)
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	if n.content == nil && n.msgType != dingtalkMsgImage {
		if content, err = payloadString(messageData(msg)); err != nil {
			return nil, fmt.Errorf("content: %w", err)
		}
	}

	switch n.msgType {
	case dingtalkMsgImage:
		imageURL, err := renderTemplate(n.imageURL, env)
		if err != nil {
			return nil, fmt.Errorf("imageurl: %w", err)
		}
		if imageURL = strings.TrimSpace(imageURL); imageURL == "" {
			return nil, fmt.Errorf("imageurl is empty")
		}
		text := fmt.Sprintf("![%s](%s)", title, imageURL)
		if strings.TrimSpace(content) != "" {
			text += "\n\n" + content
		}
		return markdownMessage(title, text), nil
	case dingtalkMsgMarkdown:
		if strings.TrimSpace(content) == "" {
			return nil, fmt.Errorf("content is empty")
		}
		return markdownMessage(title, content), nil
	default:
		if strings.TrimSpace(content) == "" {
			return nil, fmt.Errorf("content is empty")
		}
		return map[string]any{
			"msgtype": dingtalkMsgText,
			"text":    map[string]any{"content": content},
		}, nil
	}
}

func markdownMessage(title, text string) map[string]any {
	if title == "" {
		title = text
	}
	return map[string]any{
		"msgtype":  dingtalkMsgMarkdown,
		"markdown": map[string]any{"title": title, "text": text},
	}
}

func (n *nodeDingtalk) sendToGroup(ctx context.Context, env map[string]any, body map[string]any) error {
	mobiles, err := renderList(n.atMobiles, env)
	if err != nil {
		return fmt.Errorf("atmobiles: %w", err)
	}
	userIDs, err := renderList(n.atUserIDs, env)
	if err != nil {
		return fmt.Errorf("atuserids: %w", err)
	}
	at := map[string]any{}
	if len(mobiles) > 0 {
		at["atMobiles"] = mobiles
	}
	if len(userIDs) > 0 {
		at["atUserIds"] = userIDs
	}
	if n.atAll {
		at["isAtAll"] = true
	}
	if len(at) > 0 {
		body["at"] = at
	}
	return dingtalkDo(ctx, http.MethodPost, n.signedWebhook(time.Now()), body, nil)
}

// signedWebhook 配置了加签密钥时追加 timestamp/sign 参数
func (n *nodeDingtalk) signedWebhook(now time.Time) string {
	if n.secret == "" {
		return n.webhook.String()
	}
	timestamp := strconv.FormatInt(now.UnixMilli(), 10)
	u := *n.webhook
	query := u.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", dingtalkSign(timestamp, n.secret))
	u.RawQuery = query.Encode()
	return u.String()
}

func dingtalkSign(timestamp, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "\n" + secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (n *nodeDingtalk) sendToUsers(ctx context.Context, env map[string]any, body map[string]any) error {
	userIDs, err := renderList(n.userIDs, env)
	if err != nil {
		return fmt.Errorf("userids: %w", err)
	}
	if len(userIDs) == 0 {
		return fmt.Errorf("userids is empty")
	}

	for start := 0; start < len(userIDs); start += dingtalkMaxUsers {
		end := min(start+dingtalkMaxUsers, len(userIDs))
		token, err := n.accessToken(ctx)
		if err != nil {
			return err
		}
		payload := map[string]any{
			"agent_id":    n.agentID,
			"userid_list": strings.Join(userIDs[start:end], ","),
			"msg":         body,
		}
		endpoint := dingtalkOpenAPI + "/topapi/message/corpconversation/asyncsend_v2?access_token=" + url.QueryEscape(token)
		if err := dingtalkDo(ctx, http.MethodPost, endpoint, payload, nil); err != nil {
			var apiErr *dingtalkError
			if errors.As(err, &apiErr) {
				// access_token 可能已失效，下次调用重新获取
				n.resetToken()
			}
			return err
		}
	}
	return nil
}

// accessToken 获取并缓存企业内部应用的 access_token
func (n *nodeDingtalk) accessToken(ctx context.Context) (string, error) {
	n.tokenMu.Lock()
	defer n.tokenMu.Unlock()
	if n.token != "" && time.Now().Before(n.tokenExpire) {
		return n.token, nil
	}

	query := url.Values{"appkey": {n.appKey}, "appsecret": {n.appSecret}}
	var resp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := dingtalkDo(ctx, http.MethodGet, dingtalkOpenAPI+"/gettoken?"+query.Encode(), nil, &resp); err != nil {
		return "", fmt.Errorf("get access_token: %w", err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("get access_token: empty token")
	}
	ttl := time.Duration(resp.ExpiresIn)*time.Second - dingtalkTokenEarlyExpire
	if ttl <= 0 {
		ttl = time.Minute
	}
	n.token = resp.AccessToken
	n.tokenExpire = time.Now().Add(ttl)
	return n.token, nil
}

func (n *nodeDingtalk) resetToken() {
	n.tokenMu.Lock()
	defer n.tokenMu.Unlock()
	n.token = ""
}

// dingtalkError 钉钉接口返回的业务错误
type dingtalkError struct {
	Code int
	Msg  string
}

func (e *dingtalkError) Error() string {
	return fmt.Sprintf("dingtalk errcode=%d errmsg=%s", e.Code, e.Msg)
}

// dingtalkDo 调用钉钉接口；errcode!=0 时返回 *dingtalkError，out 非空时解析响应
func dingtalkDo(ctx context.Context, method, endpoint string, payload any, out any) error {
	var reqBody io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return redactURLError(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json;charset=utf-8")
	}

	resp, err := dingtalkHTTPClient.Do(req)
	if err != nil {
		return redactURLError(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("dingtalk unexpected status %d: %s", resp.StatusCode, truncate(raw, 512))
	}

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("dingtalk invalid response: %s", truncate(raw, 512))
	}
	if result.ErrCode != 0 {
		return &dingtalkError{Code: result.ErrCode, Msg: result.ErrMsg}
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// redactURLError 去掉错误信息中的请求地址，避免 access_token/appsecret 等敏感参数写入日志
func redactURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("dingtalk request %s: %w", urlErr.Op, urlErr.Err)
	}
	return err
}

type dingtalkFactory struct{}

func (f *dingtalkFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	ext := def.ExtParams
	node := &nodeDingtalk{
		Node:    def,
		target:  strings.ToLower(strings.TrimSpace(ext.GetString("target"))),
		msgType: strings.ToLower(strings.TrimSpace(ext.GetString("msgtype"))),
	}
	if node.target == "" {
		node.target = dingtalkTargetGroup
	}
	if node.msgType == "" {
		node.msgType = dingtalkMsgText
	}
	switch node.msgType {
	case dingtalkMsgText, dingtalkMsgMarkdown, dingtalkMsgImage:
	default:
		return nil, invalidConfig(def, "unsupported msgtype %q", node.msgType)
	}

	var err error
	if node.title, err = compileTemplate(def, "title", ext.GetString("title")); err != nil {
		return nil, err
	}
	if content := ext.GetString("content"); strings.TrimSpace(content) != "" {
		if node.content, err = compileTemplate(def, "content", content); err != nil {
			return nil, err
		}
	}
	if node.msgType == dingtalkMsgImage {
		imageURL := strings.TrimSpace(ext.GetString("imageurl"))
		if imageURL == "" {
			return nil, invalidConfig(def, "imageurl is empty")
		}
		if node.imageURL, err = compileTemplate(def, "imageurl", imageURL); err != nil {
			return nil, err
		}
	}
	if node.timeout, err = getDuration(ext, "timeout", defaultDingtalkTimeout); err != nil {
		return nil, invalidConfig(def, "%v", err)
	}

	switch node.target {
	case dingtalkTargetGroup:
		err = node.buildGroup(def)
	case dingtalkTargetUser:
		err = node.buildUser(def)
	default:
		err = invalidConfig(def, "unsupported target %q", node.target)
	}
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (n *nodeDingtalk) buildGroup(def glowflow.NodeDefinition) error {
	ext := def.ExtParams
	webhook := strings.TrimSpace(ext.GetString("webhook"))
	if webhook == "" {
		return invalidConfig(def, "webhook is empty")
	}
	u, err := url.Parse(webhook)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return invalidConfig(def, "invalid webhook")
	}
	n.webhook = u
	n.secret = strings.TrimSpace(ext.GetString("secret"))
	n.atAll = ext.GetBool("atall")
	if n.atMobiles, err = compileTemplates(def, "atmobiles", getStrings(ext, "atmobiles")); err != nil {
		return err
	}
	if n.atUserIDs, err = compileTemplates(def, "atuserids", getStrings(ext, "atuserids")); err != nil {
		return err
	}
	return nil
}

func (n *nodeDingtalk) buildUser(def glowflow.NodeDefinition) error {
	ext := def.ExtParams
	n.appKey = strings.TrimSpace(ext.GetString("appkey"))
	n.appSecret = strings.TrimSpace(ext.GetString("appsecret"))
	if n.appKey == "" || n.appSecret == "" {
		return invalidConfig(def, "appkey/appsecret is empty")
	}
	agentID, err := getInt64(ext, "agentid")
	if err != nil || agentID <= 0 {
		return invalidConfig(def, "invalid agentid %q", ext.GetString("agentid"))
	}
	n.agentID = agentID

	userIDs := getStrings(ext, "userids")
	if len(userIDs) == 0 {
		return invalidConfig(def, "userids is empty")
	}
	if n.userIDs, err = compileTemplates(def, "userids", userIDs); err != nil {
		return err
	}
	return nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    DingtalkNodeType,
		Factory: &dingtalkFactory{},
	})
}
