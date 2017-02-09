// Disp provides a simple Job Que and dispatching system. It is based on a blog post
// (http://marcio.io/2015/07/handling-1-million-requests-per-minute-with-golang/)
// The dispatching is kept (coudl be removed see https://gist.github.com/harlow/dbcd639cf8d396a2ab73)
// but as we might move to more advanced cross entity dispatching its still here
package disp

import (
_ "expvar"
"fmt"
_ "net/http/pprof"
"time"
)

// Job holds the attributes needed to perform unit of work.
type Job struct {
	Name  string
	Delay time.Duration
}

type Worker interface {
	start()
	stop()
}

// NewWorker creates takes a numeric id and a channel w/ worker pool.
func NewWorker(id int, workerPool chan chan Job) ExWorker {
	return ExWorker{
		Id:         id,
		JobQueue:   make(chan Job),
		WorkerPool: workerPool,
		QuitChan:   make(chan bool),
	}
}

type ExWorker struct {
	Id         int
	JobQueue   chan Job
	WorkerPool chan chan Job
	QuitChan   chan bool
}

func (w ExWorker) start() {
	go func() {
		for {
			// Add my jobQueue to the worker pool.
			w.WorkerPool <- w.JobQueue
			select {
			case job := <-w.JobQueue:
			// Dispatcher has added a job to my jobQueue.
				fmt.Printf("worker%d: started %s, blocking for %f seconds\n", w.Id, job.Name, job.Delay.Seconds())
				time.Sleep(job.Delay)
				fmt.Printf("worker%d: completed %s!\n", w.Id, job.Name)
			case <-w.QuitChan:
			// We have been asked to stop.
				fmt.Printf("worker%d stopping\n", w.Id)
				return
			}
		}
	}()
}

func (w ExWorker) stop() {
	go func() {
		w.QuitChan <- true
	}()
}

// NewDispatcher creates, and returns a new Dispatcher object.
func NewDispatcher(jobQueue chan Job, maxWorkers int) *Dispatcher {
	workerPool := make(chan chan Job, maxWorkers)

	return &Dispatcher{
		jobQueue:   jobQueue,
		maxWorkers: maxWorkers,
		workerPool: workerPool,
	}
}

type Dispatcher struct {
	workerPool chan chan Job
	maxWorkers int
	jobQueue   chan Job
}

func (d *Dispatcher) Run(makeWorker func(int, chan chan Job)Worker) {
	for i := 0; i < d.maxWorkers; i++ {
		worker := makeWorker(i+1, d.workerPool)
		worker.start()
	}

	go d.dispatch()
}

func (d *Dispatcher) dispatch() {
	for {
		select {
		case job := <-d.jobQueue:
			go func() {
				fmt.Printf("fetching workerJobQueue for: %s\n", job.Name)
				workerJobQueue := <-d.workerPool
				fmt.Printf("adding %s to workerJobQueue\n", job.Name)
				workerJobQueue <- job
			}()
		}
	}
}