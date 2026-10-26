package cluster

import (
	"context"
	"crypto/x509"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// recordedWatches records the Kueue watch calls the controller makes. (The
// generated mocks import this package, so its own tests cannot use them.)
type recordedWatches struct {
	mu    sync.Mutex
	calls []string
}

func (w *recordedWatches) record(call string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, call)
}

// Watch records the cluster's token tag too, to show which credentials the
// watch is started with.
func (w *recordedWatches) Watch(c *v2pb.Cluster) {
	w.record("watch " + c.Name + " " + c.Spec.GetKubernetes().GetRest().GetTokenTag())
}
func (w *recordedWatches) Rewatch(name string) { w.record("rewatch " + name) }
func (w *recordedWatches) Unwatch(name string) { w.record("unwatch " + name) }

func (w *recordedWatches) get() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

func TestKueueWatchChange(t *testing.T) {
	spec := func(schedulerType v2pb.SchedulerType, updates ...func(*v2pb.ClusterSpec)) *v2pb.ClusterSpec {
		s := _testCluster.DeepCopy().Spec
		s.SchedulerType = schedulerType
		s.GetKubernetes().Rest.TokenTag = "token"
		s.GetKubernetes().Rest.CaDataTag = "ca"
		for _, update := range updates {
			update(&s)
		}
		return &s
	}
	newToken := func(s *v2pb.ClusterSpec) { s.GetKubernetes().Rest.TokenTag = "token-new" }
	newCA := func(s *v2pb.ClusterSpec) { s.GetKubernetes().Rest.CaDataTag = "ca-new" }
	newHost := func(s *v2pb.ClusterSpec) { s.GetKubernetes().Rest.Host = "https://new" }
	newPort := func(s *v2pb.ClusterSpec) { s.GetKubernetes().Rest.Port = "443" }
	newSLA := func(s *v2pb.ClusterSpec) { s.Sla = v2pb.SLA_TYPE_PRODUCTION }
	kueue, def := v2pb.SCHEDULER_TYPE_KUEUE, v2pb.SCHEDULER_TYPE_DEFAULT

	tests := []struct {
		name    string
		oldSpec *v2pb.ClusterSpec
		newSpec *v2pb.ClusterSpec
		want    watchChange
	}{
		{name: "Kueue-managed cluster added", newSpec: spec(kueue), want: startWatches},
		{name: "default cluster added", newSpec: spec(def), want: keepWatches},
		{name: "cluster without a scheduler type added", newSpec: spec(v2pb.SCHEDULER_TYPE_INVALID), want: keepWatches},
		{name: "marked", oldSpec: spec(def), newSpec: spec(kueue), want: startWatches},
		{name: "marked with new credentials", oldSpec: spec(def), newSpec: spec(kueue, newToken), want: startWatches},
		{name: "unmarked", oldSpec: spec(kueue), newSpec: spec(def), want: stopWatches},
		{name: "unmarked with new credentials", oldSpec: spec(kueue), newSpec: spec(def, newToken), want: stopWatches},
		{name: "token tag change", oldSpec: spec(kueue), newSpec: spec(kueue, newToken), want: startWatches},
		{name: "CA data tag change", oldSpec: spec(kueue), newSpec: spec(kueue, newCA), want: startWatches},
		{name: "host change", oldSpec: spec(kueue), newSpec: spec(kueue, newHost), want: startWatches},
		{name: "port change", oldSpec: spec(kueue), newSpec: spec(kueue, newPort), want: startWatches},
		{name: "other spec change", oldSpec: spec(kueue), newSpec: spec(kueue, newSLA), want: keepWatches},
		{name: "unchanged", oldSpec: spec(kueue), newSpec: spec(kueue), want: keepWatches},
		{name: "default cluster's credentials change", oldSpec: spec(def), newSpec: spec(def, newToken), want: keepWatches},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, kueueWatchChange(tt.oldSpec, tt.newSpec))
		})
	}
}

