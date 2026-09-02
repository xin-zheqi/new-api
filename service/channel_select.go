package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
)

func GetChannelConstraints(c *gin.Context) *dto.ChannelConstraints {
	if c == nil {
		return &dto.ChannelConstraints{}
	}
	if existing, ok := common.GetContextKeyType[*dto.ChannelConstraints](c, constant.ContextKeyChannelConstraints); ok && existing != nil {
		return existing
	}
	constraints := &dto.ChannelConstraints{}
	common.SetContextKey(c, constant.ContextKeyChannelConstraints, constraints)
	return constraints
}

func AppendTaskPluginIdentityFilter(c *gin.Context, pluginKey string) {
	if c == nil {
		return
	}
	GetChannelConstraints(c).AddFilter(dto.ChannelFilter{
		Kind:                   dto.FilterTaskPluginIdentity,
		TaskPluginKey:          pluginKey,
		TaskPluginChannelTypes: pinnedTaskPluginChannelTypes(c, pluginKey),
	})
}

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

func getChannelFromOrderedGroups(param *RetryParam, routeGroups []string, filters []dto.ChannelFilter) (*model.Channel, string, error) {
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

		channel, err = model.GetRandomSatisfiedChannel(group, param.ModelName, priorityRetry, filters)
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
			common.SetContextKey(param.Ctx, constant.ContextKeyCrossGroupExhausted, !hasLaterCandidate(routeGroups, i+1, param.ModelName, filters))
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

func hasLaterCandidate(routeGroups []string, startIndex int, modelName string, filters []dto.ChannelFilter) bool {
	if startIndex >= len(routeGroups) {
		return false
	}
	for i := startIndex; i < len(routeGroups); i++ {
		channel, err := model.GetRandomSatisfiedChannel(routeGroups[i], modelName, 0, filters)
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
	filters := append([]dto.ChannelFilter(nil), GetChannelConstraints(param.Ctx).Filters...)
	if param.RequestPath != "" {
		hasRequestPathFilter := false
		for _, filter := range filters {
			if filter.Kind == dto.FilterRequestPath && filter.RequestPath == param.RequestPath {
				hasRequestPathFilter = true
				break
			}
		}
		if !hasRequestPathFilter {
			filters = append(filters, dto.ChannelFilter{Kind: dto.FilterRequestPath, RequestPath: param.RequestPath})
		}
	}
	tokenGroups := model.NormalizeTokenGroupList(param.TokenGroup)

	if len(tokenGroups) == 1 && tokenGroups[0] == "auto" {
		autoGroups := GetRequestAutoGroups(param.Ctx, userGroup)
		if len(autoGroups) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		channel, selectGroup, err = getChannelFromOrderedGroups(param, autoGroups, filters)
		if err != nil {
			return nil, selectGroup, err
		}
	} else if len(tokenGroups) > 1 {
		channel, selectGroup, err = getChannelFromOrderedGroups(param, tokenGroups, filters)
		if err != nil {
			return nil, selectGroup, err
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannel(
			param.TokenGroup,
			param.ModelName,
			param.GetRetry(),
			filters,
		)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}

func pinnedTaskPluginChannelTypes(c *gin.Context, expected string) []int {
	if c == nil || expected == "" {
		return nil
	}
	if value, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint); exists {
		pinned, ok := value.(jsplugin.PinnedEndpoint)
		if ok && pinned.Generation != nil && len(pinned.Candidates) > 1 {
			expectedFound := false
			channelTypes := make([]int, 0, len(pinned.Candidates))
			seen := make(map[int]struct{}, len(pinned.Candidates))
			for _, candidate := range pinned.Candidates {
				if candidate.Plugin == nil {
					continue
				}
				if candidate.Plugin.Meta.Key == expected {
					expectedFound = true
				}
				for _, channelType := range candidate.Plugin.Meta.ChannelTypes {
					if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
						continue
					}
					if _, duplicate := seen[channelType]; duplicate {
						continue
					}
					if plugin, indexed := pinned.Generation.GetByChannelType(channelType); indexed && plugin == candidate.Plugin {
						seen[channelType] = struct{}{}
						channelTypes = append(channelTypes, channelType)
					}
				}
			}
			if expectedFound {
				return channelTypes
			}
		}
	}
	value, exists := c.Get(jsplugin.ContextKeyPinnedPlugin)
	pinned, ok := value.(jsplugin.PinnedPlugin)
	if !exists || !ok || pinned.Generation == nil || pinned.Plugin == nil || pinned.Plugin.Meta.Key != expected {
		return nil
	}
	channelTypes := make([]int, 0, len(pinned.Plugin.Meta.ChannelTypes))
	for _, channelType := range pinned.Plugin.Meta.ChannelTypes {
		if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
			continue
		}
		channelTypes = append(channelTypes, channelType)
	}
	if len(channelTypes) == 0 {
		return nil
	}
	return channelTypes
}
