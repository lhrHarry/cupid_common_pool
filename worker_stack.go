package cupid_pool

import "time"

// worker 栈 {items: 有效， expire :过期}
type workerStack struct {
	// 有效, 后加 后拿
	items []worker
	// 过期
	expires []worker
}

func newWorkerStack(size int) *workerStack {
	return &workerStack{
		items: make([]worker, 0, size),
	}
}

func (q *workerStack) len() int {
	return len(q.items)
}

func (q *workerStack) isEmpty() bool {
	return len(q.items) == 0
}

func (q *workerStack) insert(w worker) error {
	q.items = append(q.items, w)
	return nil
}

func (q *workerStack) poll() worker {
	l := q.len()
	if l == 0 {
		return nil
	}
	w := q.items[l-1]
	// 由于要维护固定的大小， 这里设置为nil
	q.items[l-1] = nil
	return w
}

/**
expire 每次refresh 清空过期的， 并 从 非过期的切片中， 选出过期的放入 expire
*/
func (q *workerStack) refresh(duration time.Duration) []worker {
	l := q.len()
	if l == 0 {
		return nil
	}

	expireTime := time.Now().Add(-duration)

	index := q.binarySearch(0, l-1, expireTime)

	q.expires = q.expires[:0]

	if index != -1 {
		q.expires = append(q.expires, q.items[:index+1]...)
		// copy的数量
		curIndex := copy(q.items, q.items[index+1:])
		// 移除过期的数据
		for i := curIndex; i < l; i++ {
			q.items[i] = nil
		}
		q.items = q.items[:curIndex]
	}
	return q.expires
}

// 二分搜索 找出第一个比 expireTime 小的
// 双闭区间 取h
func (q *workerStack) binarySearch(l, r int, expireTime time.Time) int {
	var mid int
	for l <= r {
		mid = (l + r) / 2
		if expireTime.Before(q.items[mid].lastUsedTime()) {
			r = mid - 1
		} else {
			l = mid + 1
		}
	}
	return r
}

// 重置worker 队列
func (q *workerStack) reset() {
	for i := 0; i < q.len(); i++ {
		q.items[i].finish()
		q.items[i] = nil
	}
	q.items = q.items[:0]
}
