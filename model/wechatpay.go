package model

import (
	"errors"
	"sort"

	"github.com/QuantumNous/new-api/common"
)

// PendingWechatOrder identifies a WeChat order whose provider-side payment
// deadline has passed but whose local state still needs reconciliation.
type PendingWechatOrder struct {
	TradeNo           string
	PaymentExpireTime int64
}

// ListExpiredPendingWechatOrders returns a bounded, oldest-first batch across
// wallet and subscription orders. Provider and local-status predicates keep
// the recovery worker isolated from every other payment integration.
func ListExpiredPendingWechatOrders(now int64, afterExpireTime int64, afterTradeNo string, limit int) ([]PendingWechatOrder, error) {
	if now <= 0 {
		return nil, errors.New("invalid WeChat reconciliation timestamp")
	}
	if afterExpireTime < 0 || (afterExpireTime == 0 && afterTradeNo != "") {
		return nil, errors.New("invalid WeChat reconciliation cursor")
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	queryExpired := func(table any) ([]PendingWechatOrder, error) {
		orders := make([]PendingWechatOrder, 0, limit)
		query := DB.Model(table).
			Select("trade_no", "payment_expire_time").
			Where("payment_provider = ? AND status = ? AND payment_expire_time > 0 AND payment_expire_time <= ?",
				PaymentProviderWechat, common.TopUpStatusPending, now)
		if afterExpireTime > 0 {
			query = query.Where("(payment_expire_time > ? OR (payment_expire_time = ? AND trade_no > ?))",
				afterExpireTime, afterExpireTime, afterTradeNo)
		}
		err := query.
			Order("payment_expire_time ASC").
			Order("trade_no ASC").
			Limit(limit).
			Find(&orders).Error
		return orders, err
	}

	topUps, err := queryExpired(&TopUp{})
	if err != nil {
		return nil, err
	}
	subscriptions, err := queryExpired(&SubscriptionOrder{})
	if err != nil {
		return nil, err
	}
	orders := append(topUps, subscriptions...)
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].PaymentExpireTime == orders[j].PaymentExpireTime {
			return orders[i].TradeNo < orders[j].TradeNo
		}
		return orders[i].PaymentExpireTime < orders[j].PaymentExpireTime
	})
	if len(orders) > limit {
		orders = orders[:limit]
	}
	return orders, nil
}
