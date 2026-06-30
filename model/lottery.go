package model

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	LotteryModeOnce      = "once"
	LotteryModeScheduled = "scheduled"

	LotteryStatusEnabled  = 1
	LotteryStatusDisabled = 2
	LotteryStatusDeleted  = 3

	LotteryRoundStatusPending            = "pending"
	LotteryRoundStatusOpen               = "open"
	LotteryRoundStatusDrawing            = "drawing"
	LotteryRoundStatusFinished           = "finished"
	LotteryRoundStatusCancelled          = "cancelled"
	LotteryRoundStatusInsufficientPrizes = "insufficient_prizes"
)

var (
	ErrLotteryDisabled            = errors.New("抽奖功能未开启")
	ErrLotteryNotFound            = errors.New("抽奖不存在")
	ErrLotteryNotOpen             = errors.New("当前不在报名时间内")
	ErrLotteryAlreadyJoined       = errors.New("你已经参与过本轮抽奖")
	ErrLotteryRechargeRequired    = errors.New("参与抽奖需要先完成一次充值")
	ErrLotteryAccountAgeRequired  = errors.New("账号注册时间不满足参与条件")
	ErrLotteryRequestCountInvalid = errors.New("请求次数不满足参与条件")
	ErrLotteryEmailRequired       = errors.New("需要绑定邮箱后才能参与抽奖")
	ErrLotteryPrizeInsufficient   = errors.New("奖品兑换码数量不足")
	ErrLotteryParticipantsEmpty   = errors.New("暂无用户参与抽奖")
	ErrLotteryInvalidSchedule     = errors.New("定时抽奖计划无效")
	ErrLotteryNotEditable         = errors.New("已开奖或开奖中的一次性抽奖不能修改")
	ErrLotteryDeleted             = errors.New("抽奖已删除")
)

type Lottery struct {
	Id                int            `json:"id"`
	Title             string         `json:"title" gorm:"type:varchar(128);not null;index"`
	Description       string         `json:"description" gorm:"type:text"`
	PrizeName         string         `json:"prize_name" gorm:"type:varchar(128);not null"`
	Mode              string         `json:"mode" gorm:"type:varchar(16);not null;default:'once';index"`
	Status            int            `json:"status" gorm:"type:int;not null;default:1;index"`
	WinnerCount       int            `json:"winner_count" gorm:"type:int;not null;default:1"`
	PrizePerWinner    int            `json:"prize_per_winner" gorm:"type:int;not null;default:1"`
	RegistrationStart int64          `json:"registration_start" gorm:"index"`
	RegistrationEnd   int64          `json:"registration_end" gorm:"index"`
	DrawTime          int64          `json:"draw_time" gorm:"index"`
	ScheduleWeekdays  string         `json:"schedule_weekdays" gorm:"type:varchar(32);default:''"`
	ScheduleStartTime string         `json:"schedule_start_time" gorm:"type:varchar(5);default:''"`
	ScheduleEndTime   string         `json:"schedule_end_time" gorm:"type:varchar(5);default:''"`
	ScheduleDrawTime  string         `json:"schedule_draw_time" gorm:"type:varchar(5);default:''"`
	CreatedBy         int            `json:"created_by" gorm:"index"`
	CreatedAt         int64          `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt         int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`
}

type LotteryRound struct {
	Id                int    `json:"id"`
	LotteryId         int    `json:"lottery_id" gorm:"index;not null"`
	RoundKey          string `json:"round_key" gorm:"type:varchar(32);index;not null"`
	Status            string `json:"status" gorm:"type:varchar(32);not null;default:'pending';index"`
	RegistrationStart int64  `json:"registration_start" gorm:"index"`
	RegistrationEnd   int64  `json:"registration_end" gorm:"index"`
	DrawTime          int64  `json:"draw_time" gorm:"index"`
	DrawnAt           int64  `json:"drawn_at" gorm:"index"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type LotteryPrize struct {
	Id           int    `json:"id"`
	LotteryId    int    `json:"lottery_id" gorm:"index;not null"`
	RoundId      int    `json:"round_id" gorm:"index;default:0"`
	WinnerUserId int    `json:"winner_user_id" gorm:"index;default:0"`
	PrizeName    string `json:"prize_name" gorm:"type:varchar(128);not null"`
	Code         string `json:"code,omitempty" gorm:"type:text;not null"`
	Status       string `json:"status" gorm:"type:varchar(32);not null;default:'available';index"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime;index"`
	AssignedAt   int64  `json:"assigned_at" gorm:"index"`
}

type LotteryEntry struct {
	Id          int    `json:"id"`
	LotteryId   int    `json:"lottery_id" gorm:"index;not null"`
	RoundId     int    `json:"round_id" gorm:"uniqueIndex:idx_lottery_round_user;index;not null"`
	UserId      int    `json:"user_id" gorm:"uniqueIndex:idx_lottery_round_user;index;not null"`
	Username    string `json:"username" gorm:"type:varchar(64);not null"`
	DisplayName string `json:"display_name" gorm:"type:varchar(64);default:''"`
	MaskedName  string `json:"masked_name" gorm:"-:all"`
	IsWinner    bool   `json:"is_winner" gorm:"index;not null;default:false"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime;index"`
	WonAt       int64  `json:"won_at" gorm:"index"`
}

type LotterySettings struct {
	Enabled                   bool                      `json:"enabled"`
	RequireRecharge           bool                      `json:"require_recharge"`
	MinRechargeAmount         float64                   `json:"min_recharge_amount"`
	RechargeWindowDays        int                       `json:"recharge_window_days"`
	CountRedemptionAsRecharge bool                      `json:"count_redemption_as_recharge"`
	MinAccountAgeDays         int                       `json:"min_account_age_days"`
	MinRequestCount           int                       `json:"min_request_count"`
	RequireEmailVerified      bool                      `json:"require_email_verified"`
	Eligibility               *LotteryEligibilityStatus `json:"eligibility,omitempty"`
}

type LotteryEligibilityIssue struct {
	Code                      string  `json:"code"`
	Message                   string  `json:"message"`
	RequiredAmount            float64 `json:"required_amount,omitempty"`
	CurrentAmount             float64 `json:"current_amount,omitempty"`
	RemainingAmount           float64 `json:"remaining_amount,omitempty"`
	WindowDays                int     `json:"window_days,omitempty"`
	CountRedemptionAsRecharge bool    `json:"count_redemption_as_recharge,omitempty"`
}

type LotteryEligibilityStatus struct {
	Eligible bool                      `json:"eligible"`
	Issues   []LotteryEligibilityIssue `json:"issues,omitempty"`
}

