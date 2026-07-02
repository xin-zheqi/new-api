package model

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enableLotteryForTest(t *testing.T) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMap["LotteryEnabled"] = "true"
	common.OptionMapRWMutex.Unlock()
}

func insertLotteryUser(t *testing.T, id int) {
	t.Helper()
	user := &User{
		Id:           id,
		Username:     fmt.Sprintf("lottery_user_%d", id),
		DisplayName:  fmt.Sprintf("Lottery User %d", id),
		Status:       common.UserStatusEnabled,
		AffCode:      fmt.Sprintf("lottery_aff_%d", id),
		CreatedAt:    common.GetTimestamp() - 86400,
		RequestCount: 10,
	}
	require.NoError(t, DB.Create(user).Error)
}

func successfulTopUpForLotteryTest(t *testing.T, userId int, money float64) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userId,
		Amount:          10,
		Money:           money,
		TradeNo:         fmt.Sprintf("lottery_topup_%d", userId),
		PaymentMethod:   PaymentMethodManual,
		PaymentProvider: PaymentProviderManual,
		CreateTime:      common.GetTimestamp(),
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(topUp).Error)
}

func successfulLotteryRedemptionForTest(t *testing.T, userId int, quota int, redeemedTime int64) {
	t.Helper()
	redemption := &Redemption{
		UserId:       1,
		Key:          fmt.Sprintf("lottery_redemption_%d_%d", userId, redeemedTime),
		Status:       common.RedemptionCodeStatusUsed,
		Name:         "lottery redemption",
		Quota:        quota,
		RedeemType:   RedemptionTypeQuota,
		CreatedTime:  redeemedTime - 60,
		RedeemedTime: redeemedTime,
		UsedUserId:   userId,
	}
	require.NoError(t, DB.Create(redemption).Error)
}

func successfulLotterySubscriptionRedemptionForTest(t *testing.T, userId int, planId int, priceAmount float64, redeemedTime int64) {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:          planId,
		Title:       fmt.Sprintf("Lottery Plan %d", planId),
		PriceAmount: priceAmount,
		Currency:    "USD",
		Enabled:     true,
	}
	plan.NormalizeDefaults()
	require.NoError(t, DB.Create(plan).Error)
	redemption := &Redemption{
		UserId:             1,
		Key:                fmt.Sprintf("lottery_subscription_redemption_%d_%d", userId, redeemedTime),
		Status:             common.RedemptionCodeStatusUsed,
		Name:               "lottery subscription redemption",
		RedeemType:         RedemptionTypeSubscription,
		SubscriptionPlanId: planId,
		CreatedTime:        redeemedTime - 60,
		RedeemedTime:       redeemedTime,
		UsedUserId:         userId,
	}
	require.NoError(t, DB.Create(redemption).Error)
}

func TestCreateLotteryRequiresCodesForPrizeAllocation(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	_, err := CreateLottery(LotteryCreateRequest{
		Title:             "Small pool",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       2,
		PrizePerWinner:    1,
		RegistrationStart: now + 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"A"},
	}, 1)

	require.ErrorIs(t, err, ErrLotteryPrizeInsufficient)
}

func TestCreateLotteryDeduplicatesImportedPrizeCodes(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Duplicate codes",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		RegistrationStart: now + 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"A", " A ", "B", "", "B", "C"},
	}, 1)
	require.NoError(t, err)

	var codes []string
	require.NoError(t, DB.Model(&LotteryPrize{}).Where("lottery_id = ?", lottery.Id).Order("id asc").Pluck("code", &codes).Error)
	assert.Equal(t, []string{"A", "B", "C"}, codes)
}

func TestCreateLotteryRequiresRegistrationEndAfterNow(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	_, err := CreateLottery(LotteryCreateRequest{
		Title:             "Past registration end",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		RegistrationStart: now - 360,
		RegistrationEnd:   now - 60,
		DrawTime:          now + 120,
		PrizeCodes:        []string{"A"},
	}, 1)

	require.Error(t, err)
	require.ErrorContains(t, err, "报名结束时间必须晚于当前时间和报名开始时间")
}

