// Package fusion — Stage 35 · PR-3 GREEN
//
// msgIDLRU 是 FusionWorker 用的同 messageID 限流器。
//
// 目的：避免一个 tick 周期（5s）内对同一 messageID 重复调 LLM，浪费 LLM 配额。
// 触发场景：DB Upsert 与 ListPending 之间有时差，stale pending row 在相邻 tick 中反复列出。
//
// 设计：
//   - 纯内存 LRU（container/list + map[int64]*list.Element）
//   - cap 默认 1024，可通过 NewMsgIDLRU 覆盖
//   - TTL：若 msgID 在 TTL 内被 Add 过，Touch 返回 true（命中 → Worker 应 skip）
//   - 线程安全：sync.Mutex
//
// 操作语义：
//   - Add(msgID)：记录当前时间戳，若 cap 满则 Evict 最久未用
//   - Touch(msgID)：若 msgID 存在且未过期 → 返回 true（命中）；否则 Add + 返回 false
//
// 注意：时间戳取的是 Monotonic clock（time.Now），不依赖 wall clock。
package fusion

import (
	"container/list"
	"sync"
	"time"
)

// 默认 LRU 容量与 TTL。
const (
	defaultLRUCapacity = 1024
	defaultLRUTTL      = 4 * time.Minute // > tick 5s × 48，覆盖"半个多小时内的重复融合"
)

// msgIDLRUEntry LRU 节点值（存时间戳用于 TTL 判定）。
type msgIDLRUEntry struct {
	msgID   int64
	addedAt time.Time
}

// MsgIDLRU 线程安全的 LRU 缓存。
type MsgIDLRU struct {
	mu       sync.Mutex
	cap      int
	ttl      time.Duration
	items    map[int64]*list.Element // msgID → list element
	order    *list.List              // 双向链表：front=最新，back=最旧
	hits     int64                   // 累计命中次数（供 metrics 读取）
	misses   int64                   // 累计未命中次数（供 metrics 读取）
}

// NewMsgIDLRU 构造器。
//
// 参数：capacity 必须 > 0；ttl 必须 > 0。
func NewMsgIDLRU(capacity int, ttl time.Duration) *MsgIDLRU {
	if capacity <= 0 {
		capacity = defaultLRUCapacity
	}
	if ttl <= 0 {
		ttl = defaultLRUTTL
	}
	return &MsgIDLRU{
		cap:   capacity,
		ttl:   ttl,
		items: make(map[int64]*list.Element, capacity),
		order: list.New(),
	}
}

// Touch 判定 msgID 是否在 TTL 内已被 Add 过。
//
// 返回值：
//   - true：命中（Worker 应 skip，节省配额）
//   - false：未命中（Worker 应处理；同时内部 Add 记录但不裁剪）
//
// 行为约定：
//   - Touch 不触发 evict（避免误杀其他 entry）
//   - Touch 命中时只 MoveToFront + 刷新时间戳
//   - Touch 未命中时插入新 entry，但不裁剪（cap 是建议上限不是硬上限）
func (l *MsgIDLRU) Touch(msgID int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if elem, ok := l.items[msgID]; ok {
		entry := elem.Value.(*msgIDLRUEntry)
		if now.Sub(entry.addedAt) < l.ttl {
			// 命中：移到 front 刷新 recency
			l.order.MoveToFront(elem)
			entry.addedAt = now
			l.hits++
			return true
		}
		// 过期 → 删除原 entry
		l.order.Remove(elem)
		delete(l.items, msgID)
	}
	// 未命中（或已过期）→ 插入但不裁剪
	l.misses++
	entry := &msgIDLRUEntry{msgID: msgID, addedAt: now}
	elem := l.order.PushFront(entry)
	l.items[msgID] = elem
	return false
}

// Add 把 msgID 加入 LRU，记录当前时间戳。
//
// 若 cap 满 → Evict 最久未用（list back）。
// 已存在的 msgID → 更新到 list front 并刷新时间戳。
//
// 注：Add 触发 evict；Touch 不触发（见 Touch 注释）。
func (l *MsgIDLRU) Add(msgID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if elem, ok := l.items[msgID]; ok {
		l.order.MoveToFront(elem)
		elem.Value.(*msgIDLRUEntry).addedAt = now
		return
	}

	entry := &msgIDLRUEntry{msgID: msgID, addedAt: now}
	elem := l.order.PushFront(entry)
	l.items[msgID] = elem

	if l.order.Len() > l.cap {
		oldest := l.order.Back()
		if oldest != nil {
			l.order.Remove(oldest)
			delete(l.items, oldest.Value.(*msgIDLRUEntry).msgID)
		}
	}
}

// Len 当前 LRU 元素数。
func (l *MsgIDLRU) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.order.Len()
}

// Stats 返回 hits / misses 累计计数（供 metrics 读取）。
func (l *MsgIDLRU) Stats() (hits, misses int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.hits, l.misses
}