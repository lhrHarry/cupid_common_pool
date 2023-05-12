package cupid_pool

import "time"

// 定义 worker的接口
type worker interface {
	run()
	finish()
	lastUsedTime() time.Time
	inputFunc(func())
	inputParam(interface{})
}

// 定义 worker queue 的接口
type workerQueue interface {
	len() int
	isEmpty() bool
	// 插入worker
	insert(worker) error
	// 拿出worker
	poll() worker
	// 清理过期的worker 返回
	refresh(duration time.Duration) []worker
	// 重制
	reset()
}
