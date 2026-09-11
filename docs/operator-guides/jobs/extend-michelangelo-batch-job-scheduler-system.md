# Extend the Job Scheduler System

This page describes the Michelangelo AI Job Scheduler architecture and how to extend it with custom backends or plugins.

## Architecture overview

The scheduler system is designed with clean separation of concerns, allowing job controllers to remain agnostic to the underlying scheduling implementation.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Job Controllers                                   │
│  (RayCluster Controller, SparkJob Controller, etc.)                         │
│                                                                             │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                      JobQueue.Enqueue(job)                             │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           JobQueue Interface                                │
│                                                                             │
│  ┌─────────────────────────┐              ┌────────────────────────────────┐│
│  │   Default Scheduler     │     OR       │    Custom Backend              ││
│  │   (internal queue +     │              │    (e.g., Kueue, Volcano)      ││
│  │    goroutine loop)      │              │                                ││
│  └─────────────────────────┘              └────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                       AssignmentStrategy Interface                          │
│                                                                             │
│  ┌─────────────────────────┐              ┌────────────────────────────────┐│
│  │ ClusterOnlyAssignment   │     OR       │    Custom Strategy             ││
│  │ Strategy (default)      │              │    (resource-aware, ML-based)  ││
│  └─────────────────────────┘              └────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                      FederatedClient                                        │
│  (Dispatches job to assigned remote Compute Cluster)                        │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Core Interfaces

### 1. JobQueue Interface

The primary interface that job controllers depend on. Controllers only know about `JobQueue` - they don't care how scheduling is implemented.

**Location:** `go/components/jobs/scheduler/scheduler.go`

```go
// JobQueue is the job queue for the scheduler.
type JobQueue interface {
    Enqueue(ctx context.Context, job matypes.SchedulableJob) error
}
```

**Key Points:**
- Controllers call `Enqueue()` when a job needs scheduling
- The implementation decides _how_ to schedule (internal queue, external system, etc.)
- Job controllers remain completely decoupled from scheduler internals

### 2. AssignmentStrategy Interface

Plugin interface for cluster selection logic. Determines which compute cluster should run a job.

**Location:** `go/components/jobs/scheduler/framework/interface.go`

```go
// AssignmentStrategy decides assignment for a job.
type AssignmentStrategy interface {
    // Select decides an assignment for the given job.
    // Returns (assignment, found, reason, err)
    Select(ctx context.Context, job BatchJob) (*v2pb.AssignmentInfo, bool, string, error)
}
```

**Return Values:**
| Return | Description |
|--------|-------------|
| `*v2pb.AssignmentInfo` | The assignment (cluster name, etc.) if found |
| `bool found` | Whether a suitable cluster was found |
| `string reason` | Human-readable reason for the decision |
| `error` | Any error during selection |

### 3. BatchJob Interface

Rich job interface used by the scheduler for making assignment decisions. Provides access to job metadata, resource requirements, and scheduling preferences.

**Location:** `go/components/jobs/scheduler/framework/job.go`

```go
type BatchJob interface {
    // From SchedulableJob
    GetName() string
    GetNamespace() string
    GetGeneration() int64
    GetJobType() JobType

    // Scheduling-specific
    GetAffinity() *v2pb.Affinity              // Cluster affinity preferences
    GetConditions() *[]*apipb.Condition       // Job conditions
    GetAssignmentInfo() *v2pb.AssignmentInfo  // Current assignment (if any)
    GetObject() client.Object                 // Underlying K8s object
    GetLabels() map[string]string             // Job labels
    GetAnnotations() map[string]string        // Job annotations
    GetResourceRequirement() (map[string]v1.ResourceList, error)  // Resource needs
    GetUserName() string                      // Submitting user
    GetTerminationSpec() *v2pb.TerminationSpec
    IsPreemptibleJob() bool                   // Preemption eligibility
}
```

### 4. SchedulableJob Interface

Minimal job interface for queue operations. Used when the full job details aren't needed.

**Location:** `go/components/jobs/common/types/types.go`

```go
type SchedulableJob interface {
    GetName() string
    GetNamespace() string
    GetGeneration() int64
    GetJobType() JobType
}
```

### 5. Queue Interface

Internal queue abstraction used by the default scheduler implementation.

**Location:** `go/components/jobs/common/scheduler/queue.go`

```go
type Queue interface {
    Add(ctx context.Context, obj types.SchedulableJob) error
    Get(ctx context.Context) (types.SchedulableJob, error)
    Done(ctx context.Context, obj types.SchedulableJob) error
    Length() int
}
```

### 6. RegisteredClustersCache Interface

Provides access to available compute clusters for assignment decisions.

