package cupid_pool

import (
	"runtime/debug"
	"time"
)

type goWorkerWithFunc struct {
	pool *PoolWithFunc

	args chan interface{}

	lastUsed time.Time
}

func (gw *goWorkerWithFunc) run() {
	gw.pool.addRunning(1)
	go func() {
		defer func() {
			gw.pool.addRunning(-1)
			gw.pool.workCache.Put(gw)
			if p := recover(); p != nil {
				if ph := gw.pool.options.PanicHandler; ph != nil {
					ph(p)
				} else {
					gw.pool.options.Logger.Printf("worker exit for panic:%v\n%s", p, debug.Stack())
				}
			}
			gw.pool.cond.Signal()
		}()

		for arg := range gw.args {
			if arg == nil {
				return
			}
			gw.pool.poolFunc(arg)
			// 归还 go 池 失败
			if ok := gw.pool.revertWorker(gw); !ok {
				return
			}
		}
	}()
}

func (gw *goWorkerWithFunc) finish() {
	gw.args <- nil
}

func (gw *goWorkerWithFunc) lastUsedTime() time.Time {
	return gw.lastUsed
}

func (gw *goWorkerWithFunc) inputFunc(_ func()) {
	panic("unreachable")
}

func (gw *goWorkerWithFunc) inputParam(arg interface{}) {
	gw.args <- arg
}
