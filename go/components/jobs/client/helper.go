//go:generate mamockgen Helper
package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/kuberay"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// WatcherParams defined params for creating watcher
type WatcherParams struct {
	Client               rest.Interface             // required
	ResourceName         string                     // required
	Namespace            string                     // required
	ObjType              runtime.Object             // required
	ResourceEventHandler cache.ResourceEventHandler // required

	LabelSelector *metav1.LabelSelector // optional
}

// Helper gives helper functions for clients to create resources in kube clusters
type Helper interface {
	CreateResource(ctx context.Context, client rest.Interface, objectBody runtime.Object, resource string, namespace string) error
	GetResource(ctx context.Context, client rest.Interface, resource string, namespace string, name string, result runtime.Object) error
	ListResources(ctx context.Context, client rest.Interface, resource string, namespace string, result runtime.Object) error
	DeleteResource(ctx context.Context, client rest.Interface, resource string, namespace string, name string, opts metav1.DeleteOptions) (err error)
	Watcher(params []*WatcherParams) ([]*ResourceWatcher, error)
	NewFilteredListWatchFromClient(client rest.Interface, resource string, namespace string, codec runtime.ParameterCodec, optionsModifier func(options *metav1.ListOptions)) *cache.ListWatch
	GetClusterHealth(ctx context.Context, client rest.Interface) (*v2pb.ClusterStatus, error)
}

// defaultHelper implements the Helper interface
type defaultHelper struct{}

// NewHelper is the constructor for defaultHelper
func NewHelper() Helper {
	return defaultHelper{}
}

// CreateResource creates resource in k8s cluster using given cluster client
func (d defaultHelper) CreateResource(
	ctx context.Context, client rest.Interface, objectBody runtime.Object, resource string, namespace string,
) error {
	result := objectBody.DeepCopyObject()
	err := client.Post().Resource(resource).Namespace(namespace).
		Body(objectBody).Do(ctx).Into(result)
	return err
}

// GetResource gets resource from k8s cluster using given cluster client
func (d defaultHelper) GetResource(
	ctx context.Context, client rest.Interface, resource string, namespace string, name string, result runtime.Object,
) error {
	return client.Get().Resource(resource).Namespace(namespace).Name(name).
		Do(ctx).Into(result)
}

// ListResources lists the available resources
func (d defaultHelper) ListResources(
	ctx context.Context, client rest.Interface, resource string, namespace string, result runtime.Object,
) error {
	err := client.Get().Resource(resource).Namespace(namespace).
		Do(ctx).Into(result)
	return err
}

// DeleteResource deletes resource in k8s cluster using given cluster client
func (d defaultHelper) DeleteResource(ctx context.Context, client rest.Interface, resource string, namespace string, name string, opts metav1.DeleteOptions) (err error) {
	return client.Delete().Resource(resource).Namespace(namespace).Name(name).
		Body(&opts).Do(ctx).Error()
}

// NewFilteredListWatchFromClient creates a new ListWatch from the specified client, resource, namespace, codec and option modifier.
// Option modifier is a function takes a ListOptions and modifies the consumed ListOptions. Provide customized modifier function
// to apply modification to ListOptions with a field selector, a label selector, or any other desired options.
// Implements NewFilteredListWatchFromClient from k8s.io/client-go/tools/cache/listwatch.go with our custom scheme
func (d defaultHelper) NewFilteredListWatchFromClient(client rest.Interface, resource string, namespace string,
	codec runtime.ParameterCodec, optionsModifier func(options *metav1.ListOptions),
) *cache.ListWatch {
	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		optionsModifier(&options)
		return client.Get().
			Namespace(namespace).
			Resource(resource).
			VersionedParams(&options, codec).
			Do(context.TODO()).
			Get()
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true
		optionsModifier(&options)
		return client.Get().
			Namespace(namespace).
			Resource(resource).
			VersionedParams(&options, codec).
			Watch(context.TODO())
	}
	return &cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc}
}

// If non-zero, will re-list this often (you will get OnUpdate
// calls, even if nothing changed). Otherwise, re-list will be delayed as
// long as possible (until the upstream source closes the watch or times out,
// or you stop the controller).
const _reSyncPeriod = time.Minute * 5

// Watcher creates a watch on specified resource using given cluster client
func (d defaultHelper) Watcher(params []*WatcherParams) ([]*ResourceWatcher, error) {
	var result []*ResourceWatcher

	for _, p := range params {
		m, err := metav1.LabelSelectorAsMap(p.LabelSelector)
		if err != nil {
			return nil, err
		}

		ls := labels.SelectorFromSet(m).String()

		listOptionsFunc := func(options *metav1.ListOptions) {
			options.LabelSelector = ls
		}

		var lw *cache.ListWatch
		switch p.ResourceName {
		// For CRDs, we need a list and watch with our custom scheme
		case constants.KubeSparkResource:
			return nil, fmt.Errorf("Spark job is not supported")
		case constants.KubeRayResource:
			lw = d.NewFilteredListWatchFromClient(
				p.Client,
				p.ResourceName,
				p.Namespace,
				kuberay.ParameterCodec,
				listOptionsFunc)
		default:
			lw = cache.NewFilteredListWatchFromClient(
				p.Client,
				p.ResourceName,
				p.Namespace,
				listOptionsFunc)
		}

		s, ct := cache.NewInformer(lw, p.ObjType, _reSyncPeriod, p.ResourceEventHandler)
		res := &ResourceWatcher{
			Store:      s,
			Controller: ct,
		}
		result = append(result, res)
	}
	return result, nil
}

func (d defaultHelper) GetClusterHealth(ctx context.Context, client rest.Interface) (
	*v2pb.ClusterStatus, error,
) {
	clusterStatus := &v2pb.ClusterStatus{}
	currentTime := metav1.Now()

	body, err := client.Get().AbsPath("/healthz").Do(ctx).Raw()
	if err != nil {
		offlineCond := newClusterOfflineCondition(currentTime)
		offlineCond.Message = fmt.Sprintf("%s: %v", ClusterNotReachableMsg, err)
		clusterStatus.StatusConditions = append(clusterStatus.StatusConditions, offlineCond)
		return clusterStatus, err
	}

	if !strings.EqualFold(string(body), "ok") {
		// connected but not ready
		clusterStatus.StatusConditions = append(clusterStatus.StatusConditions,
			newClusterNotReadyCondition(currentTime), newClusterNotOfflineCondition(currentTime))
		return clusterStatus, err
	}

	// connected and ready
	clusterStatus.StatusConditions = append(clusterStatus.StatusConditions, newClusterReadyCondition(currentTime))
	return clusterStatus, err
}
