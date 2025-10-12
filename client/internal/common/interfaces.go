package common

import (
	"context"
	"time"
)

// Starter 启动器接口
type Starter interface {
	Start() error
}

// ContextStarter 带上下文的启动器接口
type ContextStarter interface {
	Start(ctx context.Context) error
}

// Stopper 停止器接口
type Stopper interface {
	Stop() error
}

// ContextStopper 带上下文的停止器接口
type ContextStopper interface {
	Stop(ctx context.Context) error
}

// Manager 管理器接口
type Manager interface {
	Starter
	Stopper
	GetStats() map[string]interface{}
}

// ContextManager 带上下文的管理器接口
type ContextManager interface {
	ContextStarter
	Stopper
	GetStats() map[string]interface{}
}

// Component 组件接口
type Component interface {
	GetName() string
	GetVersion() string
	IsHealthy() bool
	GetHealthScore() float64
	Manager
}

// HealthChecker 健康检查器接口
type HealthChecker interface {
	CheckHealth() error
	IsHealthy() bool
	GetHealthScore() float64
}

// Configurable 可配置接口
type Configurable interface {
	LoadConfig(config interface{}) error
	GetConfig() interface{}
	ValidateConfig() error
}

// Monitorable 可监控接口
type Monitorable interface {
	GetMetrics() map[string]interface{}
	GetStats() map[string]interface{}
	EnableMonitoring(enabled bool)
}

// Cacheable 可缓存接口
type Cacheable interface {
	GetCacheKey() string
	GetCacheExpiry() time.Duration
	IsCacheable() bool
}

// Retryable 可重试接口
type Retryable interface {
	ShouldRetry(attempt int, err error) bool
	GetRetryDelay(attempt int) time.Duration
	GetMaxRetries() int
}

// Poolable 可池化接口
type Poolable interface {
	Reset() error
	IsReusable() bool
	GetPoolKey() string
}

// Serializable 可序列化接口
type Serializable interface {
	Serialize() ([]byte, error)
	Deserialize(data []byte) error
}

// Validatable 可验证接口
type Validatable interface {
	Validate() error
	IsValid() bool
}

// Closable 可关闭接口
type Closable interface {
	Close() error
	IsClosed() bool
}

// Resource 资源接口
type Resource interface {
	GetID() string
	GetType() string
	GetSize() int64
	IsExpired() bool
	Closable
}

// ConnectionLike 连接类接口
type ConnectionLike interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	LocalAddr() string
	RemoteAddr() string
}

// Logger 日志接口
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Fatal(msg string, fields ...interface{})
}

// EventEmitter 事件发射器接口
type EventEmitter interface {
	Emit(event string, data interface{}) error
	Subscribe(event string, handler func(data interface{})) error
	Unsubscribe(event string, handler func(data interface{})) error
}

// ServiceDiscovery 服务发现接口
type ServiceDiscovery interface {
	Register(service ServiceInfo) error
	Unregister(serviceID string) error
	Discover(serviceName string) ([]ServiceInfo, error)
	Watch(serviceName string, callback func([]ServiceInfo)) error
}

// ServiceInfo 服务信息
type ServiceInfo struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Version  string            `json:"version"`
	Address  string            `json:"address"`
	Port     int               `json:"port"`
	Metadata map[string]string `json:"metadata"`
	Health   string            `json:"health"`
	Tags     []string          `json:"tags"`
}

// LoadBalancer 负载均衡器接口
type LoadBalancer interface {
	Select(targets []string) (string, error)
	UpdateWeights(weights map[string]float64) error
	GetAlgorithm() string
}

// CircuitBreaker 熔断器接口
type CircuitBreaker interface {
	Call(fn func() error) error
	IsOpen() bool
	GetState() string
	Reset()
}

// RateLimiter 限流器接口
type RateLimiter interface {
	Allow() bool
	AllowN(n int) bool
	Reserve() Reservation
	Wait(ctx context.Context) error
}

// Reservation 预约接口
type Reservation interface {
	OK() bool
	Cancel()
	Delay() time.Duration
}

// Cache 缓存接口
type Cache interface {
	Get(key string) (interface{}, error)
	Set(key string, value interface{}, ttl time.Duration) error
	Delete(key string) error
	Clear() error
	Exists(key string) bool
	Size() int64
}

// Queue 队列接口
type Queue interface {
	Enqueue(item interface{}) error
	Dequeue() (interface{}, error)
	Peek() (interface{}, error)
	Size() int
	IsEmpty() bool
	Clear() error
}

// Pool 池接口
type Pool interface {
	Get() (interface{}, error)
	Put(item interface{}) error
	Size() int
	Close() error
}

