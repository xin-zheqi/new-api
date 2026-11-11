package model

import (
	"slices"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
)

var filterEvalOrder = []dto.ChannelFilterKind{
	dto.FilterRequestPath,
	dto.FilterTaskPluginIdentity,
	dto.FilterResponsesWebSocket,
}

// ChannelSatisfiesFilters reports whether ch passes every filter.
// On false, it returns the kind of the first violated filter (request_path
// then task_plugin_identity) for error attribution.
func ChannelSatisfiesFilters(ch *Channel, modelName string, filters []dto.ChannelFilter) (bool, dto.ChannelFilterKind) {
	if ch == nil {
		return false, ""
	}
	if !ch.IsScheduleActiveAt(time.Now()) {
		return false, ""
	}
	for _, kind := range filterEvalOrder {
		for _, filter := range filters {
			if filter.Kind != kind {
				continue
			}
			if !channelMatchesFilter(ch, modelName, filter, nil) {
				return false, kind
			}
		}
	}
	return true, ""
}

// filterCandidateIDs applies filters to a cached candidate id list.
// Caller must hold channelSyncLock (read lock). The input slice is never mutated.
// A missing id in channelsIDM is kept for request_path (downstream consistency
// error) and dropped for task_plugin_identity, matching the previous filters.
func filterCandidateIDs(ids []int, modelName string, filters []dto.ChannelFilter) (kept []int, emptiedBy dto.ChannelFilterKind) {
	if len(ids) == 0 {
		return ids, ""
	}
	kept = ids
	now := time.Now()
	// Retain the cached slice on the common no-rejection path. Schedule
	// evaluation uses precompiled local-time periods and no database access.
	for i, id := range ids {
		if channel, exists := channelsIDM[id]; exists && !channel.IsScheduleActiveAt(now) {
			kept = make([]int, i, len(ids)-1)
			copy(kept, ids[:i])
			for _, remaining := range ids[i+1:] {
				candidate, found := channelsIDM[remaining]
				if !found || candidate.IsScheduleActiveAt(now) {
					kept = append(kept, remaining)
				}
			}
			break
		}
	}
	for _, kind := range filterEvalOrder {
		kindFilters := filtersByKind(filters, kind)
		if len(kindFilters) == 0 {
			continue
		}
		// Most ordinary relay requests keep every candidate. Allocate a copy
		// only on the first rejection, never modifying the shared cache slice.
		next := kept
		copied := false
		for i, id := range kept {
			channel, exists := channelsIDM[id]
			if !candidatePassesKindFilters(channel, exists, modelName, kind, kindFilters) {
				if !copied {
					next = make([]int, i, len(kept)-1)
					copy(next, kept[:i])
					copied = true
				}
			} else if copied {
				next = append(next, id)
			}
		}
		if len(kept) > 0 && len(next) == 0 {
			return next, kind
		}
		kept = next
	}
	return kept, ""
}

func filtersByKind(filters []dto.ChannelFilter, kind dto.ChannelFilterKind) []dto.ChannelFilter {
	var matched []dto.ChannelFilter
	for _, filter := range filters {
		if filter.Kind == kind {
			matched = append(matched, filter)
		}
	}
	return matched
}

func candidatePassesKindFilters(ch *Channel, exists bool, modelName string, kind dto.ChannelFilterKind, filters []dto.ChannelFilter) bool {
	if kind == dto.FilterRequestPath && !exists {
		return true
	}
	if !exists || ch == nil {
		return false
	}
	for _, filter := range filters {
		// Full sync and CacheUpdateChannel publish this parsed config under
		// channelSyncLock. Reuse it while the caller holds the read lock.
		if !channelMatchesFilter(ch, modelName, filter, channel2advancedCustomConfig[ch.Id]) {
			return false
		}
	}
	return true
}

func channelMatchesFilter(ch *Channel, modelName string, filter dto.ChannelFilter, advancedConfig *kitdto.AdvancedCustomConfig) bool {
	switch filter.Kind {
	case dto.FilterRequestPath:
		if filter.RequestPath == "" {
			return true
		}
		if ch.Type != constant.ChannelTypeAdvancedCustom {
			return true
		}
		if advancedConfig == nil {
			advancedConfig = ch.GetOtherSettings().AdvancedCustom
		}
		return advancedConfig != nil && advancedConfig.SupportsPathForModel(filter.RequestPath, modelName)
	case dto.FilterTaskPluginIdentity:
		if ch.Type == constant.ChannelTypeTaskPlugin {
			return filter.TaskPluginKey != "" && ch.GetSetting().TaskPluginKey == filter.TaskPluginKey
		}
		return filter.TaskPluginKey == "" || slices.Contains(filter.TaskPluginChannelTypes, ch.Type)
	case dto.FilterResponsesWebSocket:
		if !ch.GetSetting().ResponsesWebSocketEnabled {
			return false
		}
		switch ch.Type {
		case constant.ChannelTypeOpenAI, constant.ChannelTypeCodex, constant.ChannelTypeSub2API, constant.ChannelTypeNewAPI:
			return true
		case constant.ChannelTypeAdvancedCustom:
			// The session forwards native Responses events without protocol
			// conversion, so only a converter-free /v1/responses route qualifies.
			if advancedConfig == nil {
				advancedConfig = ch.GetOtherSettings().AdvancedCustom
			}
			route, ok := advancedConfig.MatchPathForModel("/v1/responses", modelName)
			return ok && route.IsNative()
		default:
			return false
		}
	default:
		return true
	}
}
