package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type lotteryStatusRequest struct {
	Status int `json:"status"`
}

func GetLotterySettings(c *gin.Context) {
	settings, err := model.LotterySettingsForUser(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, settings)
}

func GetLotteries(c *gin.Context) {
	settings := model.LotterySettingsFromOptions()
	if !settings.Enabled {
		handleLotteryError(c, model.ErrLotteryDisabled)
		return
	}
	userId := c.GetInt("id")
	lotteries, err := model.GetPublicLotteries(userId, model.LotteryListFilter{
		DrawStatus: strings.TrimSpace(c.Query("draw_status")),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, lotteries)
}

func GetLottery(c *gin.Context) {
	settings := model.LotterySettingsFromOptions()
	if !settings.Enabled {
		common.ApiError(c, model.ErrLotteryDisabled)
		return
	}
	lotteryId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	lottery, err := model.GetPublicLottery(lotteryId, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, lottery)
}

func JoinLottery(c *gin.Context) {
	lotteryId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	err = model.JoinLotteryRound(lotteryId, c.GetInt("id"))
	if err != nil {
		handleLotteryError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetMyLotteryPrizes(c *gin.Context) {
	settings := model.LotterySettingsFromOptions()
	if !settings.Enabled {
		handleLotteryError(c, model.ErrLotteryDisabled)
		return
	}
	pageInfo := common.GetPageQuery(c)
	prizes, total, err := model.GetUserLotteryPrizes(c.GetInt("id"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(prizes)
	common.ApiSuccess(c, pageInfo)
}

func AdminListLotteries(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	lotteries, total, err := model.ListAdminLotteries(pageInfo, model.LotteryListFilter{
		Status:     strings.TrimSpace(c.Query("status")),
		Mode:       strings.TrimSpace(c.Query("mode")),
		Query:      strings.TrimSpace(c.Query("keyword")),
		DrawStatus: strings.TrimSpace(c.Query("draw_status")),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(lotteries)
	common.ApiSuccess(c, pageInfo)
}

func AdminGetLottery(c *gin.Context) {
	lotteryId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	lottery, err := model.GetAdminLottery(lotteryId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, lottery)
}

func AdminListLotteryRounds(c *gin.Context) {
	lotteryId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo := common.GetPageQuery(c)
	rounds, total, err := model.ListAdminLotteryRounds(lotteryId, pageInfo, model.LotteryRoundListFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Query:  strings.TrimSpace(c.Query("keyword")),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rounds)
	common.ApiSuccess(c, pageInfo)
}

func AdminCreateLottery(c *gin.Context) {
	var req model.LotteryCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	lottery, err := model.CreateLottery(req, c.GetInt("id"))
	if err != nil {
		handleLotteryError(c, err)
		return
	}
	recordManageAudit(c, "lottery.create", map[string]interface{}{
		"id":           lottery.Id,
		"title":        lottery.Title,
		"mode":         lottery.Mode,
		"winner_count": lottery.WinnerCount,
	})
	view, err := model.GetAdminLottery(lottery.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func AdminUpdateLottery(c *gin.Context) {
	lotteryId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req model.LotteryCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	lottery, err := model.UpdateLottery(lotteryId, req)
	if err != nil {
		handleLotteryError(c, err)
		return
	}
	recordManageAudit(c, "lottery.update", map[string]interface{}{
		"id":           lottery.Id,
		"title":        lottery.Title,
		"mode":         lottery.Mode,
		"winner_count": lottery.WinnerCount,
	})
	view, err := model.GetAdminLottery(lottery.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func AdminUpdateLotteryStatus(c *gin.Context) {
	lotteryId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req lotteryStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateLotteryStatus(lotteryId, req.Status); err != nil {
		handleLotteryError(c, err)
		return
	}
	recordManageAudit(c, "lottery.status", map[string]interface{}{
		"id":     lotteryId,
		"status": req.Status,
	})
	common.ApiSuccess(c, nil)
}

func AdminDeleteLottery(c *gin.Context) {
	lotteryId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteLottery(lotteryId); err != nil {
		handleLotteryError(c, err)
		return
	}
	recordManageAudit(c, "lottery.delete", map[string]interface{}{
		"id": lotteryId,
	})
	common.ApiSuccess(c, nil)
}

func AdminDrawLottery(c *gin.Context) {
	roundId, err := strconv.Atoi(c.Param("round_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DrawLotteryRound(roundId, c.GetInt("id")); err != nil {
		handleLotteryError(c, err)
		return
	}
	recordManageAudit(c, "lottery.draw", map[string]interface{}{
		"round_id": roundId,
	})
	common.ApiSuccess(c, nil)
}

func handleLotteryError(c *gin.Context, err error) {
	status := http.StatusOK
	if errors.Is(err, model.ErrLotteryDisabled) || errors.Is(err, model.ErrLotteryRechargeRequired) ||
		errors.Is(err, model.ErrLotteryAccountAgeRequired) || errors.Is(err, model.ErrLotteryRequestCountInvalid) ||
		errors.Is(err, model.ErrLotteryEmailRequired) {
		status = http.StatusForbidden
	}
	c.JSON(status, gin.H{
		"success": false,
		"message": err.Error(),
	})
}