// Middleware 中间件接口
type Middleware interface {
	Process(ctx context.Context, req interface{}, next func(context.Context, interface{}) (interface{}, error)) (interface{}, error)
}

// Handler 处理器接口
type Handler interface {
	Handle(ctx context.Context, req interface{}) (interface{}, error)
}

// Filter 过滤器接口
type Filter interface {
	Filter(item interface{}) bool
}

// Transformer 转换器接口
type Transformer interface {
	Transform(input interface{}) (interface{}, error)
}

// Validator 验证器接口
type Validator interface {
	Validate(input interface{}) error
}

// Encoder 编码器接口
type Encoder interface {
	Encode(data interface{}) ([]byte, error)
}

// Decoder 解码器接口
type Decoder interface {
	Decode(data []byte, target interface{}) error
}

// Codec 编解码器接口
type Codec interface {
	Encoder
	Decoder
	GetContentType() string
}

// EventBus 事件总线接口
type EventBus interface {
	Publish(topic string, event interface{}) error
	Subscribe(topic string, handler EventHandler) error
	Unsubscribe(topic string, handler EventHandler) error
}

// EventHandler 事件处理器接口
type EventHandler interface {
	Handle(event interface{}) error
	GetName() string
}

// Repository 仓储接口
type Repository interface {
	Save(entity interface{}) error
	FindByID(id string) (interface{}, error)
	FindAll() ([]interface{}, error)
	Update(entity interface{}) error
	Delete(id string) error
}

// Transaction 事务接口
type Transaction interface {
	Begin() error
	Commit() error
	Rollback() error
	IsActive() bool
}

// Lock 锁接口
type Lock interface {
	Lock() error
	Unlock() error
	TryLock() bool
	TryLockWithTimeout(timeout time.Duration) bool
}

// Semaphore 信号量接口
type Semaphore interface {
	Acquire() error
	AcquireWithTimeout(timeout time.Duration) error
	Release() error
	Available() int
}

// WorkerPool 工作池接口
type WorkerPool interface {
	Submit(task func()) error
	SubmitWithTimeout(task func(), timeout time.Duration) error
	Shutdown() error
	GetActiveWorkers() int
	GetQueueSize() int
}

// Task 任务接口
type Task interface {
	Execute() error
	GetID() string
	GetPriority() int
	IsExpired() bool
}

// Scheduler 调度器接口
type Scheduler interface {
	Schedule(task Task, delay time.Duration) error
	ScheduleRepeating(task Task, interval time.Duration) error
	Cancel(taskID string) error
	Shutdown() error
}

// Observer 观察者接口
type Observer interface {
	Update(subject Subject, event interface{}) error
}

// Subject 主题接口
type Subject interface {
	Attach(observer Observer) error
	Detach(observer Observer) error
	Notify(event interface{}) error
}

// Router 路由器接口
type Router interface {
	Route(ctx context.Context, req interface{}) (Handler, error)
	AddRoute(pattern string, handler Handler) error
	RemoveRoute(pattern string) error
}

// Plugin 插件接口
type Plugin interface {
	Component
	Install() error
	Uninstall() error
	Configure(config map[string]interface{}) error
}

// Registry 注册表接口
type Registry interface {
	Register(name string, value interface{}) error
	Unregister(name string) error
	Get(name string) (interface{}, bool)
	List() []string
}

// Factory 工厂接口
type Factory interface {
	Create(params map[string]interface{}) (interface{}, error)
	GetSupportedTypes() []string
}

// Builder 构建器接口
type Builder interface {
	Build() (interface{}, error)
	Reset() Builder
}

// Strategy 策略接口
type Strategy interface {
	Execute(ctx context.Context, input interface{}) (interface{}, error)
	GetName() string
}

// Command 命令接口
type Command interface {
	Execute() error
	Undo() error
	CanUndo() bool
}

// State 状态接口
type State interface {
	Enter(ctx context.Context) error
	Exit(ctx context.Context) error
	Handle(ctx context.Context, event interface{}) (State, error)
	GetName() string
}

// StateMachine 状态机接口
type StateMachine interface {
	GetCurrentState() State
	Transition(event interface{}) error
	CanTransition(event interface{}) bool
}

// Adapter 适配器接口
type Adapter interface {
	Adapt(source interface{}) (interface{}, error)
}

// Proxy 代理接口
type Proxy interface {
	Invoke(ctx context.Context, method string, args ...interface{}) (interface{}, error)
}

// Interceptor 拦截器接口
type Interceptor interface {
	Before(ctx context.Context, req interface{}) error
	After(ctx context.Context, req interface{}, resp interface{}) error
	OnError(ctx context.Context, req interface{}, err error) error
}

// Pipeline 管道接口
type Pipeline interface {
	AddStage(stage PipelineStage) Pipeline
	Execute(ctx context.Context, input interface{}) (interface{}, error)
}

