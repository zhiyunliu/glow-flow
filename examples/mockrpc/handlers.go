package main

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zhiyunliu/glue/context"
	"github.com/zhiyunliu/glue/errors"
)

type orderRequest struct {
	OrderNo   string  `json:"order_no"`
	UserID    string  `json:"user_id"`
	Amount    float64 `json:"amount"`
	PayAmount float64 `json:"pay_amount"`
}

// riskUsers 模拟风控名单
var riskUsers = map[string]string{
	"U7001": "设备指纹异常",
}

// riskEvaluate 风控评估：名单用户或金额 >= 10000 为 high，金额 >= 3000 为 medium，其余为 low
func riskEvaluate(ctx context.Context) any {
	req := orderRequest{}
	if err := ctx.Bind(&req); err != nil {
		return errors.New(http.StatusBadRequest, "invalid body: "+err.Error())
	}
	level, score, reason := "low", 10, "正常"
	switch {
	case riskUsers[req.UserID] != "":
		level, score, reason = "high", 95, riskUsers[req.UserID]
	case req.Amount >= 10000:
		level, score, reason = "high", 85, "大额订单"
	case req.Amount >= 3000:
		level, score, reason = "medium", 60, "金额偏高"
	}
	ctx.Log().Infof("risk evaluate: order_no=%s user_id=%s amount=%.2f level=%s", req.OrderNo, req.UserID, req.Amount, level)
	return map[string]any{
		"risk_level":  level,
		"risk_score":  score,
		"risk_reason": reason,
	}
}

type stockRequest struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

var reserveSeq atomic.Int64

// stockReserve 库存预占：请求体为订单中的单个商品；X 开头的商品无库存
func stockReserve(ctx context.Context) any {
	req := stockRequest{}
	if err := ctx.Bind(&req); err != nil {
		return errors.New(http.StatusBadRequest, "invalid body: "+err.Error())
	}
	result := map[string]any{"sku": req.SKU, "qty": req.Qty}
	if strings.HasPrefix(req.SKU, "X") {
		result["reserved"] = false
		result["reason"] = "库存不足"
	} else {
		result["reserved"] = true
		result["reserve_id"] = fmt.Sprintf("RSV-%s-%d", req.SKU, reserveSeq.Add(1))
		result["warehouse"] = "WH-SH-01"
	}
	ctx.Log().Infof("stock reserve: sku=%s qty=%d reserved=%v", req.SKU, req.Qty, result["reserved"])
	return result
}

var (
	pointsMu sync.Mutex
	balances = map[string]int{}
)

// pointsAdd 积分发放：按实付金额每 1 元积 1 分
func pointsAdd(ctx context.Context) any {
	req := orderRequest{}
	if err := ctx.Bind(&req); err != nil {
		return errors.New(http.StatusBadRequest, "invalid body: "+err.Error())
	}
	points := int(math.Floor(req.PayAmount))

	pointsMu.Lock()
	balances[req.UserID] += points
	balance := balances[req.UserID]
	pointsMu.Unlock()

	ctx.Log().Infof("points add: user_id=%s order_no=%s points=%d balance=%d", req.UserID, req.OrderNo, points, balance)
	return map[string]any{
		"user_id":  req.UserID,
		"order_no": req.OrderNo,
		"points":   points,
		"balance":  balance,
	}
}