func TestCreateLotteryAllowsDrawTimeEqualRegistrationEnd(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Equal draw time",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		RegistrationStart: now - 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 120,
		PrizeCodes:        []string{"A"},
	}, 1)

	require.NoError(t, err)
	require.NotNil(t, lottery)
}

func TestJoinLotteryRequiresRechargeWhenConfigured(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 101)
	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Recharge only",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		PrizePerWinner:    2,
		RequireRecharge:   true,
		RegistrationStart: now - 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"A", "B"},
	}, 1)
	require.NoError(t, err)

	err = JoinLotteryRound(lottery.Id, 101)
	require.ErrorIs(t, err, ErrLotteryRechargeRequired)

	successfulTopUpForLotteryTest(t, 101, 9.9)
	require.NoError(t, JoinLotteryRound(lottery.Id, 101))
}

func TestJoinLotteryRedemptionDoesNotCountAsRechargeByDefault(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 111)
	successfulLotteryRedemptionForTest(t, 111, int(common.QuotaPerUnit*10), common.GetTimestamp())

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Recharge gate default",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		PrizePerWinner:    1,
		RequireRecharge:   true,
		RegistrationStart: now - 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"A"},
	}, 1)
	require.NoError(t, err)

	err = JoinLotteryRound(lottery.Id, 111)
	require.ErrorIs(t, err, ErrLotteryRechargeRequired)
}

func TestJoinLotteryCanUseRedemptionAsRechargeWhenEnabled(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 112)
	successfulLotteryRedemptionForTest(t, 112, int(common.QuotaPerUnit*12), common.GetTimestamp())

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:                     "Recharge gate redeem",
		PrizeName:                 "Gift",
		Mode:                      LotteryModeOnce,
		WinnerCount:               1,
		PrizePerWinner:            1,
		RequireRecharge:           true,
		MinRechargeAmount:         10,
		CountRedemptionAsRecharge: true,
		RegistrationStart:         now - 60,
		RegistrationEnd:           now + 120,
		DrawTime:                  now + 180,
		PrizeCodes:                []string{"A"},
	}, 1)
	require.NoError(t, err)

	require.NoError(t, JoinLotteryRound(lottery.Id, 112))
}

func TestJoinLotteryRechargeWindowFiltersOutExpiredRecharge(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 113)
	topUp := &TopUp{
		UserId:          113,
		Amount:          10,
		Money:           8,
		TradeNo:         "lottery_topup_expired_113",
		PaymentMethod:   PaymentMethodManual,
		PaymentProvider: PaymentProviderManual,
		CreateTime:      common.GetTimestamp() - 15*86400,
		CompleteTime:    common.GetTimestamp() - 15*86400,
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(topUp).Error)

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:              "Recharge gate window",
		PrizeName:          "Gift",
		Mode:               LotteryModeOnce,
		WinnerCount:        1,
		PrizePerWinner:     1,
		RequireRecharge:    true,
		MinRechargeAmount:  5,
		RechargeWindowDays: 7,
		RegistrationStart:  now - 60,
		RegistrationEnd:    now + 120,
		DrawTime:           now + 180,
		PrizeCodes:         []string{"A"},
	}, 1)
	require.NoError(t, err)

	err = JoinLotteryRound(lottery.Id, 113)
	require.ErrorIs(t, err, ErrLotteryRechargeRequired)
}

func TestJoinLotterySubscriptionRedemptionDoesNotCountAsRechargeByDefault(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 114)
	successfulLotterySubscriptionRedemptionForTest(t, 114, 8001, 29.9, common.GetTimestamp())

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Subscription redeem default",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		PrizePerWinner:    1,
		RequireRecharge:   true,
		RegistrationStart: now - 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"A"},
	}, 1)
	require.NoError(t, err)

	err = JoinLotteryRound(lottery.Id, 114)
	require.ErrorIs(t, err, ErrLotteryRechargeRequired)
}

