package helper

import (
	"errors"
	"fmt"

	commonroot "github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

const modelMappingCacheContextKey = "_model_mapping_cache"

type modelMappingCache struct {
	raw    string
	values map[string]string
}

func getRequestModelMapping(c *gin.Context, raw string) (map[string]string, error) {
	if cached, ok := c.Get(modelMappingCacheContextKey); ok {
		if mappingCache, ok := cached.(modelMappingCache); ok && mappingCache.raw == raw {
			return mappingCache.values, nil
		}
	}
	modelMap := make(map[string]string)
	if err := commonroot.Unmarshal([]byte(raw), &modelMap); err != nil {
		return nil, err
	}
	c.Set(modelMappingCacheContextKey, modelMappingCache{
		raw:    raw,
		values: modelMap,
	})
	return modelMap, nil
}

func ModelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}

	// map model name
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" && modelMapping != "{}" {
		modelMap, err := getRequestModelMapping(c, modelMapping)
		if err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}

		// 支持链式模型重定向，最终使用链尾的模型
		currentModel := info.OriginModelName
		visitedModels := map[string]bool{
			currentModel: true,
		}
		for {
			if mappedModel, exists := modelMap[currentModel]; exists && mappedModel != "" {
				// 模型重定向循环检测，避免无限循环
				if visitedModels[mappedModel] {
					if mappedModel == currentModel {
						if currentModel == info.OriginModelName {
							info.IsModelMapped = false
							return nil
						} else {
							info.IsModelMapped = true
							break
						}
					}
					return errors.New("model_mapping_contains_cycle")
				}
				visitedModels[mappedModel] = true
				currentModel = mappedModel
				info.IsModelMapped = true
			} else {
				break
			}
		}
		if info.IsModelMapped {
			info.UpstreamModelName = currentModel
		}
	}

	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