**Location:** `go/components/jobs/cluster/types.go`

```go
type RegisteredClustersCache interface {
    GetClusters(filter FilterType) []*v2pb.Cluster
    GetCluster(name string) *v2pb.Cluster
}
```

---

## Default Scheduler Implementation

The default `Scheduler` struct implements `JobQueue` with an internal queue and a background goroutine that processes jobs.

### Flow

```
1. Controller calls Enqueue(job)
        │
        ▼
2. Job added to internal queue (with deduplication)
        │
        ▼
3. Background goroutine pulls job from queue
        │
        ▼
4. Scheduler fetches latest job state from API server
        │
        ▼
5. AssignmentStrategy.Select(job) is called
        │
        ▼
6. On success: Update job's AssignmentInfo and ScheduledCondition
        │
        ▼
7. Controller observes assignment, dispatches to compute cluster
```

### Key Components

```go
type Scheduler struct {
    api.Handler                           // API access
    log                logr.Logger
    mgr                ctrl.Manager
    metrics            *metrics.ControllerMetrics
    assignmentStrategy framework.AssignmentStrategy  // Pluggable strategy

    scheduleFunc       scheduleFunc        // Main scheduling loop
    internalQueue      sched.Queue         // Internal job queue
    initLock           atomic.Bool         // Initialization guard
}
```

---

## Extension Points

### Option 1: Custom AssignmentStrategy

**Best for:** Changing _how_ cluster selection works while keeping the queue-based model.

**Examples:**
- Resource-aware placement (GPU availability, memory)
- Cost-optimized selection
- Latency-based selection
- ML-based prediction

**Steps:**

1. **Implement the interface:**

```go
package mystrategy

import (
    "context"
    "github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
    v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

type ResourceAwareStrategy struct {
    ClusterCache cluster.RegisteredClustersCache
    // Add any dependencies you need
}

var _ framework.AssignmentStrategy = (*ResourceAwareStrategy)(nil)

func (s *ResourceAwareStrategy) Select(
    ctx context.Context,
    job framework.BatchJob,
) (*v2pb.AssignmentInfo, bool, string, error) {
    // Get job resource requirements
    resources, err := job.GetResourceRequirement()
    if err != nil {
        return nil, false, "failed to get resources", err
    }

    // Get available clusters
    clusters := s.ClusterCache.GetClusters(cluster.ReadyClusters)
    if len(clusters) == 0 {
        return nil, false, "no_clusters_available", nil
    }

    // Your custom selection logic here
    bestCluster := selectBestCluster(clusters, resources, job.GetAffinity())
    if bestCluster == nil {
        return nil, true, "no_cluster_matched_requirements", nil
    }

    return &v2pb.AssignmentInfo{
        Cluster: bestCluster.GetName(),
    }, true, "resource_matched", nil
}
```

2. **Register via FX module:**

```go
// framework/module.go
var Module = fx.Options(
    fx.Provide(
        fx.Annotate(
            NewResourceAwareStrategy,
            fx.As(new(AssignmentStrategy)),
        ),
    ),
)
```

Or use a factory pattern for runtime selection:

```go
func NewAssignmentStrategy(config Config, cache cluster.RegisteredClustersCache) AssignmentStrategy {
    switch config.StrategyType {
    case "resource-aware":
        return NewResourceAwareStrategy(cache)
    case "cost-optimized":
        return NewCostOptimizedStrategy(cache)
    default:
        return NewClusterOnlyAssignmentStrategy(cache)
    }
}
```

### Option 2: Custom JobQueue Implementation

**Best for:** Delegating admission to an external system.