type LotteryCreateRequest struct {
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	PrizeName         string   `json:"prize_name"`
	Mode              string   `json:"mode"`
	WinnerCount       int      `json:"winner_count"`
	PrizePerWinner    int      `json:"prize_per_winner"`
	RegistrationStart int64    `json:"registration_start"`
	RegistrationEnd   int64    `json:"registration_end"`
	DrawTime          int64    `json:"draw_time"`
	ScheduleWeekdays  []int    `json:"schedule_weekdays"`
	ScheduleStartTime string   `json:"schedule_start_time"`
	ScheduleEndTime   string   `json:"schedule_end_time"`
	ScheduleDrawTime  string   `json:"schedule_draw_time"`
	PrizeCodes        []string `json:"prize_codes"`
}

type LotteryListFilter struct {
	Status     string
	Mode       string
	Query      string
	DrawStatus string
}

type LotteryParticipantView struct {
	Id         int    `json:"id"`
	MaskedName string `json:"masked_name"`
	JoinedAt   int64  `json:"joined_at"`
	IsWinner   bool   `json:"is_winner,omitempty"`
}

type LotteryWinnerView struct {
	UserId     int      `json:"user_id,omitempty"`
	Username   string   `json:"username,omitempty"`
	MaskedName string   `json:"masked_name"`
	WonAt      int64    `json:"won_at"`
	Prizes     []string `json:"prizes,omitempty"`
}

type LotteryRoundDetailView struct {
	Round            LotteryRound        `json:"round"`
	ParticipantCount int64               `json:"participant_count"`
	Winners          []LotteryWinnerView `json:"winners,omitempty"`
}

type LotteryPublicView struct {
	Id                  int                       `json:"id"`
	Title               string                    `json:"title"`
	Description         string                    `json:"description"`
	PrizeName           string                    `json:"prize_name"`
	Mode                string                    `json:"mode"`
	Status              int                       `json:"status"`
	WinnerCount         int                       `json:"winner_count"`
	PrizePerWinner      int                       `json:"prize_per_winner"`
	ScheduleWeekdays    []int                     `json:"schedule_weekdays,omitempty"`
	ScheduleStartTime   string                    `json:"schedule_start_time,omitempty"`
	ScheduleEndTime     string                    `json:"schedule_end_time,omitempty"`
	ScheduleDrawTime    string                    `json:"schedule_draw_time,omitempty"`
	Round               *LotteryRound             `json:"round,omitempty"`
	ParticipantCount    int64                     `json:"participant_count"`
	Participants        []LotteryParticipantView  `json:"participants"`
	Joined              bool                      `json:"joined"`
	Won                 bool                      `json:"won"`
	Eligibility         *LotteryEligibilityStatus `json:"eligibility,omitempty"`
	CanEdit             bool                      `json:"can_edit,omitempty"`
	Winners             []LotteryWinnerView       `json:"winners,omitempty"`
	Rounds              []LotteryRoundDetailView  `json:"rounds,omitempty"`
	AvailablePrizeCount int64                     `json:"available_prize_count,omitempty"`
	PrizeCodes          []string                  `json:"prize_codes,omitempty"`
	CreatedAt           int64                     `json:"created_at"`
	Deleted             bool                      `json:"deleted,omitempty"`
}

type LotteryPrizeView struct {
	Id        int    `json:"id"`
	LotteryId int    `json:"lottery_id"`
	RoundId   int    `json:"round_id"`
	Title     string `json:"title"`
	PrizeName string `json:"prize_name"`
	Code      string `json:"code"`
	WonAt     int64  `json:"won_at"`
	DrawTime  int64  `json:"draw_time"`
}

type lotteryRechargeEligibility struct {
	Eligible                  bool
	RecordFound               bool
	RequiredAmount            float64
	CurrentAmount             float64
	RemainingAmount           float64
	WindowDays                int
	CountRedemptionAsRecharge bool
}

func LotterySettingsFromOptions() LotterySettings {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return LotterySettings{
		Enabled:                   common.OptionMap["LotteryEnabled"] == "true",
		RequireRecharge:           common.OptionMap["LotteryRequireRecharge"] == "true",
		MinRechargeAmount:         parseLotteryFloatOption(common.OptionMap["LotteryMinRechargeAmount"]),
		RechargeWindowDays:        parseLotteryIntOption(common.OptionMap["LotteryRechargeWindowDays"]),
		CountRedemptionAsRecharge: common.OptionMap["LotteryCountRedemptionAsRecharge"] == "true",
		MinAccountAgeDays:         parseLotteryIntOption(common.OptionMap["LotteryMinAccountAgeDays"]),
		MinRequestCount:           parseLotteryIntOption(common.OptionMap["LotteryMinRequestCount"]),
		RequireEmailVerified:      common.OptionMap["LotteryRequireEmailVerified"] == "true",
	}
}

func LotterySettingsForUser(userId int) (LotterySettings, error) {
	settings := LotterySettingsFromOptions()
	if userId <= 0 {
		return settings, nil
	}
	var user User
	if err := DB.Select("id", "email", "created_at", "request_count").Where("id = ?", userId).First(&user).Error; err != nil {
		return settings, err
	}
	eligibility, err := EvaluateLotteryEligibility(DB, user, settings)
	if err != nil {
		return settings, err
	}
	settings.Eligibility = eligibility
	return settings, nil
}

func isLotteryDeletedStatus(status int) bool {
	return status == LotteryStatusDeleted
}

func parseLotteryIntOption(value string) int {
	n, _ := strconv.Atoi(value)
	if n < 0 {
		return 0
	}
	return n
}

func parseLotteryFloatOption(value string) float64 {
	n, _ := strconv.ParseFloat(value, 64)
	if n < 0 {
		return 0
	}
	return n
}