func TestJoinLotteryCanUseSubscriptionRedemptionAsRechargeWhenEnabled(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 115)
	successfulLotterySubscriptionRedemptionForTest(t, 115, 8002, 29.9, common.GetTimestamp())

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:                     "Subscription redeem enabled",
		PrizeName:                 "Gift",
		Mode:                      LotteryModeOnce,
		WinnerCount:               1,
		PrizePerWinner:            1,
		RequireRecharge:           true,
		MinRechargeAmount:         20,
		CountRedemptionAsRecharge: true,
		RegistrationStart:         now - 60,
		RegistrationEnd:           now + 120,
		DrawTime:                  now + 180,
		PrizeCodes:                []string{"A"},
	}, 1)
	require.NoError(t, err)

	require.NoError(t, JoinLotteryRound(lottery.Id, 115))
}

func TestJoinLotterySubscriptionRedemptionUsesPlanPriceAmount(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 116)
	successfulLotterySubscriptionRedemptionForTest(t, 116, 8003, 29.9, common.GetTimestamp())

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:                     "Subscription redeem threshold",
		PrizeName:                 "Gift",
		Mode:                      LotteryModeOnce,
		WinnerCount:               1,
		PrizePerWinner:            1,
		RequireRecharge:           true,
		MinRechargeAmount:         30,
		CountRedemptionAsRecharge: true,
		RegistrationStart:         now - 60,
		RegistrationEnd:           now + 120,
		DrawTime:                  now + 180,
		PrizeCodes:                []string{"A"},
	}, 1)
	require.NoError(t, err)

	err = JoinLotteryRound(lottery.Id, 116)
	require.ErrorIs(t, err, ErrLotteryRechargeRequired)

	var user User
	require.NoError(t, DB.Where("id = ?", 116).First(&user).Error)
	eligibility, err := EvaluateLotteryEligibility(DB, user, LotteryEligibilityRules{
		RequireRecharge:           true,
		MinRechargeAmount:         30,
		CountRedemptionAsRecharge: true,
	})
	require.NoError(t, err)
	require.NotNil(t, eligibility)
	require.False(t, eligibility.Eligible)
	require.NotEmpty(t, eligibility.Issues)
	assert.Equal(t, 30.0, eligibility.Issues[0].RequiredAmount)
	assert.InDelta(t, 29.9, eligibility.Issues[0].CurrentAmount, 0.0001)
	assert.InDelta(t, 0.1, eligibility.Issues[0].RemainingAmount, 0.0001)
}

func TestJoinLotteryRechargeWindowFiltersOutExpiredSubscriptionRedemption(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 117)
	successfulLotterySubscriptionRedemptionForTest(t, 117, 8004, 29.9, common.GetTimestamp()-15*86400)

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:                     "Subscription redeem window",
		PrizeName:                 "Gift",
		Mode:                      LotteryModeOnce,
		WinnerCount:               1,
		PrizePerWinner:            1,
		RequireRecharge:           true,
		MinRechargeAmount:         10,
		RechargeWindowDays:        7,
		CountRedemptionAsRecharge: true,
		RegistrationStart:         now - 60,
		RegistrationEnd:           now + 120,
		DrawTime:                  now + 180,
		PrizeCodes:                []string{"A"},
	}, 1)
	require.NoError(t, err)

	err = JoinLotteryRound(lottery.Id, 117)
	require.ErrorIs(t, err, ErrLotteryRechargeRequired)
}

