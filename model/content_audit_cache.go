package model

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const (
	contentAuditPolicyCacheTTL = 3 * time.Second
	contentAuditCacheTimeout   = 250 * time.Millisecond
	contentAuditCacheMaxBytes  = 64 << 10
)

// A reader can publish a policy only while it still owns its unique loading
// marker. Saving rules replaces that marker before SQL and deletes it after
// SQL, so a delayed read cannot resurrect a pre-save policy.
var finishContentAuditPolicyRead = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
    if ARGV[2] ~= '' then
        redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
    else
        redis.call('DEL', KEYS[1])
    end
end
return 0`)

func contentAuditPolicyCacheKey(userID int) string {
	return "content_audit:policy:v1:" + strconv.Itoa(userID)
}

// GetContentAuditPolicyForRelay caches encrypted policies and known absence.
// Settings pages must keep using GetContentAuditPolicy: disabled policies may
// still contain rules that their owner needs to edit. No request content or
// decrypted rules enter Redis, entries have a size limit, and hits never extend
// the short TTL. Invalid ciphertext and SQL errors are not cached.
//
// Use the same Redis DB on all nodes of one deployment. There is deliberately
// no local negative cache or Pub/Sub dependency: Redis faults fall back to SQL.
func GetContentAuditPolicyForRelay(ctx context.Context, userID int) (ContentAuditPolicy, error) {
	if err := ctx.Err(); err != nil {
		return ContentAuditPolicy{}, err
	}
	rdb := common.RDB
	if !common.RedisEnabled || rdb == nil || userID <= 0 {
		return GetContentAuditPolicy(ctx, userID)
	}
	key := contentAuditPolicyCacheKey(userID)
	cacheCtx, cancel := context.WithTimeout(ctx, contentAuditCacheTimeout)
	state, cacheErr := rdb.Get(cacheCtx, key).Result()
	loading := ""
	if cacheErr == nil {
		if state == "empty" {
			cancel()
			return ContentAuditPolicy{UserID: userID}, ctx.Err()
		}
		if len(state) <= contentAuditCacheMaxBytes && strings.HasPrefix(state, "policy:") {
			// Require both fields so a truncated/malformed entry cannot silently
			// become an empty policy and bypass auditing.
			var cached struct {
				Enabled        *bool   `json:"enabled"`
				EncryptedRules *string `json:"rules"`
			}
			if common.UnmarshalJsonStr(strings.TrimPrefix(state, "policy:"), &cached) == nil && cached.Enabled != nil && cached.EncryptedRules != nil {
				cancel()
				return ContentAuditPolicy{UserID: userID, Enabled: *cached.Enabled, EncryptedRules: *cached.EncryptedRules}, ctx.Err()
			}
		}
	}
	if errors.Is(cacheErr, redis.Nil) {
		candidate := "loading:" + uuid.NewString()
		if ownsRead, err := rdb.SetNX(cacheCtx, key, candidate, contentAuditPolicyCacheTTL).Result(); err == nil && ownsRead {
			loading = candidate
		}
	}
	cancel()

	row, err := GetContentAuditPolicy(ctx, userID)
	if loading != "" {
		state = ""
		if err == nil && ctx.Err() == nil {
			if !row.Enabled {
				state = "empty"
			} else if _, decodeErr := DecodeContentAuditPolicy(row); decodeErr == nil {
				encoded, encodeErr := common.Marshal(struct {
					Enabled bool   `json:"enabled"`
					Rules   string `json:"rules"`
				}{Enabled: row.Enabled, Rules: row.EncryptedRules})
				if encodeErr == nil && len(encoded)+len("policy:") <= contentAuditCacheMaxBytes {
					state = "policy:" + string(encoded)
				}
			}
		}
		cacheCtx, cancel := context.WithTimeout(ctx, contentAuditCacheTimeout)
		// Cache population is optional. In particular, SQL errors must never
		// become an empty policy, and a failed CAS must not change SQL results.
		_ = finishContentAuditPolicyRead.Run(cacheCtx, rdb, []string{key}, loading, state, contentAuditPolicyCacheTTL.Milliseconds()).Err()
		cancel()
	}
	return row, err
}
