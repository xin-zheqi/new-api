package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	RequestPath  string
	Retry        *int
	resetNextTry bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func getCurrentGroupIndex(ctx *gin.Context) int {
	startGroupIndex := 0
	if lastGroupIndex, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := lastGroupIndex.(int); ok && idx > 0 {
			startGroupIndex = idx
		}
	}
	return startGroupIndex
}

func getCrossGroupPrimaryIndex(ctx *gin.Context) (int, bool) {
	if primaryIndex, exists := common.GetContextKey(ctx, constant.ContextKeyCrossGroupPrimaryIndex); exists {
		if idx, ok := primaryIndex.(int); ok {
			return idx, true
		}
	}
	return 0, false
}

func getChannelFromOrderedGroups(param *RetryParam, routeGroups []string) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	startGroupIndex := getCurrentGroupIndex(param.Ctx)
	crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
	common.SetContextKey(param.Ctx, constant.ContextKeyCrossGroupExhausted, startGroupIndex >= len(routeGroups))

	for i := startGroupIndex; i < len(routeGroups); i++ {
		group := routeGroups[i]
		priorityRetry := param.GetRetry()
		if primaryIndex, ok := getCrossGroupPrimaryIndex(param.Ctx); ok && i != primaryIndex {
			priorityRetry = 0
		} else if crossGroupRetry && i > startGroupIndex {
			priorityRetry = 0
		}
		logger.LogDebug(param.Ctx, "Selecting group: %s, priorityRetry: %d", group, priorityRetry)

		channel, err = model.GetRandomSatisfiedChannel(group, param.ModelName, priorityRetry, param.RequestPath)
		if err != nil {
			return nil, group, err
		}
		if channel == nil {
			logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", group, param.ModelName, priorityRetry)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			if crossGroupRetry {
				param.SetRetry(0)
			}
			continue
		}

		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, group)
		selectGroup = group
		primaryIndex, primaryExists := getCrossGroupPrimaryIndex(param.Ctx)
		if !primaryExists {
			common.SetContextKey(param.Ctx, constant.ContextKeyCrossGroupPrimaryIndex, i)
			primaryIndex = i
		}

		shouldAdvance := false
		if crossGroupRetry {
			if i == primaryIndex {
				shouldAdvance = param.GetRetry() >= common.RetryTimes
			} else {
				shouldAdvance = true
			}
		}

		if shouldAdvance {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			common.SetContextKey(param.Ctx, constant.ContextKeyCrossGroupExhausted, !hasLaterCandidate(routeGroups, i+1, param.ModelName, param.RequestPath))
			param.SetRetry(0)
			param.ResetRetryNextTry()
		} else {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			common.SetContextKey(param.Ctx, constant.ContextKeyCrossGroupExhausted, false)
		}
		return channel, selectGroup, nil
	}
	common.SetContextKey(param.Ctx, constant.ContextKeyCrossGroupExhausted, true)
	return nil, selectGroup, nil
}

func hasLaterCandidate(routeGroups []string, startIndex int, modelName string, requestPath string) bool {
	if startIndex >= len(routeGroups) {
		return false
	}
	for i := startIndex; i < len(routeGroups); i++ {
		channel, err := model.GetRandomSatisfiedChannel(routeGroups[i], modelName, 0, requestPath)
		if err == nil && channel != nil {
			return true
		}
	}
	return false
}

// CacheGetRandomSatisfiedChannel selects a channel for single, auto, or ordered multi-group tokens.
// For auto and multi-group tokens, groups are checked in order and the selected concrete
// group is stored in ContextKeyAutoGroup for downstream pricing and logging.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if TokenGroupIsAuto(param.TokenGroup) {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetTokenRouteGroups(param.TokenGroup, userGroup)
		channel, selectGroup, err = getChannelFromOrderedGroups(param, autoGroups)
		if err != nil {
			return nil, selectGroup, err
		}
	} else if TokenGroupIsMulti(param.TokenGroup) {
		routeGroups := GetTokenRouteGroups(param.TokenGroup, userGroup)
		channel, selectGroup, err = getChannelFromOrderedGroups(param, routeGroups)
		if err != nil {
			return nil, selectGroup, err
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry(), param.RequestPath)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}
