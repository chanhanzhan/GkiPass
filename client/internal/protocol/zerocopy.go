package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// ZeroCopyConfig 零拷贝配置
type ZeroCopyConfig struct {
	// 启用选项
	EnableSendfile  bool `json:"enable_sendfile"`   // 启用sendfile系统调用
	EnableSplice    bool `json:"enable_splice"`     // 启用splice系统调用
	EnableMemoryMap bool `json:"enable_memory_map"` // 启用内存映射
	EnableDirectIO  bool `json:"enable_direct_io"`  // 启用直接IO

	// 缓冲区配置
	BufferSize     int `json:"buffer_size"`      // 缓冲区大小
	ChunkSize      int `json:"chunk_size"`       // 块大小
	MaxMemoryUsage int `json:"max_memory_usage"` // 最大内存使用量

	// 性能调优
	BatchSize        int           `json:"batch_size"`        // 批处理大小
	FlushInterval    time.Duration `json:"flush_interval"`    // 刷新间隔
	ConcurrentCopies int           `json:"concurrent_copies"` // 并发拷贝数
}

// DefaultZeroCopyConfig 默认零拷贝配置
func DefaultZeroCopyConfig() *ZeroCopyConfig {
	return &ZeroCopyConfig{
		EnableSendfile:   true,
		EnableSplice:     true,
		EnableMemoryMap:  true,
		EnableDirectIO:   false,             // 直接IO通常需要特殊的文件系统支持
		BufferSize:       64 * 1024,         // 64KB
		ChunkSize:        4 * 1024,          // 4KB
		MaxMemoryUsage:   256 * 1024 * 1024, // 256MB
		BatchSize:        16,
		FlushInterval:    10 * time.Millisecond,
		ConcurrentCopies: 4,
	}
}

// ZeroCopyEngine 零拷贝引擎
type ZeroCopyEngine struct {
	config *ZeroCopyConfig
	logger *zap.Logger

	// 缓冲池
	bufferPool sync.Pool
	chunkPool  sync.Pool

	// 统计信息
	stats struct {
		totalBytes    atomic.Int64
		sendfileBytes atomic.Int64
		spliceBytes   atomic.Int64
		mmapBytes     atomic.Int64
		fallbackBytes atomic.Int64
		operations    atomic.Int64
		errors        atomic.Int64
		avgLatency    atomic.Int64 // 纳秒
	}

	// 工作池
	workerPool chan *CopyJob
	workers    sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// CopyJob 拷贝任务
type CopyJob struct {
	ID       string
	Source   io.Reader
	Dest     io.Writer
	Size     int64
	Callback func(job *CopyJob, err error)
	Priority int
	Metadata map[string]interface{}

	// 内部字段
	startTime   time.Time
	method      string
	bytesCopied int64
}

// CopyMethod 拷贝方法
type CopyMethod string

const (
	MethodSendfile CopyMethod = "sendfile"
	MethodSplice   CopyMethod = "splice"
	MethodMMap     CopyMethod = "mmap"
	MethodStandard CopyMethod = "standard"
	MethodDirect   CopyMethod = "direct"
)

// NewZeroCopyEngine 创建零拷贝引擎
func NewZeroCopyEngine(config *ZeroCopyConfig) *ZeroCopyEngine {
	if config == nil {
		config = DefaultZeroCopyConfig()
	}

	engine := &ZeroCopyEngine{
		config:     config,
		logger:     zap.L().Named("zerocopy"),
		workerPool: make(chan *CopyJob, config.ConcurrentCopies*2),
	}

	// 初始化缓冲池
	engine.bufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, config.BufferSize)
		},
	}

	engine.chunkPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, config.ChunkSize)
		},
	}

	return engine
}

// Start 启动零拷贝引擎
func (zce *ZeroCopyEngine) Start(ctx context.Context) error {
	zce.ctx, zce.cancel = context.WithCancel(ctx)

	// 启动工作协程
	for i := 0; i < zce.config.ConcurrentCopies; i++ {
		zce.workers.Add(1)
		go zce.worker(i)
	}

	zce.logger.Info("🚀 零拷贝引擎启动",
		zap.Int("workers", zce.config.ConcurrentCopies),
		zap.Int("buffer_size", zce.config.BufferSize),
		zap.Bool("sendfile", zce.config.EnableSendfile),
		zap.Bool("splice", zce.config.EnableSplice),
		zap.Bool("mmap", zce.config.EnableMemoryMap))

	return nil
}