func TestDrawLotteryAssignsOnlyConfiguredPrizeCountPerWinner(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Fair draw",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       2,
		PrizePerWinner:    2,
		RegistrationStart: now - 120,
		RegistrationEnd:   now + 60,
		DrawTime:          now + 120,
		PrizeCodes:        []string{"A", "B", "C", "D", "E"},
	}, 1)
	require.NoError(t, err)

	for userId := 201; userId <= 204; userId++ {
		insertLotteryUser(t, userId)
		require.NoError(t, JoinLotteryRound(lottery.Id, userId))
	}

	var round LotteryRound
	require.NoError(t, DB.Where("lottery_id = ?", lottery.Id).First(&round).Error)
	require.NoError(t, DrawLotteryRound(round.Id, 1))

	var winners []LotteryEntry
	require.NoError(t, DB.Where("round_id = ? AND is_winner = ?", round.Id, true).Find(&winners).Error)
	require.Len(t, winners, 2)

	var assigned []LotteryPrize
	require.NoError(t, DB.Where("round_id = ? AND status = ?", round.Id, "assigned").Find(&assigned).Error)
	require.Len(t, assigned, 4)

	for _, winner := range winners {
		var count int64
		require.NoError(t, DB.Model(&LotteryPrize{}).Where("round_id = ? AND winner_user_id = ?", round.Id, winner.UserId).Count(&count).Error)
		assert.EqualValues(t, 2, count)
	}

	var available int64
	require.NoError(t, DB.Model(&LotteryPrize{}).Where("lottery_id = ? AND status = ?", lottery.Id, "available").Count(&available).Error)
	assert.EqualValues(t, 1, available)
}

func TestLotteryPublicViewDoesNotExposeWinnerCodes(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Private codes",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		PrizePerWinner:    1,
		RegistrationStart: now - 120,
		RegistrationEnd:   now + 60,
		DrawTime:          now + 120,
		PrizeCodes:        []string{"SECRET-A", "SECRET-B"},
	}, 1)
	require.NoError(t, err)

	insertLotteryUser(t, 251)
	require.NoError(t, JoinLotteryRound(lottery.Id, 251))

	var round LotteryRound
	require.NoError(t, DB.Where("lottery_id = ?", lottery.Id).First(&round).Error)
	require.NoError(t, DrawLotteryRound(round.Id, 1))

	publicView, err := GetPublicLottery(lottery.Id, 251)
	require.NoError(t, err)
	assert.True(t, publicView.Won)
	require.Len(t, publicView.Winners, 1)
	assert.Empty(t, publicView.Winners[0].Prizes)
	assert.Empty(t, publicView.Winners[0].Username)
	assert.Zero(t, publicView.Winners[0].UserId)
	assert.Zero(t, publicView.AssignedPrizeCount)
	assert.Zero(t, publicView.AvailablePrizeCount)

	adminView, err := GetAdminLottery(lottery.Id)
	require.NoError(t, err)
	require.Len(t, adminView.Winners, 1)
	assert.Len(t, adminView.Winners[0].Prizes, 1)
	require.Len(t, adminView.Winners[0].PrizeDetails, 1)
	assert.Equal(t, "Gift", adminView.Winners[0].PrizeDetails[0].PrizeName)
	assert.Equal(t, adminView.Winners[0].Prizes[0], adminView.Winners[0].PrizeDetails[0].Code)
	assert.Equal(t, "lottery_user_251", adminView.Winners[0].Username)
	assert.Equal(t, 251, adminView.Winners[0].UserId)
	assert.EqualValues(t, 1, adminView.AssignedPrizeCount)
	assert.EqualValues(t, 1, adminView.AvailablePrizeCount)

	rounds, total, err := ListAdminLotteryRounds(lottery.Id, &common.PageInfo{Page: 1, PageSize: 10}, LotteryRoundListFilter{})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rounds, 1)
	require.Len(t, rounds[0].Winners, 1)
	assert.Equal(t, "lottery_user_251", rounds[0].Winners[0].Username)
	assert.Equal(t, 251, rounds[0].Winners[0].UserId)
	require.Len(t, rounds[0].Winners[0].PrizeDetails, 1)
	assert.Equal(t, "Gift", rounds[0].Winners[0].PrizeDetails[0].PrizeName)
	assert.Equal(t, rounds[0].Winners[0].Prizes[0], rounds[0].Winners[0].PrizeDetails[0].Code)
}

