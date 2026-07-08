package ingester

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const drainFinalizer = "pipelineruns.michelangelo.uber.com/drain"

func makeReconciler(t *testing.T, obj client.Object, target client.Object, ms *MockMetadataStorage, retain cascadedelete.RetainPolicy) (*Reconciler, ctrl.Request) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()
	r := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		target,
		ms,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
		WithRetainPolicy(retain),
	)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}}
	return r, req
}

// TestCascadeDeletion_OptedInDraining: an opted-in kind with a non-ingester
// (drain) finalizer is still draining → ingester refreshes MySQL and waits
// (keeps its finalizer, requeues).
func TestCascadeDeletion_OptedInDraining(t *testing.T) {
	now := metav1.NewTime(time.Now())
	run := &v2.PipelineRun{
		TypeMeta: metav1.TypeMeta{APIVersion: "michelangelo.uber.com/v2", Kind: "PipelineRun"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "draining-run",
			Namespace:         "default",
			UID:               types.UID("u1"),
			DeletionTimestamp: &now,
			Finalizers:        []string{api.IngesterFinalizer, drainFinalizer},
		},
	}
	ms := new(MockMetadataStorage)
	ms.On("Upsert", mock.Anything, mock.Anything, false, mock.Anything).Return(nil)
	retain := cascadedelete.NewStaticRetainPolicy("PipelineRun", "TriggerRun")

	r, req := makeReconciler(t, run, &v2.PipelineRun{}, ms, retain)
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	// wait → requeue, ingester finalizer kept.
	assert.NotZero(t, res.RequeueAfter)
	ms.AssertCalled(t, "Upsert", mock.Anything, mock.Anything, false, mock.Anything)

	got := &v2.PipelineRun{}
	require.NoError(t, r.Get(context.Background(), req.NamespacedName, got))
	assert.True(t, ctrlutil.ContainsFinalizer(got, api.IngesterFinalizer), "ingester finalizer must be kept while draining")
}

// TestCascadeDeletion_OptedInDrainComplete: an opted-in kind with no non-ingester
// finalizer (drain complete) → ingester upserts the final state (retain) and then
// removes its finalizer.
func TestCascadeDeletion_OptedInDrainComplete(t *testing.T) {
	now := metav1.NewTime(time.Now())
	run := &v2.PipelineRun{
		TypeMeta: metav1.TypeMeta{APIVersion: "michelangelo.uber.com/v2", Kind: "PipelineRun"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "retained-run",
			Namespace:         "default",
			UID:               types.UID("u2"),
			DeletionTimestamp: &now,
			Finalizers:        []string{api.IngesterFinalizer},
		},
	}
	ms := new(MockMetadataStorage)
	ms.On("Upsert", mock.Anything, mock.Anything, false, mock.Anything).Return(nil)
	retain := cascadedelete.NewStaticRetainPolicy("PipelineRun", "TriggerRun")

	r, req := makeReconciler(t, run, &v2.PipelineRun{}, ms, retain)
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)
	// retain → final state upserted.
	ms.AssertCalled(t, "Upsert", mock.Anything, mock.Anything, false, mock.Anything)
	ms.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	// finalizer removed → object gone from fake client.
	got := &v2.PipelineRun{}
	err = r.Get(context.Background(), req.NamespacedName, got)
	assert.Error(t, err, "object should be deleted once ingester finalizer removed")
}

// TestCascadeDeletion_NonOptedIn: a non-opted-in kind being deleted directly
// (no DeletingAnnotation) → NO upsert and NO delete to MySQL; the ingester just
// removes its finalizer (unchanged behavior). Proves the scoping.
func TestCascadeDeletion_NonOptedIn(t *testing.T) {
	now := metav1.NewTime(time.Now())
	dep := &v2.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "michelangelo.uber.com/v2", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "some-deployment",
			Namespace:         "default",
			UID:               types.UID("u3"),
			DeletionTimestamp: &now,
			Finalizers:        []string{api.IngesterFinalizer, "some.other/finalizer"},
		},
	}
	ms := new(MockMetadataStorage)
	retain := cascadedelete.NewStaticRetainPolicy("PipelineRun", "TriggerRun")

	r, req := makeReconciler(t, dep, &v2.Deployment{}, ms, retain)
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)
	// Non-opted-in: no MySQL upsert and no MySQL delete from the cascade path.
	ms.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	ms.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	got := &v2.Deployment{}
	require.NoError(t, r.Get(context.Background(), req.NamespacedName, got))
	assert.False(t, ctrlutil.ContainsFinalizer(got, api.IngesterFinalizer), "ingester finalizer must be removed")
}

func TestHasNonIngesterFinalizer(t *testing.T) {
	withDrain := &v2.PipelineRun{ObjectMeta: metav1.ObjectMeta{Finalizers: []string{api.IngesterFinalizer, drainFinalizer}}}
	onlyIngester := &v2.PipelineRun{ObjectMeta: metav1.ObjectMeta{Finalizers: []string{api.IngesterFinalizer}}}
	none := &v2.PipelineRun{}
	assert.True(t, hasNonIngesterFinalizer(withDrain))
	assert.False(t, hasNonIngesterFinalizer(onlyIngester))
	assert.False(t, hasNonIngesterFinalizer(none))
}
