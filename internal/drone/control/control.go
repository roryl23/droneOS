package control

import (
	"container/list"
	"sync"
)

type PriorityMutex struct {
	mu        sync.Mutex
	isLocked  bool
	waitQueue map[int]*list.List // Priority level to list of channels
}

func NewPriorityMutex() *PriorityMutex {
	return &PriorityMutex{
		waitQueue: make(map[int]*list.List),
	}
}

func (pm *PriorityMutex) Lock(priority int) {
	pm.mu.Lock()
	if !pm.isLocked {
		pm.isLocked = true
		pm.mu.Unlock()
		return
	}

	// Create a channel for this goroutine to wait on
	ch := make(chan struct{})
	if pm.waitQueue[priority] == nil {
		pm.waitQueue[priority] = list.New()
	}
	// Add the channel to the wait queue for the given priority
	pm.waitQueue[priority].PushBack(ch)
	pm.mu.Unlock()

	// Wait on the channel
	<-ch
}

func (pm *PriorityMutex) Unlock() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Release the lock
	pm.isLocked = false

	// Find the highest priority level with waiting goroutines
	for priority := 1; priority <= 10; priority++ {
		if queue, ok := pm.waitQueue[priority]; ok && queue.Len() > 0 {
			// Remove the first goroutine in the queue
			elem := queue.Front()
			ch := elem.Value.(chan struct{})
			queue.Remove(elem)
			if queue.Len() == 0 {
				delete(pm.waitQueue, priority)
			}
			// Lock is now held by the waiting goroutine
			pm.isLocked = true
			// Signal the waiting goroutine
			close(ch)
			return
		}
	}
}
