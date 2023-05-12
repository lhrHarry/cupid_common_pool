package cupid_pool

import (
	"context"
	syncx "github.com/lhrHarry/cupid_common_pool/internal/sync"
	"sync"
	"sync/atomic"
	"time"
)

type Pool struct {
	// 容量
	capacity int32

	// 当前运行的goroutines数量
	running int32

	// 保护工作队列
	lock sync.Locker

	// 工作队列
	workers workerQueue

	// 用于通知 协程 关闭
	state int32

	// 为了等待获取空闲worker 的条件
	cond *sync.Cond

	// 加速获取没有工作的worker，检索
	workerCache sync.Pool

	// 正在等待的goroutines 数量，已经被blocked 在池子中 on submit, 被pool.lock保护
	waiting int32

	// 清理的状态，结束 定期清理完成之后会设置为1
	purgeDown int32

	// 上下文取消, 可以手动调用上下文取消的方法，来终止purge的协程
	stopPurge context.CancelFunc

	ticktockDown int32
	stopTicktock context.CancelFunc

	// 当前时间
	now atomic.Value

	// 配置
	options *Options
}

func (p *Pool) Submit(task func()) error {
	if p.IsClosed() {
		return ErrPoolClosed
	}
	if w := p.retrieveWorker(); w != nil {
		w.inputFunc(task)
		return nil
	}
	return ErrPoolOverload
}

// Available 返回可用的数量 容量 - 运行数 = 可用数
func (p *Pool) Available() int {
	c := p.Cap()
	if c < 0 {
		return -1
	}
	return c - p.Running()
}

func (p *Pool) Cap() int {
	return int(atomic.LoadInt32(&p.capacity))
}

func (p *Pool) Waiting() int {
	return int(atomic.LoadInt32(&p.waiting))
}

func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

func NewPool(size int, options ...Option) (*Pool, error) {
	if size <= 0 {
		size = -1
	}

	// 加载options
	opts := loadOptions(options...)

	if !opts.DisablePurge {
		if expire := opts.ExpireDuration; expire < 0 {
			return nil, ErrInvalidPoolExpiry
		} else if expire == 0 {
			opts.ExpireDuration = DefaultCleanIntervalTime
		}
	}

	if opts.Logger == nil {
		opts.Logger = defaultLogger
	}

	p := &Pool{
		capacity: int32(size),
		lock:     syncx.NewCasLock(), // 自定义的 cas lock
		options:  opts,
	}

	p.workerCache.New = func() any {
		return &goWorker{
			pool: p,
			task: make(chan func(), workerChanCap),
		}
	}

	if p.options.PreAlloc {
		if size == -1 {
			return nil, ErrInvalidPreAllocSize
		}
		p.workers = newWorkerArray(queueTypeLoopQueue, size)
	} else {
		p.workers = newWorkerArray(queueTypeStack, 0)
	}

	p.cond = sync.NewCond(p.lock)

	p.goPurge()
	p.goTicktock()

	return p, nil
}

// Release 释放池
func (p *Pool) Release() {
	if !atomic.CompareAndSwapInt32(&p.state, OPENED, CLOSE) {
		return
	}
	p.lock.Lock()
	p.workers.reset()
	p.lock.Unlock()
	// 唤醒 等待 go 程
	p.cond.Broadcast()

	// 停止定时任务
	p.stopPurge()
	p.stopTicktock()
}

func (p *Pool) Reboot() {
	if !atomic.CompareAndSwapInt32(&p.state, CLOSE, OPENED) {
		return
	}
	atomic.StoreInt32(&p.purgeDown, 0)
	p.goPurge()
	atomic.StoreInt32(&p.ticktockDown, 0)
	p.goTicktock()

}

// ----------------------------------- 私有方法 ---------------------------------------->

/*  执行过期清理方法 */
func (p *Pool) goPurge() {
	if p.options.DisablePurge {
		return
	}

	// 开启一个 协程 来周期性的清理过期worker
	var ctx context.Context
	ctx, p.stopPurge = context.WithCancel(context.Background())
	go p.purgeExpiredWorkers(ctx)
}

/* 执行时钟 */
func (p *Pool) goTicktock() {
	p.now.Store(time.Now())
	var ctx context.Context
	ctx, p.stopTicktock = context.WithCancel(context.Background())
	go p.ticktock(ctx)
}