func normalizeLotteryCreateRequest(req LotteryCreateRequest) (LotteryCreateRequest, error) {
	req.Title = strings.TrimSpace(req.Title)
	req.PrizeName = strings.TrimSpace(req.PrizeName)
	req.Description = strings.TrimSpace(req.Description)
	req.Mode = strings.TrimSpace(strings.ToLower(req.Mode))
	if req.Mode == "" {
		req.Mode = LotteryModeOnce
	}
	if req.Mode != LotteryModeOnce && req.Mode != LotteryModeScheduled {
		return req, errors.New("抽奖模式无效")
	}
	if req.Title == "" || utf8.RuneCountInString(req.Title) > 128 {
		return req, errors.New("抽奖标题长度无效")
	}
	if req.PrizeName == "" || utf8.RuneCountInString(req.PrizeName) > 128 {
		return req, errors.New("奖品名称长度无效")
	}
	if req.WinnerCount <= 0 || req.WinnerCount > 100 {
		return req, errors.New("中奖人数必须在 1 到 100 之间")
	}
	codes := cleanLotteryPrizeCodes(req.PrizeCodes)
	if req.Mode == LotteryModeOnce && req.PrizePerWinner <= 0 {
		req.PrizePerWinner = len(codes) / req.WinnerCount
	} else if req.PrizePerWinner <= 0 {
		req.PrizePerWinner = len(codes) / req.WinnerCount
	}
	if req.PrizePerWinner <= 0 {
		req.PrizePerWinner = 1
	}
	if req.PrizePerWinner > 100 {
		return req, errors.New("每人奖品数量不能超过 100")
	}
	required := req.WinnerCount * req.PrizePerWinner
	if len(codes) < required {
		return req, ErrLotteryPrizeInsufficient
	}
	req.PrizeCodes = codes
	if req.Mode == LotteryModeOnce {
		now := common.GetTimestamp()
		if req.RegistrationStart <= 0 {
			return req, errors.New("报名开始时间无效")
		}
		if req.RegistrationEnd <= req.RegistrationStart || req.RegistrationEnd <= now {
			return req, errors.New("报名结束时间必须晚于当前时间和报名开始时间")
		}
		if req.DrawTime < req.RegistrationEnd {
			return req, errors.New("开奖时间必须晚于或等于报名结束时间")
		}
		return req, nil
	}
	weekdays, err := normalizeLotteryWeekdays(req.ScheduleWeekdays)
	if err != nil {
		return req, err
	}
	req.ScheduleWeekdays = weekdays
	if !validLotteryClock(req.ScheduleStartTime) || !validLotteryClock(req.ScheduleEndTime) || !validLotteryClock(req.ScheduleDrawTime) {
		return req, ErrLotteryInvalidSchedule
	}
	startMinute := lotteryClockMinute(req.ScheduleStartTime)
	endMinute := lotteryClockMinute(req.ScheduleEndTime)
	drawMinute := lotteryClockMinute(req.ScheduleDrawTime)
	if !(startMinute < endMinute && endMinute < drawMinute) {
		return req, ErrLotteryInvalidSchedule
	}
	return req, nil
}

func cleanLotteryPrizeCodes(codes []string) []string {
	clean := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		clean = append(clean, code)
	}
	return clean
}

func normalizeLotteryWeekdays(days []int) ([]int, error) {
	if len(days) == 0 {
		return nil, ErrLotteryInvalidSchedule
	}
	seen := map[int]struct{}{}
	for _, day := range days {
		if day < 0 || day > 6 {
			return nil, ErrLotteryInvalidSchedule
		}
		seen[day] = struct{}{}
	}
	normalized := make([]int, 0, len(seen))
	for day := range seen {
		normalized = append(normalized, day)
	}
	sort.Ints(normalized)
	return normalized, nil
}

func validLotteryClock(value string) bool {
	if len(value) != 5 || value[2] != ':' {
		return false
	}
	hour, err1 := strconv.Atoi(value[:2])
	minute, err2 := strconv.Atoi(value[3:])
	return err1 == nil && err2 == nil && hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59
}

func lotteryClockMinute(value string) int {
	hour, _ := strconv.Atoi(value[:2])
	minute, _ := strconv.Atoi(value[3:])
	return hour*60 + minute
}

func lotteryWeekdaysToString(days []int) string {
	parts := make([]string, 0, len(days))
	for _, day := range days {
		parts = append(parts, strconv.Itoa(day))
	}
	return strings.Join(parts, ",")
}

func lotteryWeekdaysFromString(value string) []int {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	days := make([]int, 0, len(parts))
	for _, part := range parts {
		day, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && day >= 0 && day <= 6 {
			days = append(days, day)
		}
	}
	return days
}

func CreateLottery(req LotteryCreateRequest, creatorId int) (*Lottery, error) {
	req, err := normalizeLotteryCreateRequest(req)
	if err != nil {
		return nil, err
	}
	lottery := &Lottery{
		Title:             req.Title,
		Description:       req.Description,
		PrizeName:         req.PrizeName,
		Mode:              req.Mode,
		Status:            LotteryStatusEnabled,
		WinnerCount:       req.WinnerCount,
		PrizePerWinner:    req.PrizePerWinner,
		RegistrationStart: req.RegistrationStart,
		RegistrationEnd:   req.RegistrationEnd,
		DrawTime:          req.DrawTime,
		ScheduleWeekdays:  lotteryWeekdaysToString(req.ScheduleWeekdays),
		ScheduleStartTime: req.ScheduleStartTime,
		ScheduleEndTime:   req.ScheduleEndTime,
		ScheduleDrawTime:  req.ScheduleDrawTime,
		CreatedBy:         creatorId,
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(lottery).Error; err != nil {
			return err
		}
		prizes := make([]LotteryPrize, 0, len(req.PrizeCodes))
		for _, code := range req.PrizeCodes {
			prizes = append(prizes, LotteryPrize{
				LotteryId: lottery.Id,
				PrizeName: lottery.PrizeName,
				Code:      code,
				Status:    "available",
			})
		}
		if len(prizes) > 0 {
			if err := tx.Create(&prizes).Error; err != nil {
				return err
			}
		}
		if lottery.Mode == LotteryModeOnce {
			round := LotteryRound{
				LotteryId:         lottery.Id,
				RoundKey:          strconv.FormatInt(lottery.DrawTime, 10),
				Status:            LotteryRoundStatusPending,
				RegistrationStart: lottery.RegistrationStart,
				RegistrationEnd:   lottery.RegistrationEnd,
				DrawTime:          lottery.DrawTime,
			}
			return tx.Create(&round).Error
		}
		round, err := nextScheduledLotteryRound(lottery, time.Now())
		if err != nil {
			return err
		}
		return tx.Create(round).Error
	})
	if err != nil {
		return nil, err
	}
	return lottery, nil
}

func UpdateLottery(lotteryId int, req LotteryCreateRequest) (*Lottery, error) {
	req, err := normalizeLotteryCreateRequest(req)
	if err != nil {
		return nil, err
	}
	var lottery Lottery
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", lotteryId).First(&lottery).Error; err != nil {
			return ErrLotteryNotFound
		}
		if isLotteryDeletedStatus(lottery.Status) {
			return ErrLotteryDeleted
		}
		if editable, err := isLotteryEditable(tx, lottery.Id); err != nil {
			return err
		} else if !editable {
			return ErrLotteryNotEditable
		}

		updates := map[string]interface{}{
			"title":               req.Title,
			"description":         req.Description,
			"prize_name":          req.PrizeName,
			"mode":                req.Mode,
			"winner_count":        req.WinnerCount,
			"prize_per_winner":    req.PrizePerWinner,
			"registration_start":  req.RegistrationStart,
			"registration_end":    req.RegistrationEnd,
			"draw_time":           req.DrawTime,
			"schedule_weekdays":   lotteryWeekdaysToString(req.ScheduleWeekdays),
			"schedule_start_time": req.ScheduleStartTime,
			"schedule_end_time":   req.ScheduleEndTime,
			"schedule_draw_time":  req.ScheduleDrawTime,
		}
		if err := tx.Model(&Lottery{}).Where("id = ?", lottery.Id).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(&LotteryPrize{}).
			Where("lottery_id = ? AND status = ?", lottery.Id, "available").
			Delete(&LotteryPrize{}).Error; err != nil {
			return err
		}
		prizes := make([]LotteryPrize, 0, len(req.PrizeCodes))
		for _, code := range req.PrizeCodes {
			prizes = append(prizes, LotteryPrize{
				LotteryId: lottery.Id,
				PrizeName: req.PrizeName,
				Code:      code,
				Status:    "available",
			})
		}
		if len(prizes) > 0 {
			if err := tx.Create(&prizes).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id = ?", lottery.Id).First(&lottery).Error; err != nil {
			return err
		}
		if err := replaceEditableLotteryRound(tx, &lottery); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &lottery, nil
}

