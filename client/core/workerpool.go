package core

import (
	"sync"
)

// WorkerPool Goroutine工作池
type WorkerPool struct {
	maxWorkers  int
	taskQueue   chan func()
	workerQueue chan struct{}
	wg          sync.WaitGroup
	stopOnce    sync.Once
	stopChan    chan struct{}
}

// NewWorkerPool 创建工作池
func NewWorkerPool(maxWorkers int) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = 10000 // 默认最大10000个worker
	}

	wp := &WorkerPool{
		maxWorkers:  maxWorkers,
		taskQueue:   make(chan func(), maxWorkers*2),
		workerQueue: make(chan struct{}, maxWorkers),
		stopChan:    make(chan struct{}),
	}

	// 预填充worker信号量
	for i := 0; i < maxWorkers; i++ {
		wp.workerQueue <- struct{}{}
	}

	return wp
}

// Start 启动工作池
func (wp *WorkerPool) Start() {
	go wp.dispatcher()
}

// Submit 提交任务
func (wp *WorkerPool) Submit(task func()) bool {
	select {
	case wp.taskQueue <- task:
		return true
	case <-wp.stopChan:
		return false
	default:
		// 任务队列满，拒绝任务
		return false
	}
}

// dispatcher 任务分发器
func (wp *WorkerPool) dispatcher() {
	for {
		select {
		case task := <-wp.taskQueue:
			// 等待worker槽位
			select {
			case <-wp.workerQueue:
				wp.wg.Add(1)
				go wp.worker(task)
			case <-wp.stopChan:
				return
			}
		case <-wp.stopChan:
			return
		}
	}
}

// worker 工作协程
func (wp *WorkerPool) worker(task func()) {
	defer func() {
		// 归还worker槽位
		wp.workerQueue <- struct{}{}
		wp.wg.Done()

		// 恢复panic
		if r := recover(); r != nil {
			// logger.Error("worker panic", zap.Any("panic", r))
		}
	}()

	// 执行任务
	task()
}

// Stop 停止工作池
func (wp *WorkerPool) Stop() {
	wp.stopOnce.Do(func() {
		close(wp.stopChan)
		wp.wg.Wait() // 等待所有worker完成
		close(wp.taskQueue)
	})
}

// ActiveWorkers 获取活跃worker数量
func (wp *WorkerPool) ActiveWorkers() int {
	return wp.maxWorkers - len(wp.workerQueue)
}

// QueuedTasks 获取队列中的任务数
func (wp *WorkerPool) QueuedTasks() int {
	return len(wp.taskQueue)
}
