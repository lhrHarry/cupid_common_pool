package cupid_pool

import (
	"time"
)

// Option 定义链式方法， 填充 Options对象
type Option func(opts *Options)

// Options 定义协程池中的一些可配置属性
type Options struct {
	// ExpireDuration 扫描goroutines的时间周期
	// 过期的worker将被回收
	ExpireDuration time.Duration

	// PreAlloc 是否提前分配空间
	PreAlloc bool

	// MaxBlockingTasks 最大的 g 阻塞 在协程池中
	MaxBlockingTasks int

	// NonBlocking true: submit时不会被阻塞住， 0:不会被阻塞住
	NonBlocking bool

	// PanicHandler 发生panic时 的处理器
	PanicHandler func(interface{})

	// 内部logger
	Logger Logger

	// DisablePurge 是否禁用清理， true:worker将不会被清理
	DisablePurge bool
}

func loadOptions(options ...Option) *Options {
	opts := &Options{}
	for _, option := range options {
		option(opts)
	}
	return opts
}

func WithOptions(options *Options) Option {
	return func(opts *Options) {
		opts = options
	}
}

func WithExpireDuration(expireDuration time.Duration) Option {
	return func(opts *Options) {
		opts.ExpireDuration = expireDuration
	}
}

func WithPreAlloc(preAlloc bool) Option {
	return func(opts *Options) {
		opts.PreAlloc = preAlloc
	}
}

func WithMaxBlockingTasks(maxBlockingTasks int) Option {
	return func(opts *Options) {
		opts.MaxBlockingTasks = maxBlockingTasks
	}
}

func WithPanicHandler(panicHandler func(interface{})) Option {
	return func(opts *Options) {
		opts.PanicHandler = panicHandler
	}
}

func WithLogger(logger Logger) Option {
	return func(opts *Options) {
		opts.Logger = logger
	}
}

func WithDisablePurge(disable bool) Option {
	return func(opts *Options) {
		opts.DisablePurge = disable
	}
}