func TestReconcileUpdatesKueueWatches(t *testing.T) {
	cluster := func(schedulerType v2pb.SchedulerType, tokenTag string) *v2pb.Cluster {
		c := _testCluster.DeepCopy()
		c.Finalizers = []string{_clusterDeleteFinalizer}
		c.Spec.SchedulerType = schedulerType
		c.Spec.GetKubernetes().Rest.TokenTag = tokenTag
		return c
	}
	withSLA := func(c *v2pb.Cluster) *v2pb.Cluster {
		c.Spec.Sla = v2pb.SLA_TYPE_PRODUCTION
		return c
	}
	deleting := func(c *v2pb.Cluster) *v2pb.Cluster {
		c.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		return c
	}
	kueue, def := v2pb.SCHEDULER_TYPE_KUEUE, v2pb.SCHEDULER_TYPE_DEFAULT

	tests := []struct {
		name string
		// cached is the cluster the controller holds; nil for a new cluster
		cached *v2pb.Cluster
		// current is the cluster the API returns
		current        *v2pb.Cluster
		wantInvalidate bool
		wantCalls      []string
	}{
		{
			name:      "added Kueue-managed cluster is watched",
			current:   cluster(kueue, "token-old"),
			wantCalls: []string{"watch cluster-1 token-old"},
		},
		{
			name:    "added default cluster is not watched",
			current: cluster(def, "token-old"),
		},
		{
			name:      "marked cluster is watched",
			cached:    cluster(def, "token-old"),
			current:   cluster(kueue, "token-old"),
			wantCalls: []string{"watch cluster-1 token-old"},
		},
		{
			name:      "unmarked cluster is no longer watched",
			cached:    cluster(kueue, "token-old"),
			current:   cluster(def, "token-old"),
			wantCalls: []string{"unwatch cluster-1"},
		},
		{
			name:           "credential change rewatches with the new credentials",
			cached:         cluster(kueue, "token-old"),
			current:        cluster(kueue, "token-new"),
			wantInvalidate: true,
			wantCalls:      []string{"watch cluster-1 token-new"},
		},
		{
			name:    "other spec change keeps the watch",
			cached:  cluster(kueue, "token-old"),
			current: withSLA(cluster(kueue, "token-old")),
		},
		{
			name:      "deleted cluster is no longer watched",
			cached:    cluster(kueue, "token-old"),
			current:   deleting(cluster(kueue, "token-old")),
			wantCalls: []string{"unwatch cluster-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gCtrl := gomock.NewController(t)
			factory := computemocks.NewMockFactory(gCtrl)
			helper := clientmocks.NewMockHelper(gCtrl)
			watches := &recordedWatches{}
			r, apiHandler := setupReconciler(t, testParams{
				gCtrl: gCtrl, factory: factory, helper: helper, kueueWatches: watches,
			})

			if tt.cached != nil {
				r.clusterDataMap.add(_testClusterName, &Data{cachedObj: tt.cached, clusterStatus: newReadyStatus()})
			}
			apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
				gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
					*obj = *tt.current.DeepCopy()
					return nil
				})
			if tt.wantInvalidate {
				factory.EXPECT().Invalidate(&tt.cached.Spec)
			}
			if tt.current.DeletionTimestamp != nil {
				// the finalizer is removed
				apiHandler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			} else {
				// the added or refreshed cluster is probed
				factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
				helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).Return(newReadyStatus(), nil)
				apiHandler.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			}

			_, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: _testClusterName, Namespace: constants.ClustersNamespace},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantCalls, watches.get())
		})
	}
}

func TestUpdateClusterHealthRewatchesInvalidatedClient(t *testing.T) {
	tests := []struct {
		name      string
		probeErr  error
		wantCalls []string
	}{
		{
			name:      "token rejected",
			probeErr:  apierrors.NewUnauthorized("token rejected"),
			wantCalls: []string{"rewatch cluster-1"},
		},
		{
			name:      "unverified certificate",
			probeErr:  newX509Error(x509.UnknownAuthorityError{}),
			wantCalls: []string{"rewatch cluster-1"},
		},
		{
			name:     "unreachable",
			probeErr: errors.New("dial tcp: connect: connection refused"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gCtrl := gomock.NewController(t)
			factory := computemocks.NewMockFactory(gCtrl)
			helper := clientmocks.NewMockHelper(gCtrl)
			watches := &recordedWatches{}
			r, apiHandler := setupReconciler(t, testParams{
				gCtrl: gCtrl, factory: factory, helper: helper, kueueWatches: watches,
			})

			cluster, data := newReadyTestCluster()
			cluster.Spec.SchedulerType = v2pb.SCHEDULER_TYPE_KUEUE
			r.clusterDataMap.add(_testClusterName, data)

			factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
			helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).Return(newOfflineStatus(tt.probeErr), tt.probeErr)
			if tt.wantCalls != nil {
				// the rewatch goes with the client invalidation
				factory.EXPECT().Invalidate(&cluster.Spec)
			}
			apiHandler.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			require.Error(t, r.updateClusterHealth(_testClusterName))
			assert.Equal(t, tt.wantCalls, watches.get())
		})
	}
}