func isLotteryEditable(tx *gorm.DB, lotteryId int) (bool, error) {
	var lottery Lottery
	if err := tx.Where("id = ?", lotteryId).First(&lottery).Error; err != nil {
		return false, err
	}
	if isLotteryDeletedStatus(lottery.Status) {
		return false, nil
	}
	var blocked int64
	if err := tx.Model(&LotteryRound{}).
		Where("lottery_id = ? AND status IN ?", lotteryId, []string{LotteryRoundStatusDrawing}).
		Count(&blocked).Error; err != nil {
		return false, err
	}
	if blocked > 0 {
		return false, nil
	}
	if lottery.Mode == LotteryModeScheduled {
		return true, nil
	}
	if err := tx.Model(&LotteryRound{}).
		Where("lottery_id = ? AND status IN ?", lotteryId, []string{LotteryRoundStatusFinished, LotteryRoundStatusInsufficientPrizes}).
		Count(&blocked).Error; err != nil {
		return false, err
	}
	if blocked > 0 {
		return false, nil
	}
	var assigned int64
	if err := tx.Model(&LotteryPrize{}).
		Where("lottery_id = ? AND status <> ?", lotteryId, "available").
		Count(&assigned).Error; err != nil {
		return false, err
	}
	return assigned == 0, nil
}

func replaceEditableLotteryRound(tx *gorm.DB, lottery *Lottery) error {
	var round LotteryRound
	err := tx.Where("lottery_id = ? AND status IN ?", lottery.Id, []string{LotteryRoundStatusPending, LotteryRoundStatusOpen, LotteryRoundStatusCancelled}).
		Order("draw_time asc").First(&round).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	hasRound := err == nil
	if lottery.Mode == LotteryModeOnce {
		roundValues := LotteryRound{
			LotteryId:         lottery.Id,
			RoundKey:          strconv.FormatInt(lottery.DrawTime, 10),
			Status:            LotteryRoundStatusPending,
			RegistrationStart: lottery.RegistrationStart,
			RegistrationEnd:   lottery.RegistrationEnd,
			DrawTime:          lottery.DrawTime,
		}
		if !hasRound {
			return tx.Create(&roundValues).Error
		}
		return tx.Model(&LotteryRound{}).Where("id = ?", round.Id).Updates(map[string]interface{}{
			"round_key":          roundValues.RoundKey,
			"status":             roundValues.Status,
			"registration_start": roundValues.RegistrationStart,
			"registration_end":   roundValues.RegistrationEnd,
			"draw_time":          roundValues.DrawTime,
			"drawn_at":           int64(0),
		}).Error
	}
	nextRound, err := nextScheduledLotteryRound(lottery, time.Now())
	if err != nil {
		return err
	}
	if !hasRound {
		return tx.Create(nextRound).Error
	}
	return tx.Model(&LotteryRound{}).Where("id = ?", round.Id).Updates(map[string]interface{}{
		"round_key":          nextRound.RoundKey,
		"status":             nextRound.Status,
		"registration_start": nextRound.RegistrationStart,
		"registration_end":   nextRound.RegistrationEnd,
		"draw_time":          nextRound.DrawTime,
		"drawn_at":           int64(0),
	}).Error
}

func nextScheduledLotteryRound(lottery *Lottery, from time.Time) (*LotteryRound, error) {
	if lottery == nil || lottery.Mode != LotteryModeScheduled {
		return nil, ErrLotteryInvalidSchedule
	}
	weekdays := lotteryWeekdaysFromString(lottery.ScheduleWeekdays)
	if len(weekdays) == 0 || !validLotteryClock(lottery.ScheduleStartTime) || !validLotteryClock(lottery.ScheduleEndTime) || !validLotteryClock(lottery.ScheduleDrawTime) {
		return nil, ErrLotteryInvalidSchedule
	}
	allowed := make(map[int]struct{}, len(weekdays))
	for _, day := range weekdays {
		allowed[day] = struct{}{}
	}
	loc := from.Location()
	for offset := 0; offset < 14; offset++ {
		day := from.AddDate(0, 0, offset)
		weekday := int(day.Weekday())
		if _, ok := allowed[weekday]; !ok {
			continue
		}
		start := lotteryTimeOnDay(day, lottery.ScheduleStartTime, loc)
		end := lotteryTimeOnDay(day, lottery.ScheduleEndTime, loc)
		draw := lotteryTimeOnDay(day, lottery.ScheduleDrawTime, loc)
		if draw.After(from) && end.After(from) {
			return &LotteryRound{
				LotteryId:         lottery.Id,
				RoundKey:          draw.Format("20060102"),
				Status:            LotteryRoundStatusPending,
				RegistrationStart: start.Unix(),
				RegistrationEnd:   end.Unix(),
				DrawTime:          draw.Unix(),
			}, nil
		}
	}
	return nil, ErrLotteryInvalidSchedule
}

func lotteryTimeOnDay(day time.Time, clock string, loc *time.Location) time.Time {
	hour, _ := strconv.Atoi(clock[:2])
	minute, _ := strconv.Atoi(clock[3:])
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
}