func TestMaskLotteryNameShortUserIds(t *testing.T) {
	assert.Equal(t, "*", maskLotteryName("a"))
	assert.Equal(t, "*", maskLotteryName("中"))
	assert.Equal(t, "a*", maskLotteryName("ab"))
	assert.Equal(t, "中*", maskLotteryName("中国"))
	assert.Equal(t, "a*c", maskLotteryName("abc"))
}

func TestPublicLotteriesFilterAndSortByDrawTime(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	far, err := CreateLottery(LotteryCreateRequest{
		Title:             "Far draw",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		RegistrationStart: now + 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 600,
		PrizeCodes:        []string{"A"},
	}, 1)
	require.NoError(t, err)
	near, err := CreateLottery(LotteryCreateRequest{
		Title:             "Near draw",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		RegistrationStart: now + 30,
		RegistrationEnd:   now + 90,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"B"},
	}, 1)
	require.NoError(t, err)
	finished, err := CreateLottery(LotteryCreateRequest{
		Title:             "Finished draw",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		RegistrationStart: now - 120,
		RegistrationEnd:   now + 60,
		DrawTime:          now + 120,
		PrizeCodes:        []string{"C"},
	}, 1)
	require.NoError(t, err)
	insertLotteryUser(t, 271)
	require.NoError(t, JoinLotteryRound(finished.Id, 271))
	var round LotteryRound
	require.NoError(t, DB.Where("lottery_id = ?", finished.Id).First(&round).Error)
	require.NoError(t, DB.Model(&LotteryRound{}).Where("id = ?", round.Id).Updates(map[string]interface{}{
		"registration_end": now - 60,
		"draw_time":        now - 30,
	}).Error)
	require.NoError(t, DrawLotteryRound(round.Id, 1))

	views, err := GetPublicLotteries(271, LotteryListFilter{})
	require.NoError(t, err)
	require.Len(t, views, 3)
	assert.Equal(t, finished.Id, views[0].Id)
	assert.Equal(t, near.Id, views[1].Id)
	assert.Equal(t, far.Id, views[2].Id)

	drawn, err := GetPublicLotteries(271, LotteryListFilter{DrawStatus: "drawn"})
	require.NoError(t, err)
	require.Len(t, drawn, 1)
	assert.Equal(t, finished.Id, drawn[0].Id)
	assert.True(t, drawn[0].Won)
	require.Len(t, drawn[0].Winners, 1)
	assert.NotEmpty(t, drawn[0].Winners[0].MaskedName)

	undrawn, err := GetPublicLotteries(271, LotteryListFilter{DrawStatus: "undrawn"})
	require.NoError(t, err)
	require.Len(t, undrawn, 2)
	assert.Equal(t, near.Id, undrawn[0].Id)
	assert.Equal(t, far.Id, undrawn[1].Id)
}

