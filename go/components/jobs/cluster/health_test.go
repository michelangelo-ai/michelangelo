package cluster

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

func newReadyStatus() *v2pb.ClusterStatus {
	return &v2pb.ClusterStatus{
		StatusConditions: []*apipb.Condition{
			{Type: constants.ClusterReady, Status: apipb.CONDITION_STATUS_TRUE},
		},
	}
}

// newOfflineStatus is the status the health probe returns with an error.
func newOfflineStatus(err error) *v2pb.ClusterStatus {
	return &v2pb.ClusterStatus{
		StatusConditions: []*apipb.Condition{
			{
				Type:    constants.ClusterOffline,
				Status:  apipb.CONDITION_STATUS_TRUE,
				Reason:  client.ClusterNotReachableReason,
				Message: fmt.Sprintf("%s: %v", client.ClusterNotReachableMsg, err),
			},
		},
	}
}

// newX509Error is the error a probe returns when the server certificate does
// not verify against the cluster's CA.
func newX509Error(err error) error {
	return &url.Error{
		Op:  "Get",
		URL: "https://host.docker.internal:6443/healthz",
		Err: &tls.CertificateVerificationError{Err: err},
	}
}

// newReadyTestCluster returns the test cluster, cached as Ready.
func newReadyTestCluster() (*v2pb.Cluster, *Data) {
	cluster := _testCluster.DeepCopy()
	cluster.Finalizers = []string{_clusterDeleteFinalizer}
	cluster.Status = *newReadyStatus()
	return cluster, &Data{cachedObj: cluster, clusterStatus: newReadyStatus()}
}

func TestFailedProbeStatus(t *testing.T) {
	probeErr := errors.New("dial tcp: connect: connection refused")
	now := metav1.Now()
	connectedNotReady := &v2pb.ClusterStatus{
		StatusConditions: []*apipb.Condition{
			{Type: constants.ClusterReady, Status: apipb.CONDITION_STATUS_FALSE},
			{Type: constants.ClusterOffline, Status: apipb.CONDITION_STATUS_FALSE},
		},
	}

	tests := []struct {
		name          string
		status        *v2pb.ClusterStatus
		wantSame      bool
		wantSynthesis bool
	}{
		{name: "keeps the probe's offline status", status: newOfflineStatus(probeErr), wantSame: true},
		{name: "keeps a connected but not Ready status", status: connectedNotReady, wantSame: true},
		{name: "synthesizes a status when the client could not be built", status: nil, wantSynthesis: true},
		{name: "synthesizes a status for an empty one", status: &v2pb.ClusterStatus{}, wantSynthesis: true},
		{name: "replaces a status that claims Ready", status: newReadyStatus(), wantSynthesis: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := failedProbeStatus(tt.status, probeErr, now)
			assert.False(t, isClusterReady(got))
			if tt.wantSame {
				assert.Same(t, tt.status, got)
			}
			if tt.wantSynthesis {
				require.Len(t, got.StatusConditions, 1)
				cond := got.StatusConditions[0]
				assert.Equal(t, constants.ClusterOffline, cond.Type)
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, cond.Status)
				assert.Equal(t, client.ClusterNotReachableReason, cond.Reason)
				assert.Equal(t, client.ClusterNotReachableMsg+": "+probeErr.Error(), cond.Message)
				assert.Equal(t, now.Unix(), cond.LastUpdatedTimestamp)
			}
		})
	}
}

func TestShouldInvalidateClient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "no error", err: nil, want: false},
		{name: "token rejected", err: apierrors.NewUnauthorized("token rejected"), want: true},
		{name: "wrapped token rejection", err: fmt.Errorf("probe: %w", apierrors.NewUnauthorized("")), want: true},
		{name: "unknown certificate authority", err: newX509Error(x509.UnknownAuthorityError{}), want: true},
		{name: "expired certificate", err: newX509Error(x509.CertificateInvalidError{Reason: x509.Expired}), want: true},
		{name: "certificate for another host", err: newX509Error(x509.HostnameError{Host: "other"}), want: true},
		{name: "bare x509 error", err: x509.UnknownAuthorityError{}, want: true},
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{}, "", errors.New("denied")), want: false},
		{name: "internal error", err: apierrors.NewInternalError(errors.New("boom")), want: false},
		{name: "connection refused", err: &url.Error{Op: "Get", Err: errors.New("connection refused")}, want: false},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: false},
		{name: "client build error", err: errors.New("client cfg err:secret not found"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldInvalidateClient(tt.err))
		})
	}
}

