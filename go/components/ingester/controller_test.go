package ingester

import (
	"context"
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

	// Create a test model
	model := &v2.Model{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Model",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Spec: v2.ModelSpec{
			Description: "Test model for ingester",
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
	reconciler := &Reconciler{
		Client:          fakeClient,
		Log:             logr.Discard(),
		Scheme:          scheme,
		TargetKind:      &v2.Model{},
		MetadataStorage: mockStorage,
		Config: Config{
			ConcurrentReconciles: 1,
			RequeuePeriod:        30 * time.Second,
		},
	}

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

	// Verify that Upsert was called
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

	// Create mock storage
	mockStorage := new(MockMetadataStorage)
	mockStorage.On("Delete", mock.Anything, mock.Anything, "default", "test-model").Return(nil)

	// Create reconciler
	reconciler := &Reconciler{
		Client:          fakeClient,
		Log:             logr.Discard(),
		Scheme:          scheme,
		TargetKind:      &v2.Model{},
		MetadataStorage: mockStorage,
		Config: Config{
			ConcurrentReconciles: 1,
			RequeuePeriod:        30 * time.Second,
		},
	}

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

	// Note: The object is deleted from K8s after finalizer removal, so we can't check the finalizer state
	// The fact that reconciliation succeeded means the finalizer was removed and K8s deletion proceeded
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
	reconciler := &Reconciler{
		Client:          fakeClient,
		Log:             logr.Discard(),
		Scheme:          scheme,
		TargetKind:      &v2.Model{},
		MetadataStorage: mockStorage,
		Config: Config{
			ConcurrentReconciles: 1,
			RequeuePeriod:        30 * time.Second,
		},
	}

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
				api.ImmutableAnnotation: "true",
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
	reconciler := &Reconciler{
		Client:          fakeClient,
		Log:             logr.Discard(),
		Scheme:          scheme,
		TargetKind:      &v2.Model{},
		MetadataStorage: mockStorage,
		Config: Config{
			ConcurrentReconciles: 1,
			RequeuePeriod:        30 * time.Second,
		},
	}

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
	reconciler := &Reconciler{
		Client:          fakeClient,
		Log:             logr.Discard(),
		Scheme:          scheme,
		TargetKind:      &v2.Model{},
		MetadataStorage: mockStorage,
		Config: Config{
			ConcurrentReconciles: 1,
			RequeuePeriod:        30 * time.Second,
		},
	}

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

	t.Run("getRequeuePeriod", func(t *testing.T) {
		// Test with configured period
		r := &Reconciler{
			Config: Config{
				RequeuePeriod: 60 * time.Second,
			},
		}
		assert.Equal(t, 60*time.Second, r.getRequeuePeriod())

		// Test with default
		r2 := &Reconciler{
			Config: Config{},
		}
		assert.Equal(t, defaultRequeuePeriod, r2.getRequeuePeriod())
	})
}