// Stop 停止零拷贝引擎
func (zce *ZeroCopyEngine) Stop() error {
	zce.logger.Info("🛑 正在停止零拷贝引擎...")

	if zce.cancel != nil {
		zce.cancel()
	}

	// 关闭工作池
	close(zce.workerPool)

	// 等待所有工作协程结束
	zce.workers.Wait()

	zce.logger.Info("✅ 零拷贝引擎已停止")
	return nil
}

// Copy 执行零拷贝数据传输
func (zce *ZeroCopyEngine) Copy(source io.Reader, dest io.Writer, size int64) (int64, error) {
	job := &CopyJob{
		ID:       fmt.Sprintf("sync-%d", time.Now().UnixNano()),
		Source:   source,
		Dest:     dest,
		Size:     size,
		Priority: 5,
	}

	return zce.executeJob(job)
}

// CopyAsync 异步执行零拷贝数据传输
func (zce *ZeroCopyEngine) CopyAsync(job *CopyJob) error {
	job.startTime = time.Now()

	select {
	case zce.workerPool <- job:
		return nil
	case <-zce.ctx.Done():
		return zce.ctx.Err()
	default:
		// 工作池满，降级为同步执行
		zce.logger.Warn("工作池满，降级为同步执行", zap.String("job_id", job.ID))
		_, err := zce.executeJob(job)
		if job.Callback != nil {
			job.Callback(job, err)
		}
		return err
	}
}

// worker 工作协程
func (zce *ZeroCopyEngine) worker(id int) {
	defer zce.workers.Done()

	zce.logger.Debug("启动工作协程", zap.Int("worker_id", id))

	for job := range zce.workerPool {
		_, err := zce.executeJob(job)

		if job.Callback != nil {
			job.Callback(job, err)
		}
	}

	zce.logger.Debug("工作协程结束", zap.Int("worker_id", id))
}

// executeJob 执行拷贝任务
func (zce *ZeroCopyEngine) executeJob(job *CopyJob) (int64, error) {
	start := time.Now()
	defer func() {
		latency := time.Since(start)
		zce.updateLatencyStats(latency)
		zce.stats.operations.Add(1)
	}()

	// 尝试不同的零拷贝方法
	methods := zce.getOptimalMethods(job.Source, job.Dest, job.Size)

	var lastErr error
	for _, method := range methods {
		bytesCopied, err := zce.copyWithMethod(job, method)
		if err == nil {
			job.bytesCopied = bytesCopied
			job.method = string(method)
			zce.stats.totalBytes.Add(bytesCopied)
			zce.updateMethodStats(method, bytesCopied)
			return bytesCopied, nil
		}

		lastErr = err
		zce.logger.Debug("拷贝方法失败，尝试下一个",
			zap.String("method", string(method)),
			zap.Error(err))
	}

	// 所有方法都失败
	zce.stats.errors.Add(1)
	return 0, fmt.Errorf("所有拷贝方法都失败: %w", lastErr)
}

// getOptimalMethods 获取最优拷贝方法序列
func (zce *ZeroCopyEngine) getOptimalMethods(source io.Reader, dest io.Writer, size int64) []CopyMethod {
	methods := make([]CopyMethod, 0, 4)

	// 检查是否可以使用sendfile
	if zce.config.EnableSendfile && zce.canUseSendfile(source, dest) {
		methods = append(methods, MethodSendfile)
	}

	// 检查是否可以使用splice
	if zce.config.EnableSplice && zce.canUseSplice(source, dest) {
		methods = append(methods, MethodSplice)
	}

	// 检查是否可以使用内存映射
	if zce.config.EnableMemoryMap && zce.canUseMMap(source, size) {
		methods = append(methods, MethodMMap)
	}

	// 标准拷贝作为兜底
	methods = append(methods, MethodStandard)

	return methods
}