func TestClientNeedsRebuild(t *testing.T) {
	oldCluster := _testCluster.DeepCopy()
	oldCluster.Spec.GetKubernetes().Rest.TokenTag = "token"
	oldCluster.Spec.GetKubernetes().Rest.CaDataTag = "ca"
	updated := func(update func(*v2pb.ClusterSpec)) *v2pb.ClusterSpec {
		spec := oldCluster.DeepCopy().Spec
		update(&spec)
		return &spec
	}

	tests := []struct {
		name    string
		newSpec *v2pb.ClusterSpec
		want    bool
	}{
		{name: "unchanged", newSpec: updated(func(*v2pb.ClusterSpec) {}), want: false},
		{name: "host", newSpec: updated(func(s *v2pb.ClusterSpec) { s.GetKubernetes().Rest.Host = "https://new" }), want: true},
		{name: "port", newSpec: updated(func(s *v2pb.ClusterSpec) { s.GetKubernetes().Rest.Port = "443" }), want: true},
		{name: "token tag", newSpec: updated(func(s *v2pb.ClusterSpec) { s.GetKubernetes().Rest.TokenTag = "new" }), want: true},
		{name: "CA data tag", newSpec: updated(func(s *v2pb.ClusterSpec) { s.GetKubernetes().Rest.CaDataTag = "new" }), want: true},
		{name: "connection removed", newSpec: updated(func(s *v2pb.ClusterSpec) { s.Cluster = nil }), want: true},
		{name: "scheduler type", newSpec: updated(func(s *v2pb.ClusterSpec) { s.SchedulerType = v2pb.SCHEDULER_TYPE_KUEUE }), want: false},
		{name: "SLA", newSpec: updated(func(s *v2pb.ClusterSpec) { s.Sla = v2pb.SLA_TYPE_PRODUCTION }), want: false},
		{name: "skus", newSpec: updated(func(s *v2pb.ClusterSpec) { s.GetKubernetes().Skus = []*v2pb.SkuConfig{{}} }), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clientNeedsRebuild(&oldCluster.Spec, tt.newSpec))
		})
	}
}

func TestUpdateClusterHealthFailedProbe(t *testing.T) {
	refused := errors.New("dial tcp: connect: connection refused")
	tests := []struct {
		name           string
		setup          func(factory *computemocks.MockFactory, helper *clientmocks.MockHelper, cluster *v2pb.Cluster)
		wantErrMessage string
	}{
		{
			name: "probe error",
			setup: func(factory *computemocks.MockFactory, helper *clientmocks.MockHelper, _ *v2pb.Cluster) {
				factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
				helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).Return(newOfflineStatus(refused), refused)
			},
			wantErrMessage: "connection refused",
		},
		{
			name: "client build error",
			setup: func(factory *computemocks.MockFactory, _ *clientmocks.MockHelper, _ *v2pb.Cluster) {
				factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, errors.New("client cfg err:secret not found"))
			},
			wantErrMessage: "secret not found",
		},
		{
			name: "token rejected invalidates the client",
			setup: func(factory *computemocks.MockFactory, helper *clientmocks.MockHelper, cluster *v2pb.Cluster) {
				rejected := apierrors.NewUnauthorized("token rejected")
				factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
				helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).Return(newOfflineStatus(rejected), rejected)
				factory.EXPECT().Invalidate(&cluster.Spec)
			},
			wantErrMessage: "token rejected",
		},
		{
			name: "unverified certificate invalidates the client",
			setup: func(factory *computemocks.MockFactory, helper *clientmocks.MockHelper, cluster *v2pb.Cluster) {
				unverified := newX509Error(x509.UnknownAuthorityError{})
				factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
				helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).Return(newOfflineStatus(unverified), unverified)
				factory.EXPECT().Invalidate(&cluster.Spec)
			},
			wantErrMessage: "certificate signed by unknown authority",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gCtrl := gomock.NewController(t)
			factory := computemocks.NewMockFactory(gCtrl)
			helper := clientmocks.NewMockHelper(gCtrl)
			r, apiHandler := setupReconciler(t, testParams{gCtrl: gCtrl, factory: factory, helper: helper})

			cluster, data := newReadyTestCluster()
			r.clusterDataMap.add(_testClusterName, data)
			require.Len(t, r.GetClusters(ReadyClusters), 1)

			// Invalidate is expected only where a case sets it up
			tt.setup(factory, helper, cluster)
			var written *v2pb.Cluster
			apiHandler.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, obj ctrlRTClient.Object, _ *metav1.UpdateOptions) error {
					written = obj.(*v2pb.Cluster)
					return nil
				})

			err := r.updateClusterHealth(_testClusterName)
			require.ErrorContains(t, err, tt.wantErrMessage)

			// not Ready, so excluded from Ready clusters until a probe succeeds
			assert.False(t, isClusterReady(data.GetClusterStatus()))
			assert.Empty(t, r.GetClusters(ReadyClusters))
			assert.Len(t, r.GetClusters(UnreadyClusters), 1)
			assert.False(t, isClusterReady(&r.GetCluster(_testClusterName).Status))

			// persisted like a successful probe's status, with the error
			require.NotNil(t, written)
			assert.False(t, isClusterReady(&written.Status))
			require.NotEmpty(t, written.Status.StatusConditions)
			assert.Equal(t, constants.ClusterOffline, written.Status.StatusConditions[0].Type)
			assert.Contains(t, written.Status.StatusConditions[0].Message, tt.wantErrMessage)
		})
	}
}

