package sync

import (
	"runtime"
	"sync"
	"sync/atomic"
)

type casLock uint32

const maxBackOff = 16

func (cl *casLock) Lock() {
	backOff := 1
	for !atomic.CompareAndSwapUint32((*uint32)(cl), 0, 1) {
		// 利用指数退避算法
		for i := 0; i < backOff; i++ {
			runtime.Gosched() // 让出当前时间片
		}
		// 进行指数次位计算
		if backOff < maxBackOff {
			backOff <<= 1
		}
	}
}

func (cl *casLock) Unlock() {
	atomic.StoreUint32((*uint32)(cl), 0)
}

func NewCasLock() sync.Locker {
	return new(casLock)
}
