package state

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// clusterWatch watches a cluster's ClusterQueues, and its LocalQueues in the
// namespace jobs are dispatched to.
type clusterWatch struct {
	// cluster is the cluster as it was watched.
	cluster       *v2pb.Cluster
	clusterQueues *queueInformer
	localQueues   *queueInformer
	// cancel stops the informers; nil until they start.
	cancel context.CancelFunc
	// done is closed once the started informers have stopped.
	done chan struct{}
}

// newClusterWatch returns an unstarted watch of the cluster's queues through
// the Kueue client that build returns.
func newClusterWatch(cluster *v2pb.Cluster, build func() (rest.Interface, error), log logr.Logger) *clusterWatch {
	client := &lazyClient{build: build}
	log = log.WithValues("cluster", cluster.Name)
	return &clusterWatch{
		cluster:       cluster,
		clusterQueues: newQueueInformer(client, "clusterqueues", metav1.NamespaceNone, log),
		localQueues:   newQueueInformer(client, "localqueues", k8sengine.RayLocalNamespace, log),
		done:          make(chan struct{}),
	}
}

// start runs the informers until ctx is done or the watch is stopped.
func (w *clusterWatch) start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	var wg sync.WaitGroup
	for _, q := range []*queueInformer{w.clusterQueues, w.localQueues} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.informer.RunWithContext(ctx)
		}()
	}
	go func() {
		wg.Wait()
		close(w.done)
	}()
}

func (w *clusterWatch) started() bool {
	return w.cancel != nil
}

func (w *clusterWatch) stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

// synced reports whether both kinds of queue were listed since the watch
// started or last failed.
func (w *clusterWatch) synced() bool {
	return w.clusterQueues.synced() && w.localQueues.synced()
}

// queueInformer watches one kind of Kueue queue on a cluster.
type queueInformer struct {
	informer cache.SharedIndexInformer
	// listed is set by each successful list and cleared by each list or watch
	// error: an informer never reports unsynced again once it has synced, so
	// this is how a broken watch shows.
	listed atomic.Bool
}

func newQueueInformer(client *lazyClient, resource, namespace string, log logr.Logger) *queueInformer {
	q := &queueInformer{}
	lw := &queueListWatch{client: client, resource: resource, namespace: namespace, listed: &q.listed}
	q.informer = cache.NewSharedIndexInformer(lw, &unstructured.Unstructured{}, 0, cache.Indexers{})
	log = log.WithValues("resource", resource)
	// fails only once the informer has started
	_ = q.informer.SetWatchErrorHandlerWithContext(func(_ context.Context, _ *cache.Reflector, err error) {
		if q.listed.Swap(false) {
			log.Info("Kueue queue watch failed; the cluster's Kueue state is unsynced until its next successful list",
				"error", err.Error())
			return
		}
		log.V(1).Info("Kueue queue list or watch failed; the cluster's Kueue state stays unsynced", "error", err.Error())
	})
	return q
}

func (q *queueInformer) synced() bool {
	return q.listed.Load() && q.informer.HasSynced()
}

// get returns the queue with the given store key: its name, prefixed with its
// namespace and a slash if it is namespaced.
func (q *queueInformer) get(key string) (*unstructured.Unstructured, bool, error) {
	obj, found, err := q.informer.GetStore().GetByKey(key)
	if err != nil || !found {
		return nil, false, err
	}
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, false, fmt.Errorf("the informer stored %T, not an unstructured object", obj)
	}
	return u, true, nil
}

// queueListWatch lists and watches one kind of Kueue queue, and records each
// successful list.
type queueListWatch struct {
	client *lazyClient
	// resource is the queue kind's plural resource name.
	resource string
	// namespace is the namespace the queues are watched in; empty for
	// cluster-scoped queues.
	namespace string
	listed    *atomic.Bool
}

var _ cache.ListerWatcherWithContext = (*queueListWatch)(nil)

func (lw *queueListWatch) ListWithContext(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
	c, err := lw.client.get()
	if err != nil {
		return nil, err
	}
	list := &unstructured.UnstructuredList{}
	if err := c.Get().Namespace(lw.namespace).Resource(lw.resource).
		VersionedParams(&options, metav1.ParameterCodec).Do(ctx).Into(list); err != nil {
		return nil, err
	}
	lw.listed.Store(true)
	return list, nil
}

func (lw *queueListWatch) WatchWithContext(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
	c, err := lw.client.get()
	if err != nil {
		return nil, err
	}
	options.Watch = true
	return c.Get().Namespace(lw.namespace).Resource(lw.resource).
		VersionedParams(&options, metav1.ParameterCodec).Watch(ctx)
}

func (lw *queueListWatch) List(options metav1.ListOptions) (runtime.Object, error) {
	return lw.ListWithContext(context.Background(), options)
}

func (lw *queueListWatch) Watch(options metav1.ListOptions) (watch.Interface, error) {
	return lw.WatchWithContext(context.Background(), options)
}

// IsWatchListSemanticsUnSupported turns the informer's watch-list (streamed
// lists) off, so that every resync starts with a real list, which marks the
// informer listed.
func (lw *queueListWatch) IsWatchListSemanticsUnSupported() bool {
	return true
}

// lazyClient builds a cluster's Kueue client on first use and keeps it. A
// failed build is retried on the next use: a Secret that cannot be read yet
// fails the list, and the informer retries it with backoff.
type lazyClient struct {
	build func() (rest.Interface, error)

	mu     sync.Mutex
	client rest.Interface
}

func (l *lazyClient) get() (rest.Interface, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.client == nil {
		c, err := l.build()
		if err != nil {
			return nil, err
		}
		l.client = c
	}
	return l.client, nil
}