// canUseSendfile 检查是否可以使用sendfile
func (zce *ZeroCopyEngine) canUseSendfile(source io.Reader, dest io.Writer) bool {
	// sendfile需要源是文件，目标是socket
	sourceFile, sourceOK := source.(*os.File)
	destConn, destOK := dest.(net.Conn)

	if !sourceOK || !destOK {
		return false
	}

	// 检查文件是否为常规文件
	if stat, err := sourceFile.Stat(); err != nil || !stat.Mode().IsRegular() {
		return false
	}

	// 检查连接是否为TCP
	if tcpConn, ok := destConn.(*net.TCPConn); ok {
		return tcpConn != nil
	}

	return false
}

// canUseSplice 检查是否可以使用splice
func (zce *ZeroCopyEngine) canUseSplice(source io.Reader, dest io.Writer) bool {
	// splice需要至少一端是pipe、socket或文件
	// 这里简化检查，实际应该更严格
	_, sourceIsFD := source.(interface{ Fd() uintptr })
	_, destIsFD := dest.(interface{ Fd() uintptr })

	return sourceIsFD && destIsFD
}

// canUseMMap 检查是否可以使用mmap
func (zce *ZeroCopyEngine) canUseMMap(source io.Reader, size int64) bool {
	// mmap需要源是文件，且大小合适
	sourceFile, ok := source.(*os.File)
	if !ok {
		return false
	}

	// 检查文件大小是否适合内存映射
	if size <= 0 || size > int64(zce.config.MaxMemoryUsage) {
		return false
	}

	// 检查文件是否为常规文件
	if stat, err := sourceFile.Stat(); err != nil || !stat.Mode().IsRegular() {
		return false
	}

	return true
}

// copyWithMethod 使用指定方法进行拷贝
func (zce *ZeroCopyEngine) copyWithMethod(job *CopyJob, method CopyMethod) (int64, error) {
	switch method {
	case MethodSendfile:
		return zce.copyWithSendfile(job)
	case MethodSplice:
		return zce.copyWithSplice(job)
	case MethodMMap:
		return zce.copyWithMMap(job)
	case MethodStandard:
		return zce.copyWithStandard(job)
	default:
		return 0, fmt.Errorf("未知的拷贝方法: %s", method)
	}
}

// copyWithSendfile 使用sendfile进行拷贝
func (zce *ZeroCopyEngine) copyWithSendfile(job *CopyJob) (int64, error) {
	sourceFile := job.Source.(*os.File)
	destConn := job.Dest.(net.Conn)

	// 获取TCP连接的文件描述符
	tcpConn := destConn.(*net.TCPConn)
	destFile, err := tcpConn.File()
	if err != nil {
		return 0, fmt.Errorf("获取目标文件描述符失败: %w", err)
	}
	defer destFile.Close()

	// 获取源文件大小
	stat, err := sourceFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("获取源文件信息失败: %w", err)
	}

	size := stat.Size()
	if job.Size > 0 && job.Size < size {
		size = job.Size
	}

	// 执行sendfile系统调用
	return zce.sendfile(int(destFile.Fd()), int(sourceFile.Fd()), size)
}

// sendfile 执行sendfile系统调用
func (zce *ZeroCopyEngine) sendfile(destFd, sourceFd int, size int64) (int64, error) {
	var totalCopied int64

	for totalCopied < size {
		remaining := size - totalCopied
		chunkSize := int64(zce.config.ChunkSize)
		if remaining < chunkSize {
			chunkSize = remaining
		}

		copied, err := syscall.Sendfile(destFd, sourceFd, nil, int(chunkSize))
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EINTR {
				continue // 重试
			}
			return totalCopied, fmt.Errorf("sendfile失败: %w", err)
		}

		if copied == 0 {
			break // EOF
		}

		totalCopied += int64(copied)
	}

	return totalCopied, nil
}