func TestUpdateClusterHealthReadyAfterSuccessfulProbe(t *testing.T) {
	gCtrl := gomock.NewController(t)
	factory := computemocks.NewMockFactory(gCtrl)
	helper := clientmocks.NewMockHelper(gCtrl)
	r, apiHandler := setupReconciler(t, testParams{gCtrl: gCtrl, factory: factory, helper: helper})

	_, data := newReadyTestCluster()
	r.clusterDataMap.add(_testClusterName, data)

	refused := errors.New("connection refused")
	factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil).Times(2)
	gomock.InOrder(
		helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).Return(newOfflineStatus(refused), refused),
		helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).Return(newReadyStatus(), nil),
	)
	apiHandler.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)

	require.Error(t, r.updateClusterHealth(_testClusterName))
	require.Empty(t, r.GetClusters(ReadyClusters))

	require.NoError(t, r.updateClusterHealth(_testClusterName))
	assert.True(t, isClusterReady(data.GetClusterStatus()))
	require.Len(t, r.GetClusters(ReadyClusters), 1)
	assert.Equal(t, _testClusterName, r.GetClusters(ReadyClusters)[0].Name)
	assert.True(t, isClusterReady(&r.GetCluster(_testClusterName).Status))
}

func TestUpdateClusterHealthWritesPrivateCopy(t *testing.T) {
	gCtrl := gomock.NewController(t)
	factory := computemocks.NewMockFactory(gCtrl)
	helper := clientmocks.NewMockHelper(gCtrl)
	r, apiHandler := setupReconciler(t, testParams{gCtrl: gCtrl, factory: factory, helper: helper})

	cluster, data := newReadyTestCluster()
	cluster.ResourceVersion = "1"
	r.clusterDataMap.add(_testClusterName, data)

	factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil).Times(2)
	helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).Return(newReadyStatus(), nil).Times(2)
	writes := 0
	apiHandler.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, obj ctrlRTClient.Object, _ *metav1.UpdateOptions) error {
			writes++
			// the write gets its own copy, which it may update, and the version the last write returned
			assert.NotSame(t, r.GetCluster(_testClusterName), obj)
			assert.Equal(t, fmt.Sprint(writes), obj.GetResourceVersion())
			obj.SetResourceVersion(fmt.Sprint(writes + 1))
			return nil
		}).Times(2)

	require.NoError(t, r.updateClusterHealth(_testClusterName))
	require.NoError(t, r.updateClusterHealth(_testClusterName))
	assert.Equal(t, "3", r.GetCluster(_testClusterName).ResourceVersion)
	assert.Equal(t, "1", cluster.ResourceVersion, "a handed-out object is never modified")
}

func TestUpdateClusterHealthDropsStaleResult(t *testing.T) {
	tests := []struct {
		name string
		// during runs while the probe is in flight
		during func(r *Reconciler, data *Data)
	}{
		{
			name: "spec refreshed during the probe",
			during: func(_ *Reconciler, data *Data) {
				updated := _testCluster.DeepCopy()
				updated.Spec.SchedulerType = v2pb.SCHEDULER_TYPE_KUEUE
				data.refreshSpec(updated)
			},
		},
		{
			name: "cluster deleted during the probe",
			during: func(r *Reconciler, _ *Data) {
				r.clusterDataMap.delete(_testClusterName)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gCtrl := gomock.NewController(t)
			factory := computemocks.NewMockFactory(gCtrl)
			helper := clientmocks.NewMockHelper(gCtrl)
			// no status write is expected
			r, _ := setupReconciler(t, testParams{gCtrl: gCtrl, factory: factory, helper: helper})

			_, data := newReadyTestCluster()
			r.clusterDataMap.add(_testClusterName, data)

			refused := errors.New("connection refused")
			factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
			helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, rest.Interface) (*v2pb.ClusterStatus, error) {
					tt.during(r, data)
					return newOfflineStatus(refused), refused
				})

			require.Error(t, r.updateClusterHealth(_testClusterName))
			assert.True(t, isClusterReady(data.GetClusterStatus()), "the stale result is not stored")
		})
	}
}

