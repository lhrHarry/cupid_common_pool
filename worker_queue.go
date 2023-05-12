package cupid_pool

import (
	"errors"
)

const (
	queueTypeStack queueType = 1 << iota
	queueTypeLoopQueue
)

var (
	// ErrorQueueIsFull 队列已满已经满了
	ErrorQueueIsFull = errors.New("the queue is null")

	// ErrorQueueIsEmpty 队列长度为空
	ErrorQueueIsEmpty = errors.New("the queue length is zero")
)

// 定义 队列类型 （1，2）  stack队列 环形队列
type queueType int

func newWorkerArray(qType queueType, size int) workerQueue {
	switch qType {
	case queueTypeStack:
		return newWorkerStack(size)
	case queueTypeLoopQueue:
		return newWorkerLoopQueue(size)
	default:
		return newWorkerStack(size)
	}
}
