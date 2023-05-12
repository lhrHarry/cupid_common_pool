package cupid_pool

import (
	"runtime/debug"
	"time"
)

type goWorker struct {
	// 协程池
	pool *Pool

	// 应该执行的任务 通道
	task chan func()

	// 最后一次使用时间，， 在放入队列时更新
	lastUsed time.Time
}

func (gw *goWorker) run() {
	gw.pool.addRunning(1)
	// go func
	go func() {
		// recover, 退出时，进行处理
		defer func() {
			gw.pool.addRunning(-1)
			gw.pool.workerCache.Put(gw)
			if p := recover(); p != nil {
				if ph := gw.pool.options.PanicHandler; ph != nil {
					ph(p)
				} else {
					gw.pool.options.Logger.Printf("worker exist form panic: %v\n%s\n", p, debug.Stack())
				}
			}
			// 退出一个，释放一个型号， 让条件队列中的 go程 进行唤醒
			gw.pool.cond.Signal()
		}()

		for f := range gw.task {
			if f == nil { // finish 消息
				return
			}
			f()                                      // 任务
			if ok := gw.pool.revertWorker(gw); !ok { // 执行完成 放入 pool
				return
			}
		}
	}()
}

// 向worker中 输入 nil 表示完成工作
func (gw *goWorker) finish() {
	gw.task <- nil
}

func (gw *goWorker) lastUsedTime() time.Time {
	return gw.lastUsed
}

func (gw *goWorker) inputFunc(fn func()) {
	gw.task <- fn
}

func (gw *goWorker) inputParam(i interface{}) {
	panic("not implement it")
}
