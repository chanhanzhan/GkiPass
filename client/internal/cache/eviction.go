package cache

import (
	"container/heap"
	"math/rand"
	"sync"
)

// LRUPolicy LRU淘汰策略
type LRUPolicy struct {
	mutex sync.RWMutex
}

func (p *LRUPolicy) OnAccess(entry *CacheEntry) {
	// LRU策略通过链表位置维护，访问时移动到头部
}

func (p *LRUPolicy) OnAdd(entry *CacheEntry) {
	// 添加时自动放在头部
}

func (p *LRUPolicy) OnEvict() *CacheEntry {
	// 返回nil，让调用方从LRU尾部淘汰
	return nil
}

func (p *LRUPolicy) Name() string {
	return "lru"
}

// LFUPolicy LFU淘汰策略
type LFUPolicy struct {
	mutex       sync.RWMutex
	frequencies map[*CacheEntry]*LFUNode
	minHeap     *LFUHeap
}

type LFUNode struct {
	entry     *CacheEntry
	frequency int64
	index     int // heap中的索引
}

type LFUHeap []*LFUNode

func (h LFUHeap) Len() int           { return len(h) }
func (h LFUHeap) Less(i, j int) bool { return h[i].frequency < h[j].frequency }
func (h LFUHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *LFUHeap) Push(x interface{}) {
	n := len(*h)
	node := x.(*LFUNode)
	node.index = n
	*h = append(*h, node)
}

func (h *LFUHeap) Pop() interface{} {
	old := *h
	n := len(old)
	node := old[n-1]
	old[n-1] = nil
	node.index = -1
	*h = old[0 : n-1]
	return node
}

func NewLFUPolicy() *LFUPolicy {
	return &LFUPolicy{
		frequencies: make(map[*CacheEntry]*LFUNode),
		minHeap:     &LFUHeap{},
	}
}

func (p *LFUPolicy) OnAccess(entry *CacheEntry) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if node, exists := p.frequencies[entry]; exists {
		node.frequency = entry.AccessCount
		heap.Fix(p.minHeap, node.index)
	}
}

func (p *LFUPolicy) OnAdd(entry *CacheEntry) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	node := &LFUNode{
		entry:     entry,
		frequency: entry.AccessCount,
	}
	p.frequencies[entry] = node
	heap.Push(p.minHeap, node)
}

func (p *LFUPolicy) OnEvict() *CacheEntry {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.minHeap.Len() == 0 {
		return nil
	}

	node := heap.Pop(p.minHeap).(*LFUNode)
	delete(p.frequencies, node.entry)
	return node.entry
}

func (p *LFUPolicy) Name() string {
	return "lfu"
}

// FIFOPolicy FIFO淘汰策略
type FIFOPolicy struct {
	mutex sync.RWMutex
	queue []*CacheEntry
}

func NewFIFOPolicy() *FIFOPolicy {
	return &FIFOPolicy{
		queue: make([]*CacheEntry, 0),
	}
}

func (p *FIFOPolicy) OnAccess(entry *CacheEntry) {
	// FIFO不需要处理访问
}

func (p *FIFOPolicy) OnAdd(entry *CacheEntry) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.queue = append(p.queue, entry)
}

func (p *FIFOPolicy) OnEvict() *CacheEntry {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.queue) == 0 {
		return nil
	}

	entry := p.queue[0]
	p.queue = p.queue[1:]
	return entry
}

func (p *FIFOPolicy) Name() string {
	return "fifo"
}

// RandomPolicy 随机淘汰策略
type RandomPolicy struct {
	mutex   sync.RWMutex
	entries []*CacheEntry
}

func NewRandomPolicy() *RandomPolicy {
	return &RandomPolicy{
		entries: make([]*CacheEntry, 0),
	}
}

func (p *RandomPolicy) OnAccess(entry *CacheEntry) {
	// 随机策略不需要处理访问
}

func (p *RandomPolicy) OnAdd(entry *CacheEntry) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.entries = append(p.entries, entry)
}

func (p *RandomPolicy) OnEvict() *CacheEntry {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.entries) == 0 {
		return nil
	}

	// 随机选择一个条目
	index := rand.Intn(len(p.entries))
	entry := p.entries[index]

	// 从切片中移除
	p.entries[index] = p.entries[len(p.entries)-1]
	p.entries = p.entries[:len(p.entries)-1]

	return entry
}

func (p *RandomPolicy) Name() string {
	return "random"
}

// AdaptivePolicy 自适应淘汰策略
type AdaptivePolicy struct {
	mutex       sync.RWMutex
	lruPolicy   EvictionPolicy
	lfuPolicy   EvictionPolicy
	currentMode string
	hitRates    []float64
	windowSize  int
}