func EnsureScheduledLotteryRounds() error {
	var lotteries []Lottery
	if err := DB.Where("mode = ? AND status = ?", LotteryModeScheduled, LotteryStatusEnabled).Find(&lotteries).Error; err != nil {
		return err
	}
	now := time.Now()
	for i := range lotteries {
		lottery := lotteries[i]
		var count int64
		err := DB.Model(&LotteryRound{}).Where("lottery_id = ? AND status IN ?", lottery.Id, []string{LotteryRoundStatusPending, LotteryRoundStatusOpen, LotteryRoundStatusDrawing}).Count(&count).Error
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		round, err := nextScheduledLotteryRound(&lottery, now)
		if err != nil {
			common.SysLog("failed to create scheduled lottery round: " + err.Error())
			continue
		}
		var existing int64
		if err := DB.Model(&LotteryRound{}).Where("lottery_id = ? AND round_key = ?", lottery.Id, round.RoundKey).Count(&existing).Error; err != nil {
			return err
		}
		if existing == 0 {
			if err := DB.Create(round).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func RefreshLotteryRounds() error {
	now := common.GetTimestamp()
	if err := DB.Model(&LotteryRound{}).
		Where("status = ? AND registration_start <= ? AND registration_end > ?", LotteryRoundStatusPending, now, now).
		Update("status", LotteryRoundStatusOpen).Error; err != nil {
		return err
	}
	if err := DrawDueLotteryRounds(); err != nil {
		return err
	}
	return EnsureScheduledLotteryRounds()
}

func DrawDueLotteryRounds() error {
	now := common.GetTimestamp()
	var rounds []LotteryRound
	err := DB.Where("status IN ? AND draw_time <= ?", []string{LotteryRoundStatusPending, LotteryRoundStatusOpen}, now).Find(&rounds).Error
	if err != nil {
		return err
	}
	for i := range rounds {
		if err := DrawLotteryRound(rounds[i].Id, 0); err != nil {
			common.SysLog("lottery draw failed: " + err.Error())
		}
	}
	return nil
}

func DrawLotteryRound(roundId int, _ int) error {
	if roundId <= 0 {
		return ErrLotteryNotFound
	}
	now := common.GetTimestamp()
	type lotteryWinnerLog struct {
		userId    int
		content   string
		title     string
		prizeName string
	}
	var winnerLogs []lotteryWinnerLog
	err := DB.Transaction(func(tx *gorm.DB) error {
		var round LotteryRound
		if err := tx.Where("id = ?", roundId).First(&round).Error; err != nil {
			return ErrLotteryNotFound
		}
		if round.Status == LotteryRoundStatusFinished || round.Status == LotteryRoundStatusCancelled || round.Status == LotteryRoundStatusInsufficientPrizes {
			return nil
		}
		result := tx.Model(&LotteryRound{}).
			Where("id = ? AND status IN ?", round.Id, []string{LotteryRoundStatusPending, LotteryRoundStatusOpen}).
			Update("status", LotteryRoundStatusDrawing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		var lottery Lottery
		if err := tx.Where("id = ? AND status = ?", round.LotteryId, LotteryStatusEnabled).First(&lottery).Error; err != nil {
			return ErrLotteryNotFound
		}
		var entries []LotteryEntry
		if err := tx.Where("round_id = ?", round.Id).Find(&entries).Error; err != nil {
			return err
		}
		if len(entries) == 0 {
			round.Status = LotteryRoundStatusFinished
			round.DrawnAt = now
			return tx.Save(&round).Error
		}
		winnerCount := lottery.WinnerCount
		if winnerCount > len(entries) {
			winnerCount = len(entries)
		}
		requiredPrizes := winnerCount * lottery.PrizePerWinner
		var prizes []LotteryPrize
		if err := tx.Where("lottery_id = ? AND status = ?", lottery.Id, "available").
			Order("id asc").
			Limit(requiredPrizes).
			Find(&prizes).Error; err != nil {
			return err
		}
		if len(prizes) < requiredPrizes {
			round.Status = LotteryRoundStatusInsufficientPrizes
			round.DrawnAt = now
			return tx.Save(&round).Error
		}
		for i := len(entries) - 1; i > 0; i-- {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
			if err != nil {
				return err
			}
			j := int(n.Int64())
			entries[i], entries[j] = entries[j], entries[i]
		}
		winners := entries[:winnerCount]
		for i := range winners {
			entry := winners[i]
			entry.IsWinner = true
			entry.WonAt = now
			if err := tx.Model(&LotteryEntry{}).Where("id = ?", entry.Id).Updates(map[string]interface{}{
				"is_winner": true,
				"won_at":    now,
			}).Error; err != nil {
				return err
			}
			start := i * lottery.PrizePerWinner
			end := start + lottery.PrizePerWinner
			for _, prize := range prizes[start:end] {
				result := tx.Model(&LotteryPrize{}).Where("id = ? AND status = ?", prize.Id, "available").Updates(map[string]interface{}{
					"round_id":       round.Id,
					"winner_user_id": entry.UserId,
					"status":         "assigned",
					"assigned_at":    now,
				})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return ErrLotteryPrizeInsufficient
				}
			}
			winnerLogs = append(winnerLogs, lotteryWinnerLog{
				userId:    entry.UserId,
				content:   "抽奖中奖：" + lottery.Title + "，奖品：" + lottery.PrizeName,
				title:     lottery.Title,
				prizeName: lottery.PrizeName,
			})
		}
		round.Status = LotteryRoundStatusFinished
		round.DrawnAt = now
		return tx.Save(&round).Error
	})
	if err != nil {
		return err
	}
	for _, winnerLog := range winnerLogs {
		RecordLotteryWinLog(
			winnerLog.userId,
			winnerLog.content,
			winnerLog.title,
			winnerLog.prizeName,
		)
	}
	return nil
}

func JoinLotteryRound(lotteryId int, userId int) error {
	settings := LotterySettingsFromOptions()
	if !settings.Enabled {
		return ErrLotteryDisabled
	}
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		var lottery Lottery
		if err := tx.Where("id = ? AND status = ?", lotteryId, LotteryStatusEnabled).First(&lottery).Error; err != nil {
			return ErrLotteryNotFound
		}
		var round LotteryRound
		if err := tx.Where("lottery_id = ? AND registration_start <= ? AND registration_end > ? AND status IN ?", lottery.Id, now, now, []string{LotteryRoundStatusPending, LotteryRoundStatusOpen}).
			Order("registration_start asc").
			First(&round).Error; err != nil {
			return ErrLotteryNotOpen
		}
		if round.Status == LotteryRoundStatusPending {
			round.Status = LotteryRoundStatusOpen
			if err := tx.Save(&round).Error; err != nil {
				return err
			}
		}
		var existing int64
		if err := tx.Model(&LotteryEntry{}).Where("round_id = ? AND user_id = ?", round.Id, userId).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return ErrLotteryAlreadyJoined
		}
		user := User{}
		if err := tx.Select("id", "username", "display_name", "email", "created_at", "request_count").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		eligibility, err := EvaluateLotteryEligibility(tx, user, settings)
		if err != nil {
			return err
		}
		if !eligibility.Eligible {
			if len(eligibility.Issues) > 0 {
				return lotteryEligibilityIssueError(eligibility.Issues[0].Code)
			}
			return ErrLotteryRechargeRequired
		}
		entry := LotteryEntry{
			LotteryId:   lottery.Id,
			RoundId:     round.Id,
			UserId:      user.Id,
			Username:    user.Username,
			DisplayName: user.DisplayName,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrLotteryAlreadyJoined
		}
		return nil
	})
}

func lotteryEligibilityIssueError(code string) error {
	switch code {
	case "email_required":
		return ErrLotteryEmailRequired
	case "account_age_required":
		return ErrLotteryAccountAgeRequired
	case "request_count_required":
		return ErrLotteryRequestCountInvalid
	case "recharge_required":
		return ErrLotteryRechargeRequired
	default:
		return ErrLotteryRechargeRequired
	}
}

