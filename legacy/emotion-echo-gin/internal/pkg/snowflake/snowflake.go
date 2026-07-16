package snowflake

import (
	"sync"
	"time"
)

// Snowflake 算法常量
const (
	workerBits     uint8 = 10
	sequenceBits   uint8 = 12
	workerMax      int64 = -1 ^ (-1 << workerBits)
	sequenceMask   int64 = -1 ^ (-1 << sequenceBits)
	timeLeft       uint8 = 22
	workerLeft     uint8 = 12
	twepoch        int64 = 1609459200000 // 2021-01-01 00:00:00 UTC
)

// Generator ID 生成器
type Generator struct {
	mu        sync.Mutex
	timestamp int64
	workerID  int64
	sequence  int64
}

// NewGenerator 创建生成器
func NewGenerator(workerID int64) *Generator {
	if workerID < 0 || workerID > workerMax {
		workerID = 0
	}
	return &Generator{
		workerID: workerID,
	}
}

// Generate 生成唯一 ID
func (g *Generator) Generate() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli()
	if g.timestamp == now {
		g.sequence = (g.sequence + 1) & sequenceMask
		if g.sequence == 0 {
			for now <= g.timestamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		g.sequence = 0
	}

	g.timestamp = now
	id := ((now - twepoch) << timeLeft) | (g.workerID << workerLeft) | g.sequence
	return id
}