func TestUpdateLotteryAllowsOnlyUndrawnActivities(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Editable draw",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		RegistrationStart: now + 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"A"},
	}, 1)
	require.NoError(t, err)

	updated, err := UpdateLottery(lottery.Id, LotteryCreateRequest{
		Title:             "Updated draw",
		PrizeName:         "Updated gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       2,
		RegistrationStart: now + 90,
		RegistrationEnd:   now + 150,
		DrawTime:          now + 240,
		PrizeCodes:        []string{"B", "C"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated draw", updated.Title)
	assert.Equal(t, 2, updated.WinnerCount)

	var codes []string
	require.NoError(t, DB.Model(&LotteryPrize{}).Where("lottery_id = ?", lottery.Id).Order("id asc").Pluck("code", &codes).Error)
	assert.Equal(t, []string{"B", "C"}, codes)
	var updatedRound LotteryRound
	require.NoError(t, DB.Where("lottery_id = ?", lottery.Id).First(&updatedRound).Error)
	assert.Equal(t, now+240, updatedRound.DrawTime)

	insertLotteryUser(t, 281)
	require.NoError(t, DB.Model(&LotteryRound{}).Where("id = ?", updatedRound.Id).Updates(map[string]interface{}{
		"registration_start": now - 120,
		"registration_end":   now + 60,
		"status":             LotteryRoundStatusOpen,
	}).Error)
	require.NoError(t, JoinLotteryRound(lottery.Id, 281))
	require.NoError(t, DB.Model(&LotteryRound{}).Where("id = ?", updatedRound.Id).Updates(map[string]interface{}{
		"registration_end": now - 60,
		"draw_time":        now - 30,
	}).Error)
	require.NoError(t, DrawLotteryRound(updatedRound.Id, 1))

	_, err = UpdateLottery(lottery.Id, LotteryCreateRequest{
		Title:             "Blocked update",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		RegistrationStart: now + 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"D"},
	})
	require.ErrorIs(t, err, ErrLotteryNotEditable)
}

func TestOneTimeLotterySplitsImportedCodesAcrossWinners(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Split all possible codes",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       2,
		PrizePerWinner:    2,
		RegistrationStart: now - 120,
		RegistrationEnd:   now + 60,
		DrawTime:          now + 120,
		PrizeCodes:        []string{"A", "B", "C", "D", "E"},
	}, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, lottery.PrizePerWinner)

	for userId := 261; userId <= 263; userId++ {
		insertLotteryUser(t, userId)
		require.NoError(t, JoinLotteryRound(lottery.Id, userId))
	}

	var round LotteryRound
	require.NoError(t, DB.Where("lottery_id = ?", lottery.Id).First(&round).Error)
	require.NoError(t, DrawLotteryRound(round.Id, 1))

	var assigned int64
	require.NoError(t, DB.Model(&LotteryPrize{}).Where("round_id = ? AND status = ?", round.Id, "assigned").Count(&assigned).Error)
	assert.EqualValues(t, 4, assigned)

	var winners []LotteryEntry
	require.NoError(t, DB.Where("round_id = ? AND is_winner = ?", round.Id, true).Find(&winners).Error)
	for _, winner := range winners {
		var count int64
		require.NoError(t, DB.Model(&LotteryPrize{}).Where("round_id = ? AND winner_user_id = ?", round.Id, winner.UserId).Count(&count).Error)
		assert.EqualValues(t, 2, count)
	}
}

func TestScheduledLotteryKeepsUnusedCodesForFutureRounds(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery := &Lottery{
		Title:             "Scheduled",
		PrizeName:         "Gift",
		Mode:              LotteryModeScheduled,
		Status:            LotteryStatusEnabled,
		WinnerCount:       1,
		PrizePerWinner:    2,
		ScheduleWeekdays:  "1,2,3,4,5",
		ScheduleStartTime: "09:00",
		ScheduleEndTime:   "18:00",
		ScheduleDrawTime:  "20:00",
	}
	require.NoError(t, DB.Create(lottery).Error)
	round := &LotteryRound{
		LotteryId:         lottery.Id,
		RoundKey:          "manual",
		Status:            LotteryRoundStatusOpen,
		RegistrationStart: now - 120,
		RegistrationEnd:   now + 60,
		DrawTime:          now + 120,
	}
	require.NoError(t, DB.Create(round).Error)
	for _, code := range []string{"A", "B", "C", "D", "E", "F"} {
		require.NoError(t, DB.Create(&LotteryPrize{
			LotteryId: lottery.Id,
			PrizeName: lottery.PrizeName,
			Code:      code,
			Status:    "available",
		}).Error)
	}
	for userId := 301; userId <= 303; userId++ {
		insertLotteryUser(t, userId)
		require.NoError(t, JoinLotteryRound(lottery.Id, userId))
	}

	require.NoError(t, DrawLotteryRound(round.Id, 1))

	var assigned int64
	require.NoError(t, DB.Model(&LotteryPrize{}).Where("lottery_id = ? AND status = ?", lottery.Id, "assigned").Count(&assigned).Error)
	assert.EqualValues(t, 2, assigned)

	var available int64
	require.NoError(t, DB.Model(&LotteryPrize{}).Where("lottery_id = ? AND status = ?", lottery.Id, "available").Count(&available).Error)
	assert.EqualValues(t, 4, available)

	nextRound := &LotteryRound{
		LotteryId:         lottery.Id,
		RoundKey:          "next",
		Status:            LotteryRoundStatusOpen,
		RegistrationStart: now + 3600,
		RegistrationEnd:   now + 7200,
		DrawTime:          now + 10800,
	}
	require.NoError(t, DB.Create(nextRound).Error)

	adminView, err := GetAdminLottery(lottery.Id)
	require.NoError(t, err)
	require.NotNil(t, adminView.Round)
	assert.Equal(t, nextRound.Id, adminView.Round.Id)

	foundFinishedRound := false
	for _, detail := range adminView.Rounds {
		if detail.Round.Id != round.Id {
			continue
		}
		foundFinishedRound = true
		require.Len(t, detail.Winners, 1)
		assert.Len(t, detail.Winners[0].Prizes, 2)
	}
	assert.True(t, foundFinishedRound)
}

