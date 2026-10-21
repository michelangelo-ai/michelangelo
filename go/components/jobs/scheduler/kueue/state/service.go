package state

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// kueueBackend is the jobs.scheduler.backend value of the Kueue backend.
	kueueBackend = "kueue"
	// defaultAPIVersion is the kueue.x-k8s.io API version an empty
	// jobs.scheduler.kueue.apiVersion means, as for kueue.LocalQueues.
	defaultAPIVersion = "v1beta2"
)

var (
	_ cluster.KueueWatches = (*Service)(nil)
	_ State                = (*Service)(nil)
	_ Demand               = (*Service)(nil)
)

// Service keeps the Kueue state of the Kueue-managed clusters: it watches each
// cluster's queues as the cluster controller tells it, serves State from what
// it watched, and serves Demand. Under any backend but Kueue it is inert: it
// watches nothing and nothing is in flight.
type Service struct {
	enabled    bool
	apiVersion string
	factory    compute.Factory
	log        logr.Logger
	tracker    *tracker

	mu sync.Mutex
	// ctx is the manager's context once the manager has started the service;
	// watches wait for it.
	ctx     context.Context
	watches map[string]*clusterWatch
}

// newService returns the Kueue state service for the scheduler config; reader
// is the manager cache. Under the Kueue backend, the configured Kueue API
// version must be one the service reads.
func newService(
	cfg maconfig.SchedulerConfig, factory compute.Factory, reader client.Reader, mapper types.Mapper, log logr.Logger,
) (*Service, error) {
	s := &Service{
		enabled:    cfg.Backend == kueueBackend,
		apiVersion: cfg.Kueue.APIVersion,
		factory:    factory,
		log:        log,
		tracker:    newTracker(reader, mapper, log),
		watches:    map[string]*clusterWatch{},
	}
	if s.apiVersion == "" {
		s.apiVersion = defaultAPIVersion
	}
	if _, ok := cohortFields[s.apiVersion]; s.enabled && !ok {
		return nil, fmt.Errorf("unsupported jobs.scheduler.kueue.apiVersion %q (expected v1beta1 or v1beta2)",
			cfg.Kueue.APIVersion)
	}
	return s, nil
}

// run starts the watches under the manager's context, which stops them when
// it is done.
func (s *Service) run(ctx context.Context) error {
	s.mu.Lock()
	s.ctx = ctx
	for _, w := range s.watches {
		if !w.started() {
			w.start(ctx)
		}
	}
	s.mu.Unlock()
	<-ctx.Done()
	return nil
}

// Watch starts watching the cluster's queues, replacing a running watch of the
// cluster.
func (s *Service) Watch(c *v2pb.Cluster) {
	if !s.enabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startLocked(c.DeepCopy())
}

// Rewatch restarts the named cluster's watch, if it has one, with a new client.
func (s *Service) Rewatch(name string) {
	if !s.enabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if w, ok := s.watches[name]; ok {
		s.startLocked(w.cluster)
	}
}

// Unwatch stops watching the named cluster.
func (s *Service) Unwatch(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w, ok := s.watches[name]; ok {
		w.stop()
		delete(s.watches, name)
	}
}

// startLocked starts watching the cluster with a client built from its current
// credentials, replacing its running watch. A watch started before the
// manager starts the service waits for it.
func (s *Service) startLocked(c *v2pb.Cluster) {
	if old, ok := s.watches[c.Name]; ok {
		old.stop()
	}
	w := newClusterWatch(c, func() (rest.Interface, error) {
		cfg, err := s.factory.GetRestConfigForCluster(c)
		if err != nil {
			return nil, fmt.Errorf("get the REST config of cluster %s: %w", c.Name, err)
		}
		return newKueueClient(cfg, s.apiVersion)
	}, s.log)
	s.watches[c.Name] = w
	if s.ctx != nil {
		w.start(s.ctx)
	}
}

func (s *Service) watch(name string) *clusterWatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watches[name]
}

// Synced implements State.
func (s *Service) Synced(cluster string) bool {
	w := s.watch(cluster)
	return w != nil && w.synced()
}

// Lookup implements State.
func (s *Service) Lookup(cluster, localQueue string) (View, error) {
	w := s.watch(cluster)
	if w == nil || !w.synced() {
		return View{}, nil
	}
	view := View{Synced: true}
	lq, found, err := w.localQueues.get(k8sengine.RayLocalNamespace + "/" + localQueue)
	if err != nil || !found {
		return view, err
	}
	clusterQueue, active, err := parseLocalQueue(lq)
	if err != nil {
		return View{}, err
	}
	view.Exists, view.Active = true, active

	cq, found, err := w.clusterQueues.get(clusterQueue)
	if err != nil || !found {
		return view, err
	}
	if view.ClusterQueue, err = parseClusterQueue(cq, s.apiVersion); err != nil {
		return View{}, err
	}
	return view, nil
}

// Ask implements Demand.
func (s *Service) Ask(job *v2pb.RayCluster, cluster *v2pb.Cluster) (corev1.ResourceList, error) {
	return s.tracker.ask(job, cluster)
}

// InFlight implements Demand.
func (s *Service) InFlight(ctx context.Context, cluster *v2pb.Cluster, now time.Time) (InFlight, error) {
	if !s.enabled {
		return InFlight{Ask: corev1.ResourceList{}}, nil
	}
	return s.tracker.inFlight(ctx, cluster, now)
}

// RecordAssignment implements Demand.
func (s *Service) RecordAssignment(job *v2pb.RayCluster, cluster string, at time.Time) {
	if !s.enabled {
		return
	}
	s.tracker.recordAssignment(job, cluster, at)
}