**Shipped example: the Kueue backend.** `go/components/jobs/scheduler/kueue` implements `JobQueue` for [Kueue](https://kueue.sigs.k8s.io/)-managed compute clusters and is selected with `jobs.scheduler.backend: kueue` -- see the [Kueue Scheduler Backend](kueue-scheduler-backend.md) guide for operating it.

Its design is instructive for any external-admission backend: it does **not** replace the default scheduler or hand-craft admission objects (an earlier sketch of this page created Kueue `Workload`s from the control plane -- Kueue's own integrations do that better). Instead it wraps the default scheduler:

```go
// KueueJobQueue validates a job's Kueue placement, then delegates.
func (q *KueueJobQueue) Enqueue(ctx context.Context, job matypes.SchedulableJob) error {
    // 1. Predict the target cluster (existing assignment, else the same
    //    AssignmentStrategy the default scheduler uses).
    // 2. If the target is not Kueue-managed, pass through unchanged.
    // 3. Resolve the job's project to a LocalQueue and verify it exists on
    //    the target; fail visibly (Enqueued=False) if not.
    // 4. Hand the job to the default scheduler. The k8s engine labels the
    //    dispatched object with kueue.x-k8s.io/queue-name, and Kueue's own
    //    RayCluster integration suspends and gang-admits it on the
    //    compute cluster.
}
```

**Wiring:** the scheduler module provides the concrete `*Scheduler` plus a factory that switches on config -- add your backend as a new case:

```go
// scheduler/module.go
func provide(in ProvideIn) (JobQueue, error) {
    switch in.Config.Backend {
    case "", _backendDefault:
        return in.Scheduler, nil
    case _backendKueue:
        // wrap in.Scheduler in the Kueue-validating queue
    default:
        return nil, fmt.Errorf("unrecognized jobs.scheduler.backend %q", in.Config.Backend)
    }
}
```

A fully replacing backend (e.g. a Volcano integration that owns queueing end to end) plugs into the same switch; implementing `Enqueue` and updating the job's conditions/`AssignmentInfo` is the whole contract. Prefer the wrapping shape where the external system runs on the compute clusters, and a replacing shape only when it runs centrally.

### Option 3: Custom Queue Implementation

**Best for:** Changing queue behavior (priority, ordering) within the default scheduler.

**Examples:**
- Priority-based scheduling
- Fair-share queuing
- Time-based scheduling

**Steps:**

1. **Implement the Queue interface:**

```go
package priorityqueue

import (
    "container/heap"
    "context"
    "sync"
    matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
)

type PriorityQueue struct {
    heap       jobHeap
    processing map[string]struct{}
    mu         sync.Mutex
}

func (pq *PriorityQueue) Add(ctx context.Context, job matypes.SchedulableJob) error {
    pq.mu.Lock()
    defer pq.mu.Unlock()

    // Add with priority from job annotations or labels
    priority := extractPriority(job)
    heap.Push(&pq.heap, &priorityItem{job: job, priority: priority})
    return nil
}

func (pq *PriorityQueue) Get(ctx context.Context) (matypes.SchedulableJob, error) {
    pq.mu.Lock()
    defer pq.mu.Unlock()

    if pq.heap.Len() == 0 {
        // Block until job available or context cancelled
        return nil, ctx.Err()
    }

    item := heap.Pop(&pq.heap).(*priorityItem)
    pq.processing[getKey(item.job)] = struct{}{}
    return item.job, nil
}
```

2. **Provide via FX:**

```go
var Module = fx.Provide(
    func() scheduler.Queue {
        return priorityqueue.New()
    },
)
```

---

## Adding Support for New Job Types

To schedule a new job type (beyond RayCluster and SparkJob):

### 1. Add JobType constant

```go
// go/components/jobs/common/types/types.go
const (
    SparkJob JobType = iota + 1
    RayJob
    RayCluster
    FlinkJob  // New type
)
```

### 2. Implement BatchJob wrapper

```go
// go/components/jobs/scheduler/framework/job.go
type BatchFlinkJob struct {
    *v2pb.FlinkJob
}

var _ BatchJob = BatchFlinkJob{}

func (f BatchFlinkJob) GetAffinity() *v2pb.Affinity {
    return f.Spec.GetAffinity()
}

func (f BatchFlinkJob) GetResourceRequirement() (map[string]v1.ResourceList, error) {
    // Sum up JobManager + TaskManager resources
    // ...
}

// Implement remaining interface methods...
```

### 3. Update scheduler to handle new type

```go
// go/components/jobs/scheduler/scheduler.go
func (c *Scheduler) fetchLatestJob(ctx context.Context, job matypes.SchedulableJob, latest *framework.BatchJob) error {
    switch job.GetJobType() {
    case matypes.RayCluster:
        // existing code
    case matypes.SparkJob:
        // existing code
    case matypes.FlinkJob:  // New case
        flinkJob := &v2pb.FlinkJob{}
        if err := c.Get(getCtx, job.GetNamespace(), job.GetName(), &metav1.GetOptions{}, flinkJob); err != nil {
            return err
        }
        *latest = framework.BatchFlinkJob{FlinkJob: flinkJob}
        return nil
    }
}
```

### 4. Update controller to enqueue jobs

```go
// In your FlinkJob controller
func (r *Reconciler) enqueueIfRequired(ctx context.Context, job *v2pb.FlinkJob) error {
    if err := r.schedulerQueue.Enqueue(ctx, matypes.NewSchedulableJob(matypes.SchedulableJobParams{
        Name:       job.Name,
        Namespace:  job.Namespace,
        Generation: job.Generation,
        JobType:    matypes.FlinkJob,
    })); err != nil {
        return err
    }
    return nil
}
```