func TestReconcileRefreshesClusterInPlace(t *testing.T) {
	tests := []struct {
		name           string
		update         func(c *v2pb.Cluster)
		wantInvalidate bool
	}{
		{
			name:   "scheduler type change",
			update: func(c *v2pb.Cluster) { c.Spec.SchedulerType = v2pb.SCHEDULER_TYPE_KUEUE },
		},
		{
			name:           "token tag change",
			update:         func(c *v2pb.Cluster) { c.Spec.GetKubernetes().Rest.TokenTag = "token-new" },
			wantInvalidate: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gCtrl := gomock.NewController(t)
			factory := computemocks.NewMockFactory(gCtrl)
			helper := clientmocks.NewMockHelper(gCtrl)
			r, apiHandler := setupReconciler(t, testParams{gCtrl: gCtrl, factory: factory, helper: helper})

			cachedCluster, data := newReadyTestCluster()
			cachedCluster.Spec.SchedulerType = v2pb.SCHEDULER_TYPE_DEFAULT
			cachedCluster.Spec.GetKubernetes().Rest.TokenTag = "token-old"
			oldSpec := cachedCluster.DeepCopy().Spec
			r.clusterDataMap.add(_testClusterName, data)

			updatedCluster := cachedCluster.DeepCopy()
			updatedCluster.Status = v2pb.ClusterStatus{}
			tt.update(updatedCluster)
			apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
				gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
					*obj = *updatedCluster.DeepCopy()
					return nil
				})
			if tt.wantInvalidate {
				// dropped by the old spec, which keys the cached client
				factory.EXPECT().Invalidate(&oldSpec)
			}

			// A monitoring cycle runs concurrently with the refresh, so both
			// probes are expected any number of times.
			var probedMu sync.Mutex
			var probed []*v2pb.Cluster
			factory.EXPECT().GetClientSetForCluster(gomock.Any()).DoAndReturn(
				func(c *v2pb.Cluster) (*compute.ClientSet, error) {
					probedMu.Lock()
					defer probedMu.Unlock()
					probed = append(probed, c)
					return &compute.ClientSet{}, nil
				}).MinTimes(1)
			helper.EXPECT().GetClusterHealth(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, rest.Interface) (*v2pb.ClusterStatus, error) {
					// the refresh probe runs after the swap: the cluster is still
					// there and still Ready
					assert.NotNil(t, r.GetCluster(_testClusterName))
					assert.Len(t, r.GetClusters(ReadyClusters), 1)
					return newReadyStatus(), nil
				}).MinTimes(1)
			apiHandler.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).MinTimes(1)

			// Readers use the objects they got without locking, as callers of
			// the cache do; the reads are kept for after wg.Wait so the race
			// detector sees them.
			done := make(chan struct{})
			var sawNil atomic.Bool
			var readSpec v2pb.ClusterSpec
			var readConditions, readReady int
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				for {
					if c := r.GetCluster(_testClusterName); c != nil {
						readSpec = c.Spec
						readConditions = len(c.Status.StatusConditions)
						readReady = len(r.GetClusters(ReadyClusters))
					} else {
						sawNil.Store(true)
					}
					select {
					case <-done:
						return
					default:
					}
				}
			}()
			go func() {
				defer wg.Done()
				r.updateClusterInfo(context.Background())
			}()

			_, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: _testClusterName, Namespace: constants.ClustersNamespace},
			})
			close(done)
			wg.Wait()
			require.NoError(t, err)

			assert.False(t, sawNil.Load(), "GetCluster returned nil during the refresh")
			assert.NotEmpty(t, readSpec.Region)
			assert.Equal(t, 1, readConditions)
			assert.Equal(t, 1, readReady)
			got := r.GetCluster(_testClusterName)
			require.NotNil(t, got)
			assert.True(t, gogoproto.Equal(&updatedCluster.Spec, &got.Spec), "the cache holds the updated spec")
			require.Len(t, r.GetClusters(ReadyClusters), 1)

			probedMu.Lock()
			defer probedMu.Unlock()
			probedNewSpec := false
			for _, c := range probed {
				probedNewSpec = probedNewSpec || gogoproto.Equal(&c.Spec, &updatedCluster.Spec)
			}
			assert.True(t, probedNewSpec, "the refreshed cluster is probed")
		})
	}
}
