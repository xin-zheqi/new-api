package controller

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	wechatReconciliationInterval  = time.Minute
	wechatReconciliationTimeout   = 12 * time.Second
	wechatReconciliationBatchSize = 20
)

type wechatReconciliationCursor struct {
	PaymentExpireTime int64
	TradeNo           string
}

var (
	wechatReconciliationOnce    sync.Once
	wechatReconciliationRunning atomic.Bool
)

// StartWechatOrderReconciliationTask recovers expired Native orders even when
// the buyer closes the QR dialog before the final status poll. It is isolated
// to WeChat orders by both the model query and provider-checked transitions.
func StartWechatOrderReconciliationTask() {
	wechatReconciliationOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			ctx := context.Background()
			logger.LogInfo(ctx, fmt.Sprintf("wechat order reconciliation task started: tick=%s", wechatReconciliationInterval))
			ticker := time.NewTicker(wechatReconciliationInterval)
			defer ticker.Stop()

			cursor := wechatReconciliationCursor{}
			for {
				cursor = runWechatOrderReconciliationBatch(ctx, cursor)
				<-ticker.C
			}
		})
	})
}

func runWechatOrderReconciliationBatch(ctx context.Context, cursor wechatReconciliationCursor) wechatReconciliationCursor {
	if !wechatReconciliationRunning.CompareAndSwap(false, true) {
		return cursor
	}
	defer wechatReconciliationRunning.Store(false)

	settings := currentWechatRuntimeSettings()
	if !wechatCredentialsPresent(settings.Config) {
		return cursor
	}
	client, err := newWechatClient(settings.Config)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("wechat order reconciliation skipped: invalid configuration: %v", err))
		return cursor
	}
	orders, err := model.ListExpiredPendingWechatOrders(
		common.GetTimestamp(), cursor.PaymentExpireTime, cursor.TradeNo, wechatReconciliationBatchSize,
	)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("wechat order reconciliation query failed: %v", err))
		return cursor
	}

	nextCursor := wechatReconciliationCursor{}
	if len(orders) == wechatReconciliationBatchSize {
		last := orders[len(orders)-1]
		nextCursor = wechatReconciliationCursor{
			PaymentExpireTime: last.PaymentExpireTime,
			TradeNo:           last.TradeNo,
		}
	}
	for _, order := range orders {
		orderContext, cancel := context.WithTimeout(ctx, wechatReconciliationTimeout)
		status, reconcileErr := reconcileWechatPendingOrder(
			orderContext, client, settings.Config, order.TradeNo, true, true,
			"background_reconcile", "",
		)
		cancel()
		if reconcileErr != nil {
			logger.LogWarn(ctx, fmt.Sprintf(
				"wechat order reconciliation retained pending order: trade_no=%s status=%s error=%v",
				order.TradeNo, status, reconcileErr,
			))
		}
	}
	return nextCursor
}