func EvaluateLotteryEligibility(tx *gorm.DB, user User, settings LotterySettings) (*LotteryEligibilityStatus, error) {
	status := &LotteryEligibilityStatus{
		Eligible: true,
		Issues:   make([]LotteryEligibilityIssue, 0, 4),
	}
	if settings.RequireEmailVerified && strings.TrimSpace(user.Email) == "" {
		status.Eligible = false
		status.Issues = append(status.Issues, LotteryEligibilityIssue{
			Code:    "email_required",
			Message: "需要绑定邮箱后才能参与抽奖",
		})
	}
	if settings.MinAccountAgeDays > 0 {
		minCreatedAt := common.GetTimestamp() - int64(settings.MinAccountAgeDays*86400)
		if user.CreatedAt > minCreatedAt {
			status.Eligible = false
			status.Issues = append(status.Issues, LotteryEligibilityIssue{
				Code:    "account_age_required",
				Message: "账号注册时间不满足参与条件",
			})
		}
	}
	if settings.MinRequestCount > 0 && user.RequestCount < settings.MinRequestCount {
		status.Eligible = false
		status.Issues = append(status.Issues, LotteryEligibilityIssue{
			Code:    "request_count_required",
			Message: "请求次数不满足参与条件",
		})
	}
	if settings.RequireRecharge || settings.MinRechargeAmount > 0 {
		rechargeEligibility, err := evaluateLotteryRechargeEligibility(tx, user.Id, settings)
		if err != nil {
			return nil, err
		}
		if !rechargeEligibility.Eligible {
			status.Eligible = false
			status.Issues = append(status.Issues, LotteryEligibilityIssue{
				Code:                      "recharge_required",
				Message:                   "充值条件不满足，暂时不能参与抽奖",
				RequiredAmount:            rechargeEligibility.RequiredAmount,
				CurrentAmount:             rechargeEligibility.CurrentAmount,
				RemainingAmount:           rechargeEligibility.RemainingAmount,
				WindowDays:                rechargeEligibility.WindowDays,
				CountRedemptionAsRecharge: rechargeEligibility.CountRedemptionAsRecharge,
			})
		}
	}
	return status, nil
}

func evaluateLotteryRechargeEligibility(tx *gorm.DB, userId int, settings LotterySettings) (lotteryRechargeEligibility, error) {
	result := lotteryRechargeEligibility{
		RequiredAmount:            settings.MinRechargeAmount,
		WindowDays:                settings.RechargeWindowDays,
		CountRedemptionAsRecharge: settings.CountRedemptionAsRecharge,
	}
	startTime := int64(0)
	if settings.RechargeWindowDays > 0 {
		startTime = common.GetTimestamp() - int64(settings.RechargeWindowDays*86400)
	}
	query := tx.Model(&TopUp{}).Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess)
	if startTime > 0 {
		query = query.Where("complete_time >= ?", startTime)
	}
	var topUps []TopUp
	if err := query.Find(&topUps).Error; err != nil {
		return result, err
	}
	for _, topUp := range topUps {
		result.RecordFound = true
		if topUp.Money > result.CurrentAmount {
			result.CurrentAmount = topUp.Money
		}
	}
	if lotteryRechargeAmountMeetsSettings(result.RecordFound, result.CurrentAmount, settings) {
		result.Eligible = true
		return result, nil
	}
	if !settings.CountRedemptionAsRecharge {
		result.RemainingAmount = lotteryRechargeRemainingAmount(result.RequiredAmount, result.CurrentAmount)
		return result, nil
	}
	var redemptions []Redemption
	redemptionQuery := tx.Where("used_user_id = ? AND status = ? AND redeem_type IN ?", userId, common.RedemptionCodeStatusUsed, []string{RedemptionTypeQuota, RedemptionTypeSubscription})
	if startTime > 0 {
		redemptionQuery = redemptionQuery.Where("redeemed_time >= ?", startTime)
	}
	if err := redemptionQuery.Find(&redemptions).Error; err != nil {
		return result, err
	}
	planPrices := map[int]float64{}
	planIds := make([]int, 0)
	for _, redemption := range redemptions {
		if redemption.RedeemType != RedemptionTypeSubscription || redemption.SubscriptionPlanId <= 0 {
			continue
		}
		if _, ok := planPrices[redemption.SubscriptionPlanId]; ok {
			continue
		}
		planPrices[redemption.SubscriptionPlanId] = 0
		planIds = append(planIds, redemption.SubscriptionPlanId)
	}
	if len(planIds) > 0 {
		var plans []SubscriptionPlan
		if err := tx.Select("id", "price_amount").Where("id IN ?", planIds).Find(&plans).Error; err != nil {
			return result, err
		}
		for _, plan := range plans {
			planPrices[plan.Id] = plan.PriceAmount
		}
	}
	for _, redemption := range redemptions {
		redeemMoney := 0.0
		if redemption.RedeemType == RedemptionTypeQuota {
			result.RecordFound = true
			redeemMoney = float64(redemption.Quota) / common.QuotaPerUnit
		} else if redemption.RedeemType == RedemptionTypeSubscription {
			if _, ok := planPrices[redemption.SubscriptionPlanId]; !ok {
				continue
			}
			result.RecordFound = true
			redeemMoney = planPrices[redemption.SubscriptionPlanId]
		}
		if redeemMoney > result.CurrentAmount {
			result.CurrentAmount = redeemMoney
		}
	}
	if lotteryRechargeAmountMeetsSettings(result.RecordFound, result.CurrentAmount, settings) {
		result.Eligible = true
		return result, nil
	}
	result.RemainingAmount = lotteryRechargeRemainingAmount(result.RequiredAmount, result.CurrentAmount)
	return result, nil
}

func lotteryRechargeAmountMeetsSettings(recordFound bool, amount float64, settings LotterySettings) bool {
	if !recordFound {
		return false
	}
	if settings.MinRechargeAmount <= 0 {
		return true
	}
	return amount >= settings.MinRechargeAmount
}

func lotteryRechargeRemainingAmount(requiredAmount float64, currentAmount float64) float64 {
	if requiredAmount <= currentAmount {
		return 0
	}
	return requiredAmount - currentAmount
}

func GetPublicLotteries(userId int, filter LotteryListFilter) ([]LotteryPublicView, error) {
	if err := RefreshLotteryRounds(); err != nil {
		return nil, err
	}
	var lotteries []Lottery
	if err := DB.Where("status = ?", LotteryStatusEnabled).Find(&lotteries).Error; err != nil {
		return nil, err
	}
	views := make([]LotteryPublicView, 0, len(lotteries))
	for i := range lotteries {
		view, err := buildLotteryPublicView(DB, &lotteries[i], userId, true)
		if err != nil {
			return nil, err
		}
		if view.Round != nil && lotteryViewMatchesDrawStatus(view, filter.DrawStatus) {
			views = append(views, view)
		}
	}
	sortLotteryPublicViews(views)
	return views, nil
}

