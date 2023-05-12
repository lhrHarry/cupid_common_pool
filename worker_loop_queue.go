package cupid_pool

import "time"

type workerLoopQueue struct {
	// 有效
	items []worker
	// 过期
	expires []worker
	// 头
	head int
	// 尾
	tail int
	// 大小
	size int
	// full
	isFull bool
}

func newWorkerLoopQueue(size int) *workerLoopQueue {
	return &workerLoopQueue{
		items: make([]worker, 0, size),
		size:  size,
	}
}

func (q *workerLoopQueue) len() int {
	if q.size == 0 {
		return 0
	}

	if q.head == q.tail {
		if q.isFull {
			return q.size
		} else {
			return 0
		}
	}

	if q.tail > q.head {
		return q.tail - q.head
	}
	return q.size - q.head + q.tail
}

func (q *workerLoopQueue) isEmpty() bool {
	return q.head == q.tail && !q.isFull
}

func (q *workerLoopQueue) insert(w worker) error {
	if q.size == 0 {
		return ErrorQueueIsEmpty
	}
	if q.isFull {
		return ErrorQueueIsFull
	}

	q.items[q.tail] = w
	q.tail++

	if q.tail == q.size {
		q.tail = 0
	}

	if q.tail == q.head {
		q.isFull = true
	}
	return nil
}

func (q *workerLoopQueue) poll() worker {
	if q.isEmpty() {
		return nil
	}

	w := q.items[q.head]
	q.items[q.head] = nil
	q.head++

	if q.head == q.size {
		q.head = 0
	}
	q.isFull = false
	return w
}

func (q *workerLoopQueue) refresh(duration time.Duration) []worker {
	expireTime := time.Now().Add(-duration)
	// 真实的小于 expireTime的 index
	index := q.binarySearch(expireTime)
	if index == -1 {
		return nil
	}

	q.expires = q.expires[:0]

	if q.head <= index {
		q.expires = append(q.expires, q.items[q.head:index+1]...)
		for i := q.head; i <= index; i++ {
			q.items[i] = nil
		}
	} else {
		q.expires = append(q.expires, q.items[q.head:]...)
		q.expires = append(q.expires, q.items[:index+1]...)
		for i := 0; i <= index; i++ {
			q.items[i] = nil
		}

		for i := q.head; i < q.size; i++ {
			q.items[i] = nil
		}
	}

	q.head = (index + 1) % q.size
	// 存在有过期的元素
	if len(q.expires) > 0 {
		q.isFull = false
	}
	return q.expires
}

func (q *workerLoopQueue) reset() {
	if q.isEmpty() {
		return
	}

	// 循环完成
	w := q.poll()
	for w != nil {
		w.finish()
		w = q.poll()
	}

	// 最后的清理
	q.items = q.items[:0]
	q.head = 0
	q.tail = 0
	q.size = 0
	q.isFull = false
}

func (q *workerLoopQueue) binarySearch(expireTime time.Time) int {
	var mockMid, l, mid int
	nLen := len(q.items)

	// 空 | 第一个元素没有过期， 返回 -1
	if q.isEmpty() || expireTime.Before(q.items[q.head].lastUsedTime()) {
		return -1
	}

	/**
	nLen = 8, head = 0  t = 4  --> (4-1-0+8) % 8 = 3 --> r = 3
	[2 ,3, 4, 5, nil, nil, nil, nil]
	 0  1  2  3	  4    5    6    7
	 h            t
	[2 ,3, 4, 5, nil, nil, nil, nil]
	 l        r

	nLen = 8, head = 7  t = 4  --> (4-1-7+8) % 8 = 4 --> r = 4
	[2 ,3, 4, 5, nil, nil, nil, 1]
	 0  1  2  3	  4    5    6   7
	              t             1
	[2 ,3, 4, 5, nil, nil, nil, 1]
	          r                 l
	*/
	// 真实的 高位 位置 (在 mockL=0时)
	mockH := (q.tail - 1 - q.head + nLen) % nLen
	l = q.head
	mockL := 0

	for mockL <= mockH {
		// 虚拟的 中间位置
		mockMid = mockL + (mockH-mockL)>>1
		// 真实的 中间位置
		mid = (mockMid + l + nLen) % nLen

		if expireTime.Before(q.items[mid].lastUsedTime()) {
			mockH = mockMid - 1
		} else {
			mockL = mockMid + 1
		}
	}
	// 返回真实的 index
	return (mockH + l + nLen) % nLen
}