// copyWithSplice 使用splice进行拷贝
func (zce *ZeroCopyEngine) copyWithSplice(job *CopyJob) (int64, error) {
	// 创建管道
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		return 0, fmt.Errorf("创建管道失败: %w", err)
	}
	defer pipeR.Close()
	defer pipeW.Close()

	// 获取文件描述符
	sourceFd := zce.getFd(job.Source)
	destFd := zce.getFd(job.Dest)

	if sourceFd < 0 || destFd < 0 {
		return 0, fmt.Errorf("无法获取文件描述符")
	}

	var totalCopied int64
	size := job.Size
	if size <= 0 {
		size = 1 << 62 // 非常大的数，表示复制到EOF
	}

	for totalCopied < size {
		remaining := size - totalCopied
		chunkSize := int64(zce.config.ChunkSize)
		if remaining < chunkSize {
			chunkSize = remaining
		}

		// 从源splice到pipe
		copied1, err := zce.splice(sourceFd, int(pipeW.Fd()), int(chunkSize))
		if err != nil {
			return totalCopied, fmt.Errorf("splice到管道失败: %w", err)
		}

		if copied1 == 0 {
			break // EOF
		}

		// 从pipe splice到目标
		copied2, err := zce.splice(int(pipeR.Fd()), destFd, copied1)
		if err != nil {
			return totalCopied, fmt.Errorf("从管道splice失败: %w", err)
		}

		totalCopied += int64(copied2)
	}

	return totalCopied, nil
}

// splice 执行splice系统调用
func (zce *ZeroCopyEngine) splice(sourceFd, destFd, size int) (int, error) {
	// 注意：这是一个简化的实现
	// 实际的splice系统调用在Go中需要使用syscall.RawSyscall6
	// 这里返回错误以表示需要更详细的实现
	return 0, fmt.Errorf("splice系统调用需要更详细的实现")
}

// copyWithMMap 使用内存映射进行拷贝
func (zce *ZeroCopyEngine) copyWithMMap(job *CopyJob) (int64, error) {
	sourceFile := job.Source.(*os.File)

	// 获取文件大小
	stat, err := sourceFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("获取文件信息失败: %w", err)
	}

	size := stat.Size()
	if job.Size > 0 && job.Size < size {
		size = job.Size
	}

	// 内存映射文件
	data, err := zce.mmap(sourceFile, size)
	if err != nil {
		return 0, fmt.Errorf("内存映射失败: %w", err)
	}
	defer zce.munmap(data)

	// 写入目标
	totalWritten := int64(0)
	for totalWritten < size {
		remaining := size - totalWritten
		chunkSize := int64(zce.config.ChunkSize)
		if remaining < chunkSize {
			chunkSize = remaining
		}

		chunk := data[totalWritten : totalWritten+chunkSize]
		written, err := job.Dest.Write(chunk)
		if err != nil {
			return totalWritten, fmt.Errorf("写入失败: %w", err)
		}

		totalWritten += int64(written)
	}

	return totalWritten, nil
}

// mmap 内存映射文件
func (zce *ZeroCopyEngine) mmap(file *os.File, size int64) ([]byte, error) {
	// 使用mmap系统调用
	data, err := syscall.Mmap(int(file.Fd()), 0, int(size),
		syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap失败: %w", err)
	}

	return data, nil
}

// munmap 取消内存映射
func (zce *ZeroCopyEngine) munmap(data []byte) error {
	return syscall.Munmap(data)
}

// copyWithStandard 使用标准拷贝
func (zce *ZeroCopyEngine) copyWithStandard(job *CopyJob) (int64, error) {
	buffer := zce.bufferPool.Get().([]byte)
	defer zce.bufferPool.Put(buffer)

	return io.CopyBuffer(job.Dest, job.Source, buffer)
}

// getFd 获取文件描述符
func (zce *ZeroCopyEngine) getFd(rw interface{}) int {
	if file, ok := rw.(*os.File); ok {
		return int(file.Fd())
	}

	if conn, ok := rw.(net.Conn); ok {
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if file, err := tcpConn.File(); err == nil {
				defer file.Close()
				return int(file.Fd())
			}
		}
	}

	// 尝试使用反射获取文件描述符
	if fdGetter, ok := rw.(interface{ Fd() uintptr }); ok {
		return int(fdGetter.Fd())
	}

	return -1
}