func GetPublicLottery(lotteryId int, userId int) (*LotteryPublicView, error) {
	if err := RefreshLotteryRounds(); err != nil {
		return nil, err
	}
	var lottery Lottery
	if err := DB.Where("id = ? AND status = ?", lotteryId, LotteryStatusEnabled).First(&lottery).Error; err != nil {
		return nil, ErrLotteryNotFound
	}
	view, err := buildLotteryPublicView(DB, &lottery, userId, true)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func buildLotteryPublicView(tx *gorm.DB, lottery *Lottery, userId int, includeWinners bool) (LotteryPublicView, error) {
	view := LotteryPublicView{
		Id:                lottery.Id,
		Title:             lottery.Title,
		Description:       lottery.Description,
		PrizeName:         lottery.PrizeName,
		Mode:              lottery.Mode,
		Status:            lottery.Status,
		WinnerCount:       lottery.WinnerCount,
		PrizePerWinner:    lottery.PrizePerWinner,
		ScheduleWeekdays:  lotteryWeekdaysFromString(lottery.ScheduleWeekdays),
		ScheduleStartTime: lottery.ScheduleStartTime,
		ScheduleEndTime:   lottery.ScheduleEndTime,
		ScheduleDrawTime:  lottery.ScheduleDrawTime,
		CreatedAt:         lottery.CreatedAt,
		Deleted:           isLotteryDeletedStatus(lottery.Status),
	}
	var round LotteryRound
	err := tx.Where("lottery_id = ? AND status IN ?", lottery.Id, []string{LotteryRoundStatusPending, LotteryRoundStatusOpen, LotteryRoundStatusDrawing}).
		Order("draw_time asc").First(&round).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = tx.Where("lottery_id = ? AND status IN ?", lottery.Id, []string{LotteryRoundStatusFinished, LotteryRoundStatusInsufficientPrizes}).
			Order("draw_time desc").First(&round).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return view, nil
		}
		return view, err
	}
	view.Round = &round
	var entries []LotteryEntry
	if err := tx.Where("round_id = ?", round.Id).Order("id desc").Limit(100).Find(&entries).Error; err != nil {
		return view, err
	}
	if err := tx.Model(&LotteryEntry{}).Where("round_id = ?", round.Id).Count(&view.ParticipantCount).Error; err != nil {
		return view, err
	}
	view.Participants = make([]LotteryParticipantView, 0, len(entries))
	for _, entry := range entries {
		view.Participants = append(view.Participants, LotteryParticipantView{
			Id:         entry.Id,
			MaskedName: maskLotteryName(displayLotteryName(entry.Username, entry.DisplayName)),
			JoinedAt:   entry.CreatedAt,
			IsWinner:   entry.IsWinner,
		})
		if entry.UserId == userId {
			view.Joined = true
		}
	}
	if !view.Joined && userId > 0 {
		var count int64
		if err := tx.Model(&LotteryEntry{}).Where("round_id = ? AND user_id = ?", round.Id, userId).Count(&count).Error; err != nil {
			return view, err
		}
		view.Joined = count > 0
	}
	if userId > 0 {
		var winnerCount int64
		if err := tx.Model(&LotteryEntry{}).Where("round_id = ? AND user_id = ? AND is_winner = ?", round.Id, userId, true).Count(&winnerCount).Error; err != nil {
			return view, err
		}
		view.Won = winnerCount > 0
		var user User
		if err := tx.Select("id", "email", "created_at", "request_count").Where("id = ?", userId).First(&user).Error; err != nil {
			return view, err
		}
		eligibility, err := EvaluateLotteryEligibility(tx, user, LotterySettingsFromOptions())
		if err != nil {
			return view, err
		}
		view.Eligibility = eligibility
	}
	if includeWinners && round.Status == LotteryRoundStatusFinished {
		winners, err := getLotteryWinners(tx, round.Id, false)
		if err != nil {
			return view, err
		}
		view.Winners = winners
	}
	return view, nil
}

func getLotteryWinners(tx *gorm.DB, roundId int, includeCodes bool) ([]LotteryWinnerView, error) {
	var entries []LotteryEntry
	if err := tx.Where("round_id = ? AND is_winner = ?", roundId, true).Order("won_at asc").Find(&entries).Error; err != nil {
		return nil, err
	}
	winners := make([]LotteryWinnerView, 0, len(entries))
	for _, entry := range entries {
		winner := LotteryWinnerView{
			MaskedName: maskLotteryName(displayLotteryName(entry.Username, entry.DisplayName)),
			WonAt:      entry.WonAt,
		}
		if includeCodes {
			winner.UserId = entry.UserId
			winner.Username = entry.Username
			var prizes []LotteryPrize
			if err := tx.Where("round_id = ? AND winner_user_id = ?", roundId, entry.UserId).Order("id asc").Find(&prizes).Error; err != nil {
				return nil, err
			}
			for _, prize := range prizes {
				winner.Prizes = append(winner.Prizes, prize.Code)
			}
		}
		winners = append(winners, winner)
	}
	return winners, nil
}

func lotteryViewMatchesDrawStatus(view LotteryPublicView, drawStatus string) bool {
	status := strings.ToLower(strings.TrimSpace(drawStatus))
	if status == "" || status == "all" || view.Round == nil {
		return true
	}
	if status == "drawn" {
		return lotteryRoundIsDrawn(view.Round.Status)
	}
	if status == "undrawn" {
		return lotteryRoundIsUndrawn(view.Round.Status)
	}
	return true
}

func lotteryRoundIsDrawn(status string) bool {
	return status == LotteryRoundStatusFinished || status == LotteryRoundStatusInsufficientPrizes
}

func lotteryRoundIsUndrawn(status string) bool {
	return status == LotteryRoundStatusPending || status == LotteryRoundStatusOpen || status == LotteryRoundStatusDrawing
}

func sortLotteryPublicViews(views []LotteryPublicView) {
	now := common.GetTimestamp()
	sort.SliceStable(views, func(i, j int) bool {
		left := views[i].CreatedAt
		right := views[j].CreatedAt
		if views[i].Round != nil {
			left = views[i].Round.DrawTime
		}
		if views[j].Round != nil {
			right = views[j].Round.DrawTime
		}
		leftDistance := left - now
		if leftDistance < 0 {
			leftDistance = -leftDistance
		}
		rightDistance := right - now
		if rightDistance < 0 {
			rightDistance = -rightDistance
		}
		if leftDistance == rightDistance {
			if left == right {
				return views[i].Id > views[j].Id
			}
			return left > right
		}
		return leftDistance < rightDistance
	})
}

func displayLotteryName(username string, displayName string) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = strings.TrimSpace(username)
	}
	if name == "" {
		return "用户"
	}
	return name
}

func maskLotteryName(name string) string {
	runes := []rune(name)
	if len(runes) <= 1 {
		return "*"
	}
	if len(runes) == 2 {
		return string(runes[:1]) + "*"
	}
	return string(runes[:1]) + strings.Repeat("*", len(runes)-2) + string(runes[len(runes)-1:])
}