/* 清理过期的workers */
func (p *Pool) purgeExpiredWorkers(ctx context.Context) {
	ticker := time.NewTicker(p.options.ExpireDuration)

	defer func() {
		ticker.Stop()
		atomic.StoreInt32(&p.purgeDown, 1)
	}()

	for {
		select {
		// 收到取消消息
		case <-ctx.Done():
			return
		case <-ticker.C: // 收到定时消息递送
		}
		if p.IsClosed() {
			break
		}
		// 记录是否休眠
		var isDormant bool
		p.lock.Lock() // cas 锁
		expiredWorkers := p.workers.refresh(p.options.ExpireDuration)
		n := p.Running()
		isDormant = n == 0 || n == len(expiredWorkers) // 正在运行的worker数量 为0 | 全部在运行的已经过期
		p.lock.Unlock()

		// 通知过时的工人停止。
		// 这个通知必须在 p.lock 之外，因为 w.task 可能会阻塞，如果有很多worker，可能会消耗大量时间 位于非本地 CPU 上。
		for i, w := range expiredWorkers {
			w.finish()
			expiredWorkers[i] = nil
		}

		// 可能会出现所有worker都被清理干净的情况（没有worker在运行），
		// 当一些调用者仍然停留在“p.cond.Wait()”中时，我们需要唤醒这些调用者。
		if isDormant && p.Waiting() > 0 {
			p.cond.Broadcast()
		}
	}
}

func (p *Pool) IsClosed() bool {
	return atomic.LoadInt32(&p.state) == CLOSE
}

/* 定期更新协程池中的时钟 */
func (p *Pool) ticktock(ctx context.Context) {
	ticker := time.NewTicker(nowTimeUpdateInterval)
	defer func() {
		ticker.Stop()
		atomic.StoreInt32(&p.ticktockDown, 1)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if p.IsClosed() {
			break
		}
		p.now.Store(time.Now())
	}
}

func (p *Pool) retrieveWorker() (w worker) {
	spawnWorker := func() {
		w = p.workerCache.Get().(*goWorker)
		w.run()
	}

	// 加锁获取，，保证worker不会被 refresh
	p.lock.Lock()
	w = p.workers.poll()
	if w != nil {
		p.lock.Unlock() // 拿到之后，立即解锁
	} else if capacity := p.Cap(); capacity == -1 || capacity > p.Running() {
		// 如果容量为空， 并且 容量还没满，直接进行 原始创建
		p.lock.Unlock()
		spawnWorker()
	} else { // 其他情况下 ，我们必须保证他们阻塞，等待至少一个 worker 被放入池中
		if p.options.NonBlocking {
			// 不阻塞，直接丢弃任务
			return
		}
	retry:
		if p.options.MaxBlockingTasks != 0 && p.Waiting() >= p.options.MaxBlockingTasks {
			// 超出队列，直接丢弃任务
			p.lock.Unlock()
			return
		}
		// 条件队列没有满, 挂起，之后，释放线程资源
		p.addWaiting(1)
		p.cond.Wait()
		p.addWaiting(-1)

		if p.IsClosed() {
			p.lock.Unlock()
			return
		}

		if w = p.workers.poll(); w == nil {
			if p.Available() > 0 {
				p.lock.Unlock()
				spawnWorker()
				return
			}
			goto retry
		}
		p.lock.Unlock()
	}
	return
}

func (p *Pool) addWaiting(delta int32) {
	atomic.AddInt32(&p.waiting, delta)
}

func (p *Pool) addRunning(delta int32) {
	atomic.AddInt32(&p.running, delta)
}

// 将一个 worker 放回空闲池中
func (p *Pool) revertWorker(w *goWorker) bool {
	if capacity := p.Cap(); (capacity > 0 && p.Running() > capacity) || p.IsClosed() {
		p.cond.Broadcast() // 唤醒条件队列
		return false
	}

	w.lastUsed = p.nowTime()

	// 放入空闲池中
	p.lock.Lock()
	if p.IsClosed() {
		p.lock.Unlock()
		return false
	}

	if err := p.workers.insert(w); err != nil {
		p.lock.Unlock()
		return false
	}

	p.cond.Signal()
	p.lock.Unlock()
	return true
}

func (p *Pool) nowTime() time.Time {
	return p.now.Load().(time.Time)
}
