// Package queue declares jobs and broadcasts and the Queue a driver carries them on.
package queue

import (
	"context"

	"altalune.id/oagent/worker"
)

// Queue submits jobs and broadcasts and builds the workers that run their handlers and listeners.
type Queue interface {
	// Declare records the jobs Submit accepts and the broadcasts Emit accepts, refusing an invalid or duplicated one.
	Declare(jobs []Job, broadcasts []Broadcast) error
	// Submit publishes data as one run of job j, waiting for the driver's ack.
	Submit(ctx context.Context, j Job, data any) error
	// Emit publishes data to every running instance, waiting for the driver's ack.
	Emit(ctx context.Context, b Broadcast, data any) error
	// Consumer builds the worker that runs every handler.
	Consumer(ctx context.Context, hs []Handler) (worker.Worker, error)
	// Listener builds the worker that delivers the broadcasts emitted from now on to their listeners.
	Listener(ctx context.Context, ls []Listener) (worker.Worker, error)
}

// Declared is the set of jobs and broadcasts a queue accepts; the zero value accepts none.
type Declared struct {
	jobs       map[Job]struct{}
	broadcasts map[Broadcast]struct{}
}

// NewDeclared validates jobs and broadcasts and refuses a duplicated one.
func NewDeclared(jobs []Job, broadcasts []Broadcast) (*Declared, error) {
	jobSet, jobDup, err := collect(jobs)
	if err != nil {
		return nil, err
	}
	broadcastSet, broadcastDup, err := collect(broadcasts)
	if err != nil {
		return nil, err
	}
	if err := wiringError(append(jobDup, broadcastDup...)); err != nil {
		return nil, err
	}
	return &Declared{jobs: jobSet, broadcasts: broadcastSet}, nil
}

// HasJob reports whether j was declared.
func (d *Declared) HasJob(j Job) bool {
	_, ok := d.jobs[j]
	return ok
}

// HasBroadcast reports whether b was declared.
func (d *Declared) HasBroadcast(b Broadcast) bool {
	_, ok := d.broadcasts[b]
	return ok
}

// CheckHandlers refuses a handler without Handle, an invalid job or a job handled twice.
func CheckHandlers(hs []Handler) error {
	jobs := make([]Job, 0, len(hs))
	for _, h := range hs {
		jobs = append(jobs, h.Job)
	}
	return checkWiring(jobs, func(i int) bool { return hs[i].Handle != nil })
}

// CheckListeners refuses a listener without Handle, an invalid broadcast or a broadcast listened to twice.
func CheckListeners(ls []Listener) error {
	broadcasts := make([]Broadcast, 0, len(ls))
	for _, l := range ls {
		broadcasts = append(broadcasts, l.Broadcast)
	}
	return checkWiring(broadcasts, func(i int) bool { return ls[i].Handle != nil })
}

type declarable interface {
	comparable
	Validate() error
	Subject() string
}

func collect[T declarable](items []T) (set map[T]struct{}, dup []string, err error) {
	set = make(map[T]struct{}, len(items))
	for _, it := range items {
		if err := it.Validate(); err != nil {
			return nil, nil, err
		}
		if _, seen := set[it]; seen {
			dup = append(dup, it.Subject())
		}
		set[it] = struct{}{}
	}
	return set, dup, nil
}

func checkWiring[T declarable](items []T, hasHandle func(i int) bool) error {
	for i, it := range items {
		if !hasHandle(i) {
			return &NilHandlerError{Subject: it.Subject()}
		}
	}
	_, dup, err := collect(items)
	if err != nil {
		return err
	}
	return wiringError(dup)
}
