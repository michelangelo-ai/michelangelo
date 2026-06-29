package ingester

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// MockMetadataStorage is a mock implementation of storage.MetadataStorage
type MockMetadataStorage struct {
	mock.Mock
}

func (m *MockMetadataStorage) Upsert(ctx context.Context, object runtime.Object, direct bool, indexedFields []storage.IndexedField) error {
	args := m.Called(ctx, object, direct, indexedFields)
	return args.Error(0)
}

func (m *MockMetadataStorage) GetByName(ctx context.Context, namespace string, name string, object runtime.Object) error {
	args := m.Called(ctx, namespace, name, object)
	return args.Error(0)
}

func (m *MockMetadataStorage) GetByID(ctx context.Context, uid string, object runtime.Object) error {
	args := m.Called(ctx, uid, object)
	return args.Error(0)
}

func (m *MockMetadataStorage) List(ctx context.Context, typeMeta *metav1.TypeMeta, namespace string, listOptions *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, listResponse *storage.ListResponse) error {
	args := m.Called(ctx, typeMeta, namespace, listOptions, listOptionsExt, listResponse)
	return args.Error(0)
}

func (m *MockMetadataStorage) Delete(ctx context.Context, typeMeta *metav1.TypeMeta, namespace string, name string) error {
	args := m.Called(ctx, typeMeta, namespace, name)
	return args.Error(0)
}

func (m *MockMetadataStorage) DeleteCollection(ctx context.Context, namespace string, deleteOptions *metav1.DeleteOptions, listOptions *metav1.ListOptions) error {
	args := m.Called(ctx, namespace, deleteOptions, listOptions)
	return args.Error(0)
}

func (m *MockMetadataStorage) QueryByTemplateID(ctx context.Context, typeMeta *metav1.TypeMeta, templateID string, listOptionsExt *apipb.ListOptionsExt, listResponse *storage.ListResponse) error {
	args := m.Called(ctx, typeMeta, templateID, listOptionsExt, listResponse)
	return args.Error(0)
}

func (m *MockMetadataStorage) Backfill(ctx context.Context, createFn storage.PrepareBackfillParams, opts storage.BackfillOptions) (*time.Time, error) {
	args := m.Called(ctx, createFn, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*time.Time), args.Error(1)
}

func (m *MockMetadataStorage) Close() {
	m.Called()
}