func TestDisableScheduledLotteryCancelsCurrentRoundAfterFinishedRound(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery := &Lottery{
		Title:             "Scheduled disable",
		PrizeName:         "Gift",
		Mode:              LotteryModeScheduled,
		Status:            LotteryStatusEnabled,
		WinnerCount:       1,
		PrizePerWinner:    1,
		ScheduleWeekdays:  "1,2,3,4,5",
		ScheduleStartTime: "09:00",
		ScheduleEndTime:   "18:00",
		ScheduleDrawTime:  "20:00",
	}
	require.NoError(t, DB.Create(lottery).Error)
	require.NoError(t, DB.Create(&LotteryRound{
		LotteryId:         lottery.Id,
		RoundKey:          "finished",
		Status:            LotteryRoundStatusFinished,
		RegistrationStart: now - 10800,
		RegistrationEnd:   now - 7200,
		DrawTime:          now - 3600,
		DrawnAt:           now - 3500,
	}).Error)
	currentRound := &LotteryRound{
		LotteryId:         lottery.Id,
		RoundKey:          "current",
		Status:            LotteryRoundStatusOpen,
		RegistrationStart: now - 60,
		RegistrationEnd:   now + 3600,
		DrawTime:          now + 7200,
	}
	require.NoError(t, DB.Create(currentRound).Error)

	require.NoError(t, UpdateLotteryStatus(lottery.Id, LotteryStatusDisabled))

	var round LotteryRound
	require.NoError(t, DB.Where("id = ?", currentRound.Id).First(&round).Error)
	assert.Equal(t, LotteryRoundStatusCancelled, round.Status)
}

func TestScheduledLotteryRefreshKeepsFinishedRoundAndCreatesNextRound(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery := &Lottery{
		Title:             "Scheduled refresh",
		PrizeName:         "Gift",
		Mode:              LotteryModeScheduled,
		Status:            LotteryStatusEnabled,
		WinnerCount:       1,
		PrizePerWinner:    1,
		ScheduleWeekdays:  "0,1,2,3,4,5,6",
		ScheduleStartTime: "00:01",
		ScheduleEndTime:   "00:02",
		ScheduleDrawTime:  "00:03",
	}
	require.NoError(t, DB.Create(lottery).Error)
	finishedRound := &LotteryRound{
		LotteryId:         lottery.Id,
		RoundKey:          "finished",
		Status:            LotteryRoundStatusFinished,
		RegistrationStart: now - 10800,
		RegistrationEnd:   now - 7200,
		DrawTime:          now - 3600,
		DrawnAt:           now - 3500,
	}
	require.NoError(t, DB.Create(finishedRound).Error)

	require.NoError(t, EnsureScheduledLotteryRounds())

	var reloaded LotteryRound
	require.NoError(t, DB.Where("id = ?", finishedRound.Id).First(&reloaded).Error)
	assert.Equal(t, LotteryRoundStatusFinished, reloaded.Status)
	assert.Equal(t, finishedRound.DrawnAt, reloaded.DrawnAt)

	var activeCount int64
	require.NoError(t, DB.Model(&LotteryRound{}).
		Where("lottery_id = ? AND status IN ?", lottery.Id, []string{LotteryRoundStatusPending, LotteryRoundStatusOpen, LotteryRoundStatusDrawing}).
		Count(&activeCount).Error)
	assert.EqualValues(t, 1, activeCount)
}