// updateMethodStats 更新方法统计
func (zce *ZeroCopyEngine) updateMethodStats(method CopyMethod, bytes int64) {
	switch method {
	case MethodSendfile:
		zce.stats.sendfileBytes.Add(bytes)
	case MethodSplice:
		zce.stats.spliceBytes.Add(bytes)
	case MethodMMap:
		zce.stats.mmapBytes.Add(bytes)
	default:
		zce.stats.fallbackBytes.Add(bytes)
	}
}

// updateLatencyStats 更新延迟统计
func (zce *ZeroCopyEngine) updateLatencyStats(latency time.Duration) {
	// 简单的移动平均
	oldAvg := zce.stats.avgLatency.Load()
	newAvg := (oldAvg*9 + latency.Nanoseconds()) / 10
	zce.stats.avgLatency.Store(newAvg)
}

// GetStats 获取统计信息
func (zce *ZeroCopyEngine) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_bytes":    zce.stats.totalBytes.Load(),
		"sendfile_bytes": zce.stats.sendfileBytes.Load(),
		"splice_bytes":   zce.stats.spliceBytes.Load(),
		"mmap_bytes":     zce.stats.mmapBytes.Load(),
		"fallback_bytes": zce.stats.fallbackBytes.Load(),
		"operations":     zce.stats.operations.Load(),
		"errors":         zce.stats.errors.Load(),
		"avg_latency_ns": zce.stats.avgLatency.Load(),
		"avg_latency_ms": float64(zce.stats.avgLatency.Load()) / 1e6,
		"config":         zce.config,
	}
}

// OptimizeForLatency 为延迟优化配置
func (zce *ZeroCopyEngine) OptimizeForLatency() {
	zce.config.ChunkSize = 1024 // 更小的块
	zce.config.FlushInterval = 1 * time.Millisecond
	zce.config.ConcurrentCopies = 8 // 更多并发

	zce.logger.Info("🚀 零拷贝引擎已优化延迟")
}

// OptimizeForThroughput 为吞吐量优化配置
func (zce *ZeroCopyEngine) OptimizeForThroughput() {
	zce.config.ChunkSize = 64 * 1024 // 更大的块
	zce.config.BufferSize = 256 * 1024
	zce.config.BatchSize = 32 // 更大的批次

	zce.logger.Info("📊 零拷贝引擎已优化吞吐量")
}

// ZeroCopyStream 零拷贝流接口
type ZeroCopyStream interface {
	io.Reader
	io.Writer
	io.Closer

	// ZeroCopy 执行零拷贝传输
	ZeroCopy(dest ZeroCopyStream, size int64) (int64, error)

	// GetFd 获取文件描述符（如果支持）
	GetFd() (int, bool)

	// SupportsSplice 是否支持splice
	SupportsSplice() bool

	// SupportsSendfile 是否支持sendfile
	SupportsSendfile() bool
}

// WrapAsZeroCopyStream 将普通流包装为零拷贝流
func WrapAsZeroCopyStream(rw io.ReadWriter, engine *ZeroCopyEngine) ZeroCopyStream {
	return &zeroCopyStreamWrapper{
		ReadWriter: rw,
		engine:     engine,
	}
}

// zeroCopyStreamWrapper 零拷贝流包装器
type zeroCopyStreamWrapper struct {
	io.ReadWriter
	engine *ZeroCopyEngine
}

func (w *zeroCopyStreamWrapper) Close() error {
	if closer, ok := w.ReadWriter.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (w *zeroCopyStreamWrapper) ZeroCopy(dest ZeroCopyStream, size int64) (int64, error) {
	return w.engine.Copy(w, dest, size)
}

func (w *zeroCopyStreamWrapper) GetFd() (int, bool) {
	fd := w.engine.getFd(w.ReadWriter)
	return fd, fd >= 0
}

func (w *zeroCopyStreamWrapper) SupportsSplice() bool {
	return w.engine.canUseSplice(w, w)
}

func (w *zeroCopyStreamWrapper) SupportsSendfile() bool {
	return w.engine.canUseSendfile(w, w)
}

// 确保实现了接口
var _ ZeroCopyStream = (*zeroCopyStreamWrapper)(nil)



