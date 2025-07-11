package pool

import "sync"

// WorkerPool manages a pool of goroutines to perform tasks.
type WorkerPool struct {
	tasks chan func()
	wg    sync.WaitGroup
}

// New creates a new worker pool with a specified number of workers.
func New(numWorkers int, taskQueueSize int) *WorkerPool {
	p := &WorkerPool{
		tasks: make(chan func(), taskQueueSize),
	}

	p.wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go p.worker()
	}

	return p
}

// worker is a goroutine that processes tasks from the tasks channel.
func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		task()
	}
}

// Submit adds a task to the worker pool.
func (p *WorkerPool) Submit(task func()) {
	p.tasks <- task
}

// Stop waits for all tasks to be submitted and then stops the workers.
func (p *WorkerPool) Stop() {
	close(p.tasks)
	p.wg.Wait()
}
