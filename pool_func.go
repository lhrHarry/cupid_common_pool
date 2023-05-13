package cupid_pool

import (
	"context"
	syncx "github.com/lhrHarry/cupid_common_pool/internal/sync"
	"sync"
	"sync/atomic"
	"time"
)

// PoolWithFunc 带方法的，特定方法的 go 池
type PoolWithFunc struct {
	capacity  int32
	running   int32
	lock      sync.Locker
	workers   workerQueue
	state     int32
	cond      *sync.Cond
	poolFunc  func(interface{})
	workCache sync.Pool
	waiting   int32
	purgeDone int32
	stopPurge context.CancelFunc

	ticktockDone int32
	stopTicktock context.CancelFunc

	now     atomic.Value
	options *Options
}

func (p *PoolWithFunc) Cap() int {
	return int(atomic.LoadInt32(&p.capacity))
}

func (p *PoolWithFunc) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

func (p *PoolWithFunc) IsClosed() bool {
	return atomic.LoadInt32(&p.state) == CLOSE
}

func (p *PoolWithFunc) Waiting() int {
	return int(atomic.LoadInt32(&p.waiting))
}

func (p *PoolWithFunc) Invoke(args interface{}) error {
	if p.IsClosed() {
		return ErrPoolClosed
	}
	if w := p.retrieveWorker(); w != nil {
		w.inputParam(args)
		return nil
	}
	return ErrPoolOverload
}

func (p *PoolWithFunc) Available() int {
	c := p.Cap()
	if c < 0 {
		return -1
	}
	return c - p.Running()
}

func (p *PoolWithFunc) Reboot() {
	if !atomic.CompareAndSwapInt32(&p.state, CLOSE, OPENED) {
		return
	}

	atomic.StoreInt32(&p.purgeDone, 0)
	p.goPurge()
	atomic.StoreInt32(&p.ticktockDone, 0)
	p.goTicktock()
}

func (p *PoolWithFunc) Release() {
	if !atomic.CompareAndSwapInt32(&p.state, OPENED, CLOSE) {
		return
	}
	p.lock.Lock()
	p.workers.reset()
	p.lock.Unlock()
	p.cond.Broadcast()

	if p.stopPurge != nil {
		p.stopPurge()
		p.stopPurge = nil
	}
	if p.stopTicktock != nil {
		p.stopTicktock()
		p.stopTicktock = nil
	}
}

func (p *PoolWithFunc) ReleaseTimeout(timeout time.Duration) error {
	if p.IsClosed() || (!p.options.DisablePurge && p.stopPurge == nil) {
		return ErrPoolClosed
	}
	p.Release()

	endTime := time.Now().Add(timeout)

	for time.Now().Before(endTime) {
		if p.Running() == 0 &&
			(p.options.DisablePurge || atomic.LoadInt32(&p.purgeDone) == 1) &&
			atomic.LoadInt32(&p.ticktockDone) == 1 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ErrTimeout
}

func NewPoolWithFunc(size int, pf func(interface{}), options ...Option) (*PoolWithFunc, error) {
	if size <= 0 {
		return nil, ErrInvalidPreAllocSize
	}

	if pf != nil {
		return nil, ErrLackPoolFunc
	}

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

	p := &PoolWithFunc{
		capacity: int32(size),
		poolFunc: pf,
		lock:     syncx.NewCasLock(),
		options:  opts,
	}

	p.workCache.New = func() any {
		return &goWorkerWithFunc{
			pool: p,
			args: make(chan interface{}, workerChanCap),
		}
	}

	if p.options.PreAlloc {
		p.workers = newWorkerArray(queueTypeLoopQueue, size)
	} else {
		p.workers = newWorkerArray(queueTypeStack, size)
	}

	p.cond = sync.NewCond(p.lock)

	p.goPurge()
	p.goTicktock()

	return p, nil
}

// ------------------------- 私有 -------------------------->
func (p *PoolWithFunc) addRunning(delta int32) {
	atomic.AddInt32(&p.running, delta)
}

func (p *PoolWithFunc) revertWorker(gw *goWorkerWithFunc) bool {
	// isclose? running > cap
	if capacity := p.Cap(); (capacity > 0 && p.Running() > capacity) || p.IsClosed() {
		p.cond.Broadcast()
		return false
	}
	gw.lastUsed = p.nowTime()

	p.lock.Lock()
	if p.IsClosed() {
		p.lock.Unlock()
		return false
	}
	if err := p.workers.insert(gw); err != nil {
		p.lock.Unlock()
		return false
	}

	p.cond.Signal()
	p.lock.Unlock()
	return true
}

func (p *PoolWithFunc) nowTime() time.Time {
	return p.now.Load().(time.Time)
}

func (p *PoolWithFunc) goPurge() {
	if p.options.DisablePurge {
		return
	}

	var ctx context.Context
	ctx, p.stopPurge = context.WithCancel(context.Background())
	go p.purgeExpireWorkers(ctx)
}

func (p *PoolWithFunc) goTicktock() {
	p.now.Store(time.Now())
	var ctx context.Context
	ctx, p.stopTicktock = context.WithCancel(context.Background())
	go p.ticktock(ctx)
}

func (p *PoolWithFunc) purgeExpireWorkers(ctx context.Context) {
	ticker := time.NewTicker(p.options.ExpireDuration)
	defer func() {
		ticker.Stop()
		atomic.StoreInt32(&p.purgeDone, 1)
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

		var isDormant bool
		p.lock.Lock()
		expireWorkers := p.workers.refresh(p.options.ExpireDuration)
		n := p.Running()
		isDormant = n == 0 || len(expireWorkers) == n
		p.lock.Unlock()

		for i, worker := range expireWorkers {
			worker.finish()
			expireWorkers[i] = nil
		}

		if isDormant && p.Waiting() > 0 {
			p.cond.Broadcast()
		}
	}
}

func (p *PoolWithFunc) ticktock(ctx context.Context) {
	ticker := time.NewTicker(nowTimeUpdateInterval)
	defer func() {
		ticker.Stop()
		atomic.StoreInt32(&p.ticktockDone, 1)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if p.IsClosed() {
			return
		}
		p.now.Store(time.Now())
	}
}

func (p *PoolWithFunc) retrieveWorker() (w worker) {
	spawnWorker := func() {
		w = p.workCache.Get().(*goWorkerWithFunc)
		w.run()
	}

	p.lock.Lock()
	w = p.workers.poll()
	if w != nil {
		return
	} else if capacity := p.Cap(); capacity == -1 || capacity > p.Running() {
		p.lock.Unlock()
		spawnWorker()
	} else {
		if p.options.NonBlocking {
			p.lock.Unlock()
			p.options.Logger.Printf("non blocking!!!!")
			return
		}
	retry:
		if p.options.MaxBlockingTasks != 0 && p.Waiting() >= p.options.MaxBlockingTasks {
			p.lock.Unlock()
			return
		}

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

func (p *PoolWithFunc) addWaiting(delta int32) {
	atomic.AddInt32(&p.waiting, delta)
}