func NewAdaptivePolicy() *AdaptivePolicy {
	return &AdaptivePolicy{
		lruPolicy:   &LRUPolicy{},
		lfuPolicy:   NewLFUPolicy(),
		currentMode: "lru",
		hitRates:    make([]float64, 0, 10),
		windowSize:  10,
	}
}

func (p *AdaptivePolicy) OnAccess(entry *CacheEntry) {
	p.mutex.RLock()
	currentMode := p.currentMode
	p.mutex.RUnlock()

	if currentMode == "lru" {
		p.lruPolicy.OnAccess(entry)
	} else {
		p.lfuPolicy.OnAccess(entry)
	}
}

func (p *AdaptivePolicy) OnAdd(entry *CacheEntry) {
	p.mutex.RLock()
	currentMode := p.currentMode
	p.mutex.RUnlock()

	if currentMode == "lru" {
		p.lruPolicy.OnAdd(entry)
	} else {
		p.lfuPolicy.OnAdd(entry)
	}
}

func (p *AdaptivePolicy) OnEvict() *CacheEntry {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.currentMode == "lru" {
		return p.lruPolicy.OnEvict()
	} else {
		return p.lfuPolicy.OnEvict()
	}
}

func (p *AdaptivePolicy) Name() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return "adaptive-" + p.currentMode
}

// UpdateHitRate 更新命中率，用于自适应调整
func (p *AdaptivePolicy) UpdateHitRate(hitRate float64) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.hitRates = append(p.hitRates, hitRate)
	if len(p.hitRates) > p.windowSize {
		p.hitRates = p.hitRates[1:]
	}

	// 如果有足够的样本，评估是否需要切换策略
	if len(p.hitRates) >= p.windowSize {
		p.evaluateStrategy()
	}
}

func (p *AdaptivePolicy) evaluateStrategy() {
	// 计算最近的平均命中率
	var sum float64
	for _, rate := range p.hitRates {
		sum += rate
	}
	avgHitRate := sum / float64(len(p.hitRates))

	// 根据命中率趋势调整策略
	if avgHitRate < 0.6 && p.currentMode == "lru" {
		// 命中率较低，切换到LFU
		p.currentMode = "lfu"
	} else if avgHitRate > 0.8 && p.currentMode == "lfu" {
		// 命中率较高，切换回LRU
		p.currentMode = "lru"
	}
}

// createEvictionPolicy 创建淘汰策略实例
func createEvictionPolicy(policyName string) EvictionPolicy {
	switch policyName {
	case "lfu":
		return NewLFUPolicy()
	case "fifo":
		return NewFIFOPolicy()
	case "random":
		return NewRandomPolicy()
	case "adaptive":
		return NewAdaptivePolicy()
	default:
		return &LRUPolicy{}
	}
}

// TTLEvictionPolicy TTL过期淘汰策略
type TTLEvictionPolicy struct {
	basePolicy EvictionPolicy
	mutex      sync.RWMutex
}

func NewTTLEvictionPolicy(basePolicy EvictionPolicy) *TTLEvictionPolicy {
	return &TTLEvictionPolicy{
		basePolicy: basePolicy,
	}
}

func (p *TTLEvictionPolicy) OnAccess(entry *CacheEntry) {
	if !entry.IsExpired() {
		p.basePolicy.OnAccess(entry)
	}
}

func (p *TTLEvictionPolicy) OnAdd(entry *CacheEntry) {
	p.basePolicy.OnAdd(entry)
}

func (p *TTLEvictionPolicy) OnEvict() *CacheEntry {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 首先尝试淘汰过期的条目
	// 这里简化实现，实际应该维护过期条目的索引
	return p.basePolicy.OnEvict()
}

func (p *TTLEvictionPolicy) Name() string {
	return "ttl-" + p.basePolicy.Name()
}

// SizeBasedEvictionPolicy 基于大小的淘汰策略
type SizeBasedEvictionPolicy struct {
	basePolicy  EvictionPolicy
	sizePenalty float64 // 大小惩罚因子
	mutex       sync.RWMutex
}

func NewSizeBasedEvictionPolicy(basePolicy EvictionPolicy, sizePenalty float64) *SizeBasedEvictionPolicy {
	return &SizeBasedEvictionPolicy{
		basePolicy:  basePolicy,
		sizePenalty: sizePenalty,
	}
}

func (p *SizeBasedEvictionPolicy) OnAccess(entry *CacheEntry) {
	p.basePolicy.OnAccess(entry)
}

func (p *SizeBasedEvictionPolicy) OnAdd(entry *CacheEntry) {
	p.basePolicy.OnAdd(entry)
}

func (p *SizeBasedEvictionPolicy) OnEvict() *CacheEntry {
	// 优先淘汰大的条目
	// 这里简化实现，实际应该考虑大小和访问模式的权衡
	return p.basePolicy.OnEvict()
}

func (p *SizeBasedEvictionPolicy) Name() string {
	return "size-" + p.basePolicy.Name()
}