func TestDeleteLotteryHidesPublicActivityAndKeepsAdminDeletedStatus(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 501)
	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Delete me",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		PrizePerWinner:    1,
		RegistrationStart: now - 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"A"},
	}, 1)
	require.NoError(t, err)
	require.NoError(t, DeleteLottery(lottery.Id))

	_, err = GetPublicLottery(lottery.Id, 501)
	require.ErrorIs(t, err, ErrLotteryNotFound)
	require.ErrorIs(t, JoinLotteryRound(lottery.Id, 501), ErrLotteryNotFound)

	views, total, err := ListAdminLotteries(&common.PageInfo{Page: 1, PageSize: 20}, LotteryListFilter{Status: strconv.Itoa(LotteryStatusDeleted)})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, views, 1)
	assert.Equal(t, lottery.Id, views[0].Id)
	assert.Equal(t, LotteryStatusDeleted, views[0].Status)
	assert.True(t, views[0].Deleted)
	assert.False(t, views[0].CanEdit)
}

func TestDrawLotteryRecordsStructuredWinnerLog(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "中文抽奖标题",
		PrizeName:         "中文奖品名称",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		PrizePerWinner:    1,
		RegistrationStart: now - 120,
		RegistrationEnd:   now + 60,
		DrawTime:          now + 120,
		PrizeCodes:        []string{"A"},
	}, 1)
	require.NoError(t, err)

	insertLotteryUser(t, 601)
	require.NoError(t, JoinLotteryRound(lottery.Id, 601))

	var round LotteryRound
	require.NoError(t, DB.Where("lottery_id = ?", lottery.Id).First(&round).Error)
	require.NoError(t, DrawLotteryRound(round.Id, 1))

	var log Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", 601, LogTypeSystem).Order("id desc").First(&log).Error)
	assert.Equal(t, "抽奖中奖："+lottery.Title+"，奖品："+lottery.PrizeName, log.Content)

	otherMap, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	op, ok := otherMap["op"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "lottery.win", op["action"])

	params, ok := op["params"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, lottery.Title, params["title"])
	assert.Equal(t, lottery.PrizeName, params["prize_name"])
}

func TestJoinLotteryPreventsDuplicateEntry(t *testing.T) {
	truncateTables(t)
	enableLotteryForTest(t)

	insertLotteryUser(t, 401)
	now := common.GetTimestamp()
	lottery, err := CreateLottery(LotteryCreateRequest{
		Title:             "Duplicate guard",
		PrizeName:         "Gift",
		Mode:              LotteryModeOnce,
		WinnerCount:       1,
		PrizePerWinner:    1,
		RegistrationStart: now - 60,
		RegistrationEnd:   now + 120,
		DrawTime:          now + 180,
		PrizeCodes:        []string{"A", "B"},
	}, 1)
	require.NoError(t, err)

	require.NoError(t, JoinLotteryRound(lottery.Id, 401))
	err = JoinLotteryRound(lottery.Id, 401)
	require.True(t, errors.Is(err, ErrLotteryAlreadyJoined))
}
