package cupid_pool

import (
	"errors"
	"log"
	"math"
	"os"
	"runtime"
	"time"
)

/**
outer pool face
const:
	DefaultCupidPoolSize: 默认携程池大小
	DefaultCleanIntervalTime: 默认清理时间
*/

const (
	// DefaultCupidPoolSize is the default capacity for default goroutines pool
	DefaultCupidPoolSize = math.MaxInt32

	// DefaultCleanIntervalTime is the interval time to clean up goroutines
	DefaultCleanIntervalTime = time.Second
)

// 池状态
const (
	// OPENED represent that the pool is opened
	OPENED = iota

	// CLOSE represent that the pool is close
	CLOSE
)

// 0.5 s
const nowTimeUpdateInterval = 500 * time.Millisecond

var (
	// ErrLackPoolFunc 线程池中 缺少方法
	ErrLackPoolFunc = errors.New("must provide function for pool")

	// ErrInvalidPoolExpiry 无效的协程池过期
	ErrInvalidPoolExpiry = errors.New("invalid pool expire")

	ErrInvalidPreAllocSize = errors.New("invalid pre alloc size")

	// ErrPoolClosed 池 已被关闭
	ErrPoolClosed = errors.New("pool has been closed")

	// ErrPoolOverload 池已超载
	ErrPoolOverload = errors.New("pool has been overload")

	// ErrTimeout 超时
	ErrTimeout = errors.New("release time out")

	// defaultAntsPool , os.Stderr标准的错误输出， 日志打印样式
	defaultLogger = Logger(log.New(os.Stderr, "[cupid_pool]:::", log.LstdFlags|log.Lmicroseconds))

	// 默认的
	defaultCupidPool, _ = NewPool(DefaultCupidPoolSize)

	// workerChannel 的通道数
	workerChanCap = func() int {
		// 单核心， 使用阻塞 channel
		if runtime.GOMAXPROCS(0) == 1 {
			return 0
		}
		// 多核心 ， 使用非阻塞channel
		return 1
	}()
)

type Logger interface {
	Printf(format string, args ...interface{})
}

// Submit 提交任务 不返回值
func Submit(task func()) error {
	return defaultCupidPool.Submit(task)
}

// Running 返回执行正在执行的 goroutines
func Running() int {
	return defaultCupidPool.Running()
}

// Cap 线程池 容量
func Cap() int {
	return defaultCupidPool.Cap()
}

// Available 可用的线程池
func Available() int {
	return defaultCupidPool.Available()
}

// Release 释放线程池
func Release() {
	defaultCupidPool.Release()
}

// Reboot 重启线程池
func Reboot() {
	defaultCupidPool.Reboot()
}