func TestReconciler_HandleSync(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	// Use Deployment (non-immutable kind) to exercise the sync path
	deployment := &v2.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			UID:        types.UID("test-uid"),
			Finalizers: []string{api.IngesterFinalizer},
			Annotations: map[string]string{
				api.MetadataStoragePrimaryKeyAnnotation: "test-uid",
			},
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	// Create mock storage
	mockStorage := new(MockMetadataStorage)
	mockStorage.On("Upsert", mock.Anything, mock.Anything, false, mock.Anything).Return(nil)

	// Create reconciler
	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Deployment{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	// Test reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-deployment",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify that Upsert was called
	mockStorage.AssertCalled(t, "Upsert", mock.Anything, mock.Anything, false, mock.Anything)
}

func TestReconciler_HandleSync_UpsertsWithFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	// Objects always arrive with the finalizer already set — the API handler
	// adds it synchronously during Create() before writing to ETCD.
	deployment := &v2.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			UID:        types.UID("test-uid"),
			Finalizers: []string{api.IngesterFinalizer},
			Annotations: map[string]string{
				api.MetadataStoragePrimaryKeyAnnotation: "test-uid",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	mockStorage := new(MockMetadataStorage)
	mockStorage.On("Upsert", mock.Anything, mock.Anything, false, mock.Anything).Return(nil)

	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Deployment{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-deployment", Namespace: "default"},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Upsert is called immediately — no separate "add finalizer" round trip.
	mockStorage.AssertCalled(t, "Upsert", mock.Anything, mock.Anything, false, mock.Anything)
}

func TestReconciler_HandleDeletion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	now := metav1.Now()
	gracePeriod := int64(0) // Expired

	// Create a test model with deletion timestamp
	model := &v2.Model{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Model",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:                       "test-model",
			Namespace:                  "default",
			UID:                        types.UID("test-uid"),
			DeletionTimestamp:          &now,
			DeletionGracePeriodSeconds: &gracePeriod,
			Finalizers:                 []string{api.IngesterFinalizer},
		},
		Spec: v2.ModelSpec{
			Description: "Test model for deletion",
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		Build()

	// Create mock storage (not called — MySQL deletion is handled via annotation path)
	mockStorage := new(MockMetadataStorage)

	// Create reconciler
	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Model{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	// Test reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-model",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify that storage was NOT called — the DeletionTimestamp path only removes
	// the finalizer; MySQL deletion happens upstream via the DeletingAnnotation path.
	mockStorage.AssertNotCalled(t, "Delete")

	// The finalizer is removed so K8s can garbage-collect the object.
}

func TestReconciler_HandleDeletionAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	// Create a test model with deleting annotation
	model := &v2.Model{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Model",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: "default",
			UID:       types.UID("test-uid"),
			Annotations: map[string]string{
				api.DeletingAnnotation: "true",
			},
			Finalizers: []string{api.IngesterFinalizer},
		},
		Spec: v2.ModelSpec{
			Description: "Test model for annotation deletion",
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		Build()

	// Create mock storage
	mockStorage := new(MockMetadataStorage)
	mockStorage.On("Delete", mock.Anything, mock.Anything, "default", "test-model").Return(nil)

	// Create reconciler
	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Model{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	// Test reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-model",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify that Delete was called
	mockStorage.AssertCalled(t, "Delete", mock.Anything, mock.Anything, "default", "test-model")

	// Verify object was deleted from K8s
	updatedModel := &v2.Model{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-model", Namespace: "default"}, updatedModel)
	assert.True(t, client.IgnoreNotFound(err) == nil, "Object should be deleted from K8s")
}

func TestReconciler_HandleImmutableKind(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	// Model is an immutable kind (IsImmutableKind() returns true), no annotation needed
	model := &v2.Model{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Model",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-model",
			Namespace:  "default",
			UID:        types.UID("test-uid"),
			Finalizers: []string{api.IngesterFinalizer},
			Annotations: map[string]string{
				api.MetadataStoragePrimaryKeyAnnotation: "test-uid",
			},
		},
		Spec: v2.ModelSpec{
			Description: "Test immutable kind model",
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		Build()

	// Create mock storage
	mockStorage := new(MockMetadataStorage)
	mockStorage.On("Upsert", mock.Anything, mock.Anything, false, mock.Anything).Return(nil)

	// Create reconciler
	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Model{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-model",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify Upsert was called (object saved to storage before etcd removal)
	mockStorage.AssertCalled(t, "Upsert", mock.Anything, mock.Anything, false, mock.Anything)

	// Verify object was deleted from K8s/etcd
	updatedModel := &v2.Model{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-model", Namespace: "default"}, updatedModel)
	assert.True(t, client.IgnoreNotFound(err) == nil, "Immutable kind object should be removed from K8s")
}

func TestReconciler_HandleImmutableObject(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	// Create a test model with immutable annotation
	model := &v2.Model{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Model",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: "default",
			UID:       types.UID("test-uid"),
			Annotations: map[string]string{
				api.ImmutableAnnotation:                 "true",
				api.MetadataStoragePrimaryKeyAnnotation: "test-uid",
			},
			Finalizers: []string{api.IngesterFinalizer},
		},
		Spec: v2.ModelSpec{
			Description: "Test immutable model",
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		Build()

	// Create mock storage
	mockStorage := new(MockMetadataStorage)
	mockStorage.On("Upsert", mock.Anything, mock.Anything, false, mock.Anything).Return(nil)

	// Create reconciler
	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Model{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	// Test reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-model",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify that Upsert was called (to save to storage before deletion)
	mockStorage.AssertCalled(t, "Upsert", mock.Anything, mock.Anything, false, mock.Anything)

	// Verify object was deleted from K8s
	updatedModel := &v2.Model{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-model", Namespace: "default"}, updatedModel)
	assert.True(t, client.IgnoreNotFound(err) == nil, "Object should be deleted from K8s")
}

func TestReconciler_ObjectNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	// Create fake client with no objects
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create mock storage (should not be called)
	mockStorage := new(MockMetadataStorage)

	// Create reconciler
	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Model{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	// Test reconcile for non-existent object
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify storage was not called
	mockStorage.AssertNotCalled(t, "Upsert")
	mockStorage.AssertNotCalled(t, "Delete")
}

func TestHelperFunctions(t *testing.T) {
	t.Run("isDeletingAnnotationSet", func(t *testing.T) {
		// Test with annotation set
		obj := &v2.Model{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					api.DeletingAnnotation: "true",
				},
			},
		}
		assert.True(t, isDeletingAnnotationSet(obj))

		// Test with annotation not set
		obj2 := &v2.Model{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		}
		assert.False(t, isDeletingAnnotationSet(obj2))

		// Test with nil annotations
		obj3 := &v2.Model{
			ObjectMeta: metav1.ObjectMeta{},
		}
		assert.False(t, isDeletingAnnotationSet(obj3))
	})

	t.Run("isImmutable", func(t *testing.T) {
		// Test with annotation set
		obj := &v2.Model{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					api.ImmutableAnnotation: "true",
				},
			},
		}
		assert.True(t, isImmutable(obj))

		// Test with annotation not set
		obj2 := &v2.Model{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		}
		assert.False(t, isImmutable(obj2))
	})

	t.Run("isImmutableKind", func(t *testing.T) {
		// Model is an immutable kind
		model := &v2.Model{}
		assert.True(t, isImmutableKind(model))

		// Deployment is not an immutable kind
		deployment := &v2.Deployment{}
		assert.False(t, isImmutableKind(deployment))
	})

	t.Run("getRequeuePeriod", func(t *testing.T) {
		// Test with configured period
		r := &Reconciler{
			config: Config{RequeuePeriod: 60 * time.Second},
		}
		assert.Equal(t, 60*time.Second, r.getRequeuePeriod())

		// Test with default
		r2 := &Reconciler{}
		assert.Equal(t, defaultRequeuePeriod, r2.getRequeuePeriod())
	})
}

// TestSchemeGVKResolution verifies that all CRD objects in CrdObjects resolve to
// unique, non-empty kinds via the scheme.
func TestSchemeGVKResolution(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v2.AddToScheme(scheme))

	seen := map[string]bool{}
	for _, obj := range v2.CrdObjects {
		gvks, _, err := scheme.ObjectKinds(obj)
		require.NoError(t, err, "scheme.ObjectKinds failed for %T", obj)
		require.NotEmpty(t, gvks, "no GVKs found for %T", obj)

		kind := gvks[0].Kind
		assert.NotEmpty(t, kind, "empty kind for %T — GetObjectKind() was likely used instead of scheme", obj)
		assert.False(t, seen[kind], "duplicate kind %q — controller name collision would crash controllermgr", kind)
		seen[kind] = true
	}
}

// TestHandleDeletion_OnlyRemovesFinalizer verifies that handleDeletion does NOT call
// storage.Delete — MySQL deletion is owned by the annotation path. The DeletionTimestamp
// path only removes the ingester finalizer so K8s can garbage-collect the object.
func TestHandleDeletion_OnlyRemovesFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	now := metav1.Now()
	gracePeriod := int64(0)

	model := &v2.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:                       "test-model",
			Namespace:                  "default",
			UID:                        types.UID("test-uid"),
			DeletionTimestamp:          &now,
			DeletionGracePeriodSeconds: &gracePeriod,
			Finalizers:                 []string{api.IngesterFinalizer},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()

	mockStorage := new(MockMetadataStorage)

	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Model{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-model", Namespace: "default"},
	})
	require.NoError(t, err)

	// Storage must not be touched — no MySQL delete on the DeletionTimestamp path.
	mockStorage.AssertNotCalled(t, "Delete")
}

// TestHandleDeletionAnnotation_CorrectTypeMeta verifies the same scheme-based GVK
// resolution in the annotation deletion path.
func TestHandleDeletionAnnotation_CorrectTypeMeta(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	model := &v2.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: "default",
			UID:       types.UID("test-uid"),
			Annotations: map[string]string{
				api.DeletingAnnotation: "true",
			},
			Finalizers: []string{api.IngesterFinalizer},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()

	var capturedTypeMeta *metav1.TypeMeta
	mockStorage := new(MockMetadataStorage)
	mockStorage.On("Delete", mock.Anything, mock.MatchedBy(func(tm *metav1.TypeMeta) bool {
		capturedTypeMeta = tm
		return true
	}), "default", "test-model").Return(nil)

	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Model{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-model", Namespace: "default"},
	})
	require.NoError(t, err)

	require.NotNil(t, capturedTypeMeta)
	assert.Equal(t, "Model", capturedTypeMeta.Kind, "Kind must come from scheme, not empty GetObjectKind()")
	assert.NotEmpty(t, capturedTypeMeta.APIVersion, "APIVersion must come from scheme")
}

// TestDeleteOptionsFromAnnotations verifies the annotation -> []client.DeleteOption
// mapping that threads the caller's delete propagation policy through to the real K8s
// delete. Asserting DeleteOptions through the fake client is awkward, so the mapping is
// unit-tested directly by applying the returned options onto a client.DeleteOptions.
func TestDeleteOptionsFromAnnotations(t *testing.T) {
	foreground := metav1.DeletePropagationForeground

	tests := []struct {
		name        string
		annotations map[string]string
		wantPolicy  *metav1.DeletionPropagation
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			wantPolicy:  nil,
		},
		{
			name:        "no propagation annotation",
			annotations: map[string]string{api.DeletingAnnotation: "true"},
			wantPolicy:  nil,
		},
		{
			name:        "empty propagation annotation",
			annotations: map[string]string{api.DeletePropagationAnnotation: ""},
			wantPolicy:  nil,
		},
		{
			name:        "foreground propagation",
			annotations: map[string]string{api.DeletePropagationAnnotation: string(foreground)},
			wantPolicy:  &foreground,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &v2.Model{ObjectMeta: metav1.ObjectMeta{Annotations: tc.annotations}}
			opts := deleteOptionsFromAnnotations(obj)

			applied := (&client.DeleteOptions{}).ApplyOptions(opts)
			if tc.wantPolicy == nil {
				assert.Empty(t, opts)
				assert.Nil(t, applied.PropagationPolicy)
				return
			}
			require.Len(t, opts, 1)
			require.NotNil(t, applied.PropagationPolicy)
			assert.Equal(t, *tc.wantPolicy, *applied.PropagationPolicy)
		})
	}
}

// TestReconciler_HandleDeletionAnnotation_HonorsPropagationPolicy verifies the end-to-end
// path: an object carrying michelangelo/Deleting=true plus michelangelo/DeletePropagation=
// Foreground causes the ingester to issue the real K8s delete with Foreground propagation.
// A client interceptor captures the DeleteOptions actually passed to the fake client.
func TestReconciler_HandleDeletionAnnotation_HonorsPropagationPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	model := &v2.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: "default",
			UID:       types.UID("test-uid"),
			Annotations: map[string]string{
				api.DeletingAnnotation:          "true",
				api.DeletePropagationAnnotation: string(metav1.DeletePropagationForeground),
			},
			Finalizers: []string{api.IngesterFinalizer},
		},
	}

	var capturedOpts *client.DeleteOptions
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				capturedOpts = (&client.DeleteOptions{}).ApplyOptions(opts)
				return c.Delete(ctx, obj, opts...)
			},
		}).
		Build()

	mockStorage := new(MockMetadataStorage)
	mockStorage.On("Delete", mock.Anything, mock.Anything, "default", "test-model").Return(nil)

	reconciler := NewReconciler(
		fakeClient,
		logr.Discard(),
		scheme,
		&v2.Model{},
		mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-model", Namespace: "default"},
	})
	require.NoError(t, err)

	// The real K8s delete must have been issued with the caller's Foreground policy.
	require.NotNil(t, capturedOpts, "Delete should have been issued")
	require.NotNil(t, capturedOpts.PropagationPolicy, "PropagationPolicy must be threaded from the annotation")
	assert.Equal(t, metav1.DeletePropagationForeground, *capturedOpts.PropagationPolicy)

	mockStorage.AssertCalled(t, "Delete", mock.Anything, mock.Anything, "default", "test-model")
}

