package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/zhiyunliu/glue/context"
	"github.com/zhiyunliu/glue/errors"
)

// 钉钉机器人的 access_token 与加签密钥，需与流程中 dingtalk 节点的配置一致
const (
	robotAccessToken = "example-token"
	robotSecret      = "SECexample"
	maxRobotMessages = 50
)

// members 模拟会员数据；U4040 不存在，用于演示 httpcall 请求失败
var members = map[string]map[string]any{
	"U1001": {"member_level": "gold", "discount": 0.9},
	"U2002": {"member_level": "silver", "discount": 0.95},
	"U3003": {"member_level": "normal", "discount": 1},
}

// memberProfile 会员信息：GET /api/member/profile?user_id=U1001
func memberProfile(ctx context.Context) any {
	userID := ctx.Request().Query().Get("user_id")
	ctx.Log().Infof("member profile: user_id=%s order_no=%s", userID, ctx.Request().GetHeader("X-Order-No"))
	member, ok := members[userID]
	if !ok {
		return errors.New(http.StatusNotFound, "member not found: "+userID)
	}
	return map[string]any{
		"user_id":      userID,
		"member_level": member["member_level"],
		"discount":     member["discount"],
	}
}

type smsRequest struct {
	OrderNo string   `json:"order_no"`
	UserID  string   `json:"user_id"`
	Boxes   []string `json:"boxes"`
}

// smsSend 发货短信：POST /api/sms/send，请求体为订单
func smsSend(ctx context.Context) any {
	req := smsRequest{}
	if err := ctx.Request().Body().ScanTo(&req); err != nil {
		return errors.New(http.StatusBadRequest, "invalid body: "+err.Error())
	}
	content := "您的订单" + req.OrderNo + "已打包完成，共" + strconv.Itoa(len(req.Boxes)) + "箱，即将发货"
	ctx.Log().Infof("sms send: user_id=%s content=%s", req.UserID, content)
	return map[string]any{
		"sms_id":   "SMS-" + req.OrderNo,
		"order_no": req.OrderNo,
		"boxes":    len(req.Boxes),
		"content":  content,
	}
}

// robotMessage 收到的钉钉机器人消息
type robotMessage struct {
	Time string         `json:"time"`
	Body map[string]any `json:"body"`
}

var (
	robotMu   sync.Mutex
	robotMsgs []robotMessage
)

// robotSend 模拟钉钉群机器人：POST /robot/send?access_token=xx&timestamp=xx&sign=xx
// 校验 access_token 与加签，返回钉钉格式的 errcode/errmsg
func robotSend(ctx context.Context) any {
	query := ctx.Request().Query()
	if query.Get("access_token") != robotAccessToken {
		return map[string]any{"errcode": 300001, "errmsg": "token is not exist"}
	}
	if !validSign(query.Get("timestamp"), query.Get("sign")) {
		return map[string]any{"errcode": 310000, "errmsg": "sign not match"}
	}

	body := map[string]any{}
	if err := ctx.Request().Body().ScanTo(&body); err != nil {
		return map[string]any{"errcode": 400, "errmsg": "invalid body"}
	}
	ctx.Log().Infof("dingtalk robot message: %s", ctx.Request().Body().Bytes())

	robotMu.Lock()
	robotMsgs = append(robotMsgs, robotMessage{Time: time.Now().Format(time.DateTime), Body: body})
	if len(robotMsgs) > maxRobotMessages {
		robotMsgs = robotMsgs[len(robotMsgs)-maxRobotMessages:]
	}
	robotMu.Unlock()
	return map[string]any{"errcode": 0, "errmsg": "ok"}
}

// validSign 按钉钉加签规则校验：sign = Base64(HmacSHA256(secret, timestamp + "\n" + secret))，时间戳 1 小时内有效
func validSign(timestamp, sign string) bool {
	ms, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.UnixMilli(ms)).Abs() > time.Hour {
		return false
	}
	mac := hmac.New(sha256.New, []byte(robotSecret))
	mac.Write([]byte(timestamp + "\n" + robotSecret))
	return hmac.Equal([]byte(sign), []byte(base64.StdEncoding.EncodeToString(mac.Sum(nil))))
}

// robotMessages 查看最近收到的钉钉消息：GET /robot/messages
func robotMessages(ctx context.Context) any {
	robotMu.Lock()
	defer robotMu.Unlock()
	return append([]robotMessage{}, robotMsgs...)
}
