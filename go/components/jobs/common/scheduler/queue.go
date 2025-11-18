package scheduler

import (
	"context"
	"strconv"
	"sync"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"go.uber.org/fx"
)

// Queue represents a scheduler queue
type Queue interface {
	Add(ctx context.Context, obj types.SchedulableJob) error
	Get(ctx context.Context) (types.SchedulableJob, error)
	Done(ctx context.Context, obj types.SchedulableJob) error
	Length() int
}

var _ Queue = (*channelQueue)(nil)

// channelQueue is a simple FIFO implementation
// meant for consumption by job scheduler.
// We should replace this with priority queues
// when supporting job priority.
type channelQueue struct {
	q chan types.SchedulableJob

	// dirty defines all of the items that need to be processed. - used to dedupe
	dirty map[string]struct{}
	// Things that are currently being processed are in the processing set.
	processing map[string]struct{}
	// used to guard access to entries
	m sync.Mutex
}

const _queueSize int = 2000

// Result is the output of this module
type Result struct {
	fx.Out

	Queue
}

// New provides a queue.
func New() Result {
	return Result{
		Queue: &channelQueue{
			q:          make(chan types.SchedulableJob, _queueSize),
			dirty:      make(map[string]struct{}),
			processing: make(map[string]struct{}),
		},
	}
}

// Push adds an object to the queue
func (cq *channelQueue) Add(ctx context.Context, obj types.SchedulableJob) error {
	// We have only 2 job controllers. So we do not expect much contention here.
	cq.m.Lock()
	defer cq.m.Unlock()

	objKey := cq.getKey(obj)

	if _, ok := cq.dirty[objKey]; ok {
		return types.ErrJobAlreadyExists
	}

	if _, ok := cq.processing[objKey]; ok {
		return types.ErrJobAlreadyExists
	}

	select {
	case cq.q <- obj:
		cq.dirty[objKey] = struct{}{}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Get returns an object from the queue
func (cq *channelQueue) Get(ctx context.Context) (types.SchedulableJob, error) {
	select {
	case obj := <-cq.q:
		cq.m.Lock()
		defer cq.m.Unlock()
		delete(cq.dirty, cq.getKey(obj))
		cq.processing[cq.getKey(obj)] = struct{}{}
		return obj, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Done marks an object as done
func (cq *channelQueue) Done(ctx context.Context, obj types.SchedulableJob) error {
	cq.m.Lock()
	defer cq.m.Unlock()
	delete(cq.processing, cq.getKey(obj))
	return nil
}

// Length returns the number of elements in the queue
func (cq *channelQueue) Length() int {
	cq.m.Lock()
	defer cq.m.Unlock()
	return len(cq.q) + len(cq.processing)
}

func (cq *channelQueue) getKey(obj types.SchedulableJob) string {
	return obj.GetNamespace() + obj.GetName() + strconv.FormatInt(obj.GetGeneration(), 10)
}