// TestReconciler_SetsMetadataStoragePrimaryKey_WhenAbsent verifies that on first reconcile
// of a new object (no MetadataStoragePrimaryKeyAnnotation), the ingester patches the
// annotation to the object's UID and returns early without calling Upsert. The watch on the
// updated object triggers a second reconcile that proceeds to upsert.
func TestReconciler_SetsMetadataStoragePrimaryKey_WhenAbsent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	deployment := &v2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			UID:        types.UID("original-uid"),
			Finalizers: []string{api.IngesterFinalizer},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
	mockStorage := new(MockMetadataStorage)

	reconciler := NewReconciler(
		fakeClient, logr.Discard(), scheme, &v2.Deployment{}, mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-deployment", Namespace: "default"}}
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Upsert must NOT be called on the first pass (returned early after patching).
	mockStorage.AssertNotCalled(t, "Upsert")

	// Annotation must be persisted to k8s with the object's UID as value.
	updated := &v2.Deployment{}
	require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-deployment", Namespace: "default"}, updated))
	assert.Equal(t, "original-uid", updated.GetAnnotations()[api.MetadataStoragePrimaryKeyAnnotation])
}

// TestReconciler_SkipsMetadataStoragePrimaryKey_WhenPresent verifies that when the
// annotation is already set (second reconcile or migrated resource), the ingester skips
// the patch and proceeds directly to upsert.
func TestReconciler_SkipsMetadataStoragePrimaryKey_WhenPresent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	deployment := &v2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			UID:        types.UID("new-uid"),
			Finalizers: []string{api.IngesterFinalizer},
			Annotations: map[string]string{
				api.MetadataStoragePrimaryKeyAnnotation: "original-uid",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
	mockStorage := new(MockMetadataStorage)
	mockStorage.On("Upsert", mock.Anything, mock.Anything, false, mock.Anything).Return(nil)

	reconciler := NewReconciler(
		fakeClient, logr.Discard(), scheme, &v2.Deployment{}, mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-deployment", Namespace: "default"}}
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Upsert must be called (annotation already present, no patch needed).
	mockStorage.AssertCalled(t, "Upsert", mock.Anything, mock.Anything, false, mock.Anything)
}

// TestReconciler_MetadataStoragePrimaryKey_UpdateError verifies that a k8s Update failure
// while setting the annotation is surfaced as an error and triggers a requeue.
func TestReconciler_MetadataStoragePrimaryKey_UpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2.AddToScheme(scheme)

	deployment := &v2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			UID:        types.UID("original-uid"),
			Finalizers: []string{api.IngesterFinalizer},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				return fmt.Errorf("simulated update failure")
			},
		}).
		Build()
	mockStorage := new(MockMetadataStorage)

	reconciler := NewReconciler(
		fakeClient, logr.Discard(), scheme, &v2.Deployment{}, mockStorage,
		WithConfig(Config{ConcurrentReconciles: 1, RequeuePeriod: 30 * time.Second}),
	)

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-deployment", Namespace: "default"}}
	result, err := reconciler.Reconcile(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, 30*time.Second, result.RequeueAfter)
	mockStorage.AssertNotCalled(t, "Upsert")
}
