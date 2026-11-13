package dial

import (
	"container/heap"
	"context"
	"sync"

	"github.com/innovativebioresearch/innbc-chain/network/common"

	"github.com/libp2p/go-libp2p/core/peer"
)

// DialQueue is a queue that holds dials tasks for potential peers, implemented as a min-heap
type DialQueue struct {
	sync.Mutex

	heap  dialQueueImpl
	tasks map[peer.ID]*DialTask

	/* START ALPHA CHANGE 2772 -- retain one pending reconnect while a peer has a dial owner -- */
	inFlight map[peer.ID]*DialTask
	pending  map[peer.ID]*DialTask
	/* FINISH ALPHA CHANGE 2772 -- retain one pending reconnect while a peer has a dial owner -- */

	updateCh chan struct{}
	closeCh  chan struct{}

	/* START ALPHA CHANGE 1138 -- make dial queue close idempotent -- */
	closeOnce sync.Once
	/* FINISH ALPHA CHANGE 1138 -- make dial queue close idempotent -- */
}

// NewDialQueue creates a new DialQueue instance
/* START ALPHA CHANGE 2773 -- initialize dial ownership and pending reconnect maps -- */
func NewDialQueue() *DialQueue {
	return &DialQueue{
		heap:     dialQueueImpl{},
		tasks:    map[peer.ID]*DialTask{},
		inFlight: map[peer.ID]*DialTask{},
		pending:  map[peer.ID]*DialTask{},
		updateCh: make(chan struct{}, 1),
		closeCh:  make(chan struct{}),
	}
}

/* FINISH ALPHA CHANGE 2773 -- initialize dial ownership and pending reconnect maps -- */

// Close closes the running DialQueue
func (d *DialQueue) Close() {
	/* START ALPHA CHANGE 1139 -- guard dial queue close channel -- */
	d.closeOnce.Do(func() {
		close(d.closeCh)
	})
	/* FINISH ALPHA CHANGE 1139 -- guard dial queue close channel -- */
}

// Wait waits for closing or updating event or end of context.
// Returns true for closing event or end of the context [BLOCKING].
func (d *DialQueue) Wait(ctx context.Context) bool {
	select {
	case <-ctx.Done(): // blocks until context is done ...
		return true
	case <-d.updateCh: // ... or AddTask is called ...
		return false
	case <-d.closeCh: // ... or dial queue is closed
		return true
	}
}

// PopTask is the implementation for task popping from the min-heap
func (d *DialQueue) PopTask() *DialTask {
	d.Lock()
	defer d.Unlock()

	if len(d.heap) != 0 {
		// pop the first value and remove it from the heap
		task, ok := heap.Pop(&d.heap).(*DialTask)
		if !ok {
			return nil
		}

		delete(d.tasks, task.addrInfo.ID)

		return task
	}

	return nil
}

/* START ALPHA CHANGE 2774 -- reserve dial ownership atomically and resume pending work after release -- */

/* PopDialTask removes a runnable task and reserves its peer until FinishDial. */
func (d *DialQueue) PopDialTask() *DialTask {
	d.Lock()
	defer d.Unlock()

	select {
	case <-d.closeCh:
		return nil
	default:
	}

	if len(d.heap) == 0 {
		return nil
	}

	task, ok := heap.Pop(&d.heap).(*DialTask)
	if !ok {
		return nil
	}

	delete(d.tasks, task.addrInfo.ID)
	d.inFlight[task.addrInfo.ID] = task

	return task
}

/* FinishDial releases only the supplied owner and makes its pending request runnable. */
func (d *DialQueue) FinishDial(task *DialTask) {
	if task == nil {
		return
	}

	d.Lock()

	id := task.addrInfo.ID
	if d.inFlight[id] != task {
		d.Unlock()

		return
	}

	delete(d.inFlight, id)
	next := d.pending[id]
	delete(d.pending, id)

	select {
	case <-d.closeCh:
		d.Unlock()

		return
	default:
	}

	if next != nil {
		d.tasks[id] = next
		heap.Push(&d.heap, next)
	}

	d.Unlock()

	if next != nil {
		select {
		case d.updateCh <- struct{}{}:
		default:
		}
	}
}

/* FINISH ALPHA CHANGE 2774 -- reserve dial ownership atomically and resume pending work after release -- */

/* START ALPHA CHANGE 2775 -- delete deferred reconnect requests with queued tasks -- */
// DeleteTask deletes a task from the dial queue for the specified peer
func (d *DialQueue) DeleteTask(peer peer.ID) {
	d.Lock()
	defer d.Unlock()

	delete(d.pending, peer)

	item, ok := d.tasks[peer]
	if ok {
		heap.Remove(&d.heap, item.index)
		delete(d.tasks, peer)
	}
}

/* FINISH ALPHA CHANGE 2775 -- delete deferred reconnect requests with queued tasks -- */

// AddTask adds a new task to the dial queue
func (d *DialQueue) AddTask(addrInfo *peer.AddrInfo, priority common.DialPriority) {
	if d.addTaskImpl(addrInfo, priority) {
		select {
		case d.updateCh <- struct{}{}:
		default:
		}
	}
}

/* START ALPHA CHANGE 2776 -- coalesce busy peer requests and retain the latest supplied address -- */
func (d *DialQueue) addTaskImpl(addrInfo *peer.AddrInfo, priority common.DialPriority) bool {
	d.Lock()
	defer d.Unlock()

	select {
	case <-d.closeCh:
		return false
	default:
	}

	if _, busy := d.inFlight[addrInfo.ID]; busy {
		if item := d.pending[addrInfo.ID]; item != nil {
			item.addrInfo = addrInfo
			if item.priority > uint64(priority) {
				item.priority = uint64(priority)
			}
		} else {
			d.pending[addrInfo.ID] = &DialTask{
				addrInfo: addrInfo,
				priority: uint64(priority),
			}
		}

		/* Ownership release will notify the dial loop when this becomes runnable. */
		return false
	}

	if item, ok := d.tasks[addrInfo.ID]; ok {
		/* Address freshness is independent of whether priority improves. */
		item.addrInfo = addrInfo
		if item.priority > uint64(priority) {
			item.priority = uint64(priority)
			heap.Fix(&d.heap, item.index)

			return true
		}

		return false
	}

	task := &DialTask{
		addrInfo: addrInfo,
		priority: uint64(priority),
	}
	d.tasks[addrInfo.ID] = task
	heap.Push(&d.heap, task)

	return true
}

/* FINISH ALPHA CHANGE 2776 -- coalesce busy peer requests and retain the latest supplied address -- */
