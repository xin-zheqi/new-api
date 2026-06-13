package common

import (
	"os"
	"sync"
	"time"
)

const (
	snowflakeEpoch     int64 = 1704067200000 // 2024-01-01T00:00:00Z
	snowflakeNodeBits        = 10
	snowflakeSeqBits         = 12
	snowflakeMaxNodeID int64 = -1 ^ (-1 << snowflakeNodeBits)
	snowflakeMaxSeq    int64 = -1 ^ (-1 << snowflakeSeqBits)
)

var (
	defaultSnowflake     *snowflakeGenerator
	defaultSnowflakeOnce sync.Once
)

type snowflakeGenerator struct {
	mu        sync.Mutex
	nodeID    int64
	lastMilli int64
	sequence  int64
}

func newSnowflakeGenerator(nodeID int64) *snowflakeGenerator {
	return &snowflakeGenerator{nodeID: nodeID & snowflakeMaxNodeID}
}

func defaultSnowflakeNodeID() int64 {
	seed := int64(os.Getpid())
	for _, b := range []byte(NodeName + GetIp()) {
		seed = seed*31 + int64(b)
	}
	return seed & snowflakeMaxNodeID
}

func currentMilli() int64 {
	return time.Now().UnixMilli()
}

func (g *snowflakeGenerator) nextID() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := currentMilli()
	if now < g.lastMilli {
		now = g.lastMilli
	}
	if now == g.lastMilli {
		g.sequence = (g.sequence + 1) & snowflakeMaxSeq
		if g.sequence == 0 {
			for now <= g.lastMilli {
				now = currentMilli()
			}
		}
	} else {
		g.sequence = 0
	}

	g.lastMilli = now
	return ((now - snowflakeEpoch) << (snowflakeNodeBits + snowflakeSeqBits)) |
		(g.nodeID << snowflakeSeqBits) |
		g.sequence
}

func NextSnowflakeID() int64 {
	defaultSnowflakeOnce.Do(func() {
		defaultSnowflake = newSnowflakeGenerator(defaultSnowflakeNodeID())
	})
	return defaultSnowflake.nextID()
}