// PipelineStage 管道阶段接口
type PipelineStage interface {
	Process(ctx context.Context, input interface{}) (interface{}, error)
	GetName() string
}

// Aggregator 聚合器接口
type Aggregator interface {
	Add(item interface{}) error
	Aggregate() (interface{}, error)
	Reset() error
	Count() int
}

// Iterator 迭代器接口
type Iterator interface {
	HasNext() bool
	Next() (interface{}, error)
	Reset() error
}

// Collection 集合接口
type Collection interface {
	Add(item interface{}) error
	Remove(item interface{}) error
	Contains(item interface{}) bool
	Size() int
	IsEmpty() bool
	Clear() error
	Iterator() Iterator
}

// Comparable 可比较接口
type Comparable interface {
	CompareTo(other interface{}) int
	Equals(other interface{}) bool
}

// Sortable 可排序接口
type Sortable interface {
	Sort() error
	IsSorted() bool
}

// Searchable 可搜索接口
type Searchable interface {
	Search(query interface{}) ([]interface{}, error)
	SearchOne(query interface{}) (interface{}, error)
}

// Indexable 可索引接口
type Indexable interface {
	CreateIndex(field string) error
	DropIndex(field string) error
	GetIndexes() []string
}

// Compressible 可压缩接口
type Compressible interface {
	Compress() ([]byte, error)
	Decompress(data []byte) error
	GetCompressionRatio() float64
}

// Encryptable 可加密接口
type Encryptable interface {
	Encrypt(data []byte) ([]byte, error)
	Decrypt(data []byte) ([]byte, error)
	GetEncryptionAlgorithm() string
}

// Auditable 可审计接口
type Auditable interface {
	GetAuditLog() []AuditEntry
	AddAuditEntry(entry AuditEntry) error
}

// AuditEntry 审计条目
type AuditEntry struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	User      string                 `json:"user"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource"`
	Details   map[string]interface{} `json:"details"`
	Result    string                 `json:"result"`
}

// Notifiable 可通知接口
type Notifiable interface {
	SendNotification(notification Notification) error
}

// Notification 通知
type Notification struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Severity  string                 `json:"severity"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// Exportable 可导出接口
type Exportable interface {
	Export(format string) ([]byte, error)
	GetSupportedFormats() []string
}

// Importable 可导入接口
type Importable interface {
	Import(data []byte, format string) error
	GetSupportedFormats() []string
}

// Backupable 可备份接口
type Backupable interface {
	Backup() ([]byte, error)
	Restore(data []byte) error
	GetBackupMetadata() map[string]interface{}
}

// Migratable 可迁移接口
type Migratable interface {
	Migrate(fromVersion, toVersion string) error
	GetCurrentVersion() string
	GetSupportedVersions() []string
}

// Testable 可测试接口
type Testable interface {
	RunTests() ([]TestResult, error)
	GetTestSuite() string
}

// TestResult 测试结果
type TestResult struct {
	Name     string                 `json:"name"`
	Status   string                 `json:"status"`
	Duration time.Duration          `json:"duration"`
	Error    string                 `json:"error,omitempty"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// Debuggable 可调试接口
type Debuggable interface {
	EnableDebug(enabled bool)
	IsDebugEnabled() bool
	GetDebugInfo() map[string]interface{}
}

// Traceable 可追踪接口
type Traceable interface {
	StartTrace(name string) TraceSpan
	GetTraceID() string
}

// TraceSpan 追踪片段接口
type TraceSpan interface {
	SetTag(key, value string)
	SetError(err error)
	Finish()
	GetDuration() time.Duration
}

// Discoverable 可发现接口
type Discoverable interface {
	Discover() ([]interface{}, error)
	RegisterDiscoveryHandler(handler DiscoveryHandler) error
}

// DiscoveryHandler 发现处理器接口
type DiscoveryHandler interface {
	OnDiscovered(item interface{}) error
	OnLost(item interface{}) error
}

// Extensible 可扩展接口
type Extensible interface {
	AddExtension(name string, extension interface{}) error
	RemoveExtension(name string) error
	GetExtension(name string) (interface{}, bool)
	ListExtensions() []string
}

// Pluggable 可插拔接口
type Pluggable interface {
	LoadPlugin(path string) error
	UnloadPlugin(name string) error
	GetPlugins() []Plugin
}

// Feature 特性接口
type Feature interface {
	IsEnabled() bool
	Enable() error
	Disable() error
	GetName() string
	GetDescription() string
}

// FeatureFlag 特性标志接口
type FeatureFlag interface {
	IsEnabled(feature string) bool
	Enable(feature string) error
	Disable(feature string) error
	GetFeatures() []Feature
}