func ListAdminLotteries(pageInfo *common.PageInfo, filter LotteryListFilter) ([]LotteryPublicView, int64, error) {
	if err := RefreshLotteryRounds(); err != nil {
		return nil, 0, err
	}
	query := DB.Model(&Lottery{})
	if filter.Mode != "" {
		query = query.Where("mode = ?", filter.Mode)
	}
	if filter.Status != "" {
		status, err := strconv.Atoi(filter.Status)
		if err == nil {
			query = query.Where("status = ?", status)
		}
	}
	if strings.TrimSpace(filter.Query) != "" {
		pattern := "%" + strings.TrimSpace(filter.Query) + "%"
		query = query.Where("title LIKE ? OR prize_name LIKE ?", pattern, pattern)
	}
	var lotteries []Lottery
	if err := query.Find(&lotteries).Error; err != nil {
		return nil, 0, err
	}
	views := make([]LotteryPublicView, 0, len(lotteries))
	for i := range lotteries {
		view, err := buildLotteryPublicView(DB, &lotteries[i], 0, true)
		if err != nil {
			return nil, 0, err
		}
		if err := attachAdminLotteryRounds(&view, 10); err != nil {
			return nil, 0, err
		}
		if editable, err := isLotteryEditable(DB, view.Id); err != nil {
			return nil, 0, err
		} else {
			view.CanEdit = editable
		}
		if lotteryViewMatchesDrawStatus(view, filter.DrawStatus) {
			views = append(views, view)
		}
	}
	sortLotteryPublicViews(views)
	total := int64(len(views))
	start := pageInfo.GetStartIdx()
	if start >= len(views) {
		return []LotteryPublicView{}, total, nil
	}
	end := pageInfo.GetEndIdx()
	if end > len(views) {
		end = len(views)
	}
	return views[start:end], total, nil
}

func GetAdminLottery(lotteryId int) (*LotteryPublicView, error) {
	var lottery Lottery
	if err := DB.Where("id = ?", lotteryId).First(&lottery).Error; err != nil {
		return nil, ErrLotteryNotFound
	}
	view, err := buildLotteryPublicView(DB, &lottery, 0, true)
	if err != nil {
		return nil, err
	}
	if view.Round != nil {
		winners, err := getLotteryWinners(DB, view.Round.Id, true)
		if err != nil {
			return nil, err
		}
		view.Winners = winners
	}
	if err := attachAdminLotteryRounds(&view, 50); err != nil {
		return nil, err
	}
	if editable, err := isLotteryEditable(DB, view.Id); err != nil {
		return nil, err
	} else {
		view.CanEdit = editable
	}
	return &view, nil
}

func attachAdminLotteryRounds(view *LotteryPublicView, limit int) error {
	if view == nil || view.Id <= 0 {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	var rounds []LotteryRound
	if err := DB.Where("lottery_id = ?", view.Id).Order("draw_time desc").Limit(limit).Find(&rounds).Error; err != nil {
		return err
	}
	view.Rounds = make([]LotteryRoundDetailView, 0, len(rounds))
	if err := DB.Model(&LotteryPrize{}).Where("lottery_id = ? AND status = ?", view.Id, "available").Count(&view.AvailablePrizeCount).Error; err != nil {
		return err
	}
	if err := DB.Model(&LotteryPrize{}).
		Where("lottery_id = ? AND status = ?", view.Id, "available").
		Order("id asc").
		Pluck("code", &view.PrizeCodes).Error; err != nil {
		return err
	}
	for i := range rounds {
		detail := LotteryRoundDetailView{
			Round: rounds[i],
		}
		if err := DB.Model(&LotteryEntry{}).Where("round_id = ?", rounds[i].Id).Count(&detail.ParticipantCount).Error; err != nil {
			return err
		}
		if rounds[i].Status == LotteryRoundStatusFinished {
			winners, err := getLotteryWinners(DB, rounds[i].Id, true)
			if err != nil {
				return err
			}
			detail.Winners = winners
		}
		view.Rounds = append(view.Rounds, detail)
	}
	if view.Round != nil {
		for i := range view.Rounds {
			if view.Rounds[i].Round.Id == view.Round.Id {
				view.Winners = view.Rounds[i].Winners
				break
			}
		}
	}
	return nil
}

func UpdateLotteryStatus(lotteryId int, status int) error {
	if status != LotteryStatusEnabled && status != LotteryStatusDisabled && status != LotteryStatusDeleted {
		return errors.New("抽奖状态无效")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var lottery Lottery
		if err := tx.Where("id = ?", lotteryId).First(&lottery).Error; err != nil {
			return ErrLotteryNotFound
		}
		if isLotteryDeletedStatus(lottery.Status) && status != LotteryStatusDeleted {
			return ErrLotteryDeleted
		}
		if status == LotteryStatusDisabled {
			if err := tx.Model(&LotteryRound{}).
				Where("lottery_id = ? AND status IN ?", lottery.Id, []string{LotteryRoundStatusPending, LotteryRoundStatusOpen, LotteryRoundStatusDrawing}).
				Update("status", LotteryRoundStatusCancelled).Error; err != nil {
				return err
			}
		}
		if status == LotteryStatusDeleted {
			if err := tx.Model(&LotteryRound{}).
				Where("lottery_id = ? AND status IN ?", lottery.Id, []string{LotteryRoundStatusPending, LotteryRoundStatusOpen, LotteryRoundStatusDrawing}).
				Update("status", LotteryRoundStatusCancelled).Error; err != nil {
				return err
			}
			if err := tx.Model(&LotteryPrize{}).Where("lottery_id = ?", lottery.Id).Update("status", "deleted").Error; err != nil {
				return err
			}
		}
		return tx.Model(&Lottery{}).Where("id = ?", lottery.Id).Update("status", status).Error
	})
}

func DeleteLottery(lotteryId int) error {
	return UpdateLotteryStatus(lotteryId, LotteryStatusDeleted)
}

func GetUserLotteryPrizes(userId int, pageInfo *common.PageInfo) ([]LotteryPrizeView, int64, error) {
	query := DB.Table("lottery_prizes").
		Select("lottery_prizes.id, lottery_prizes.lottery_id, lottery_prizes.round_id, lotteries.title, lottery_prizes.prize_name, lottery_prizes.code, lottery_prizes.assigned_at AS won_at, lottery_rounds.draw_time").
		Joins("JOIN lotteries ON lotteries.id = lottery_prizes.lottery_id").
		Joins("JOIN lottery_rounds ON lottery_rounds.id = lottery_prizes.round_id").
		Where("lottery_prizes.winner_user_id = ? AND lottery_prizes.status = ?", userId, "assigned")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var prizes []LotteryPrizeView
	if err := query.Order("lottery_prizes.assigned_at desc, lottery_prizes.id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Scan(&prizes).Error; err != nil {
		return nil, 0, err
	}
	return prizes, total, nil
}

func StartLotteryDrawTask() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := RefreshLotteryRounds(); err != nil {
				common.SysLog("lottery refresh failed: " + err.Error())
			}
		}
	}()
}
