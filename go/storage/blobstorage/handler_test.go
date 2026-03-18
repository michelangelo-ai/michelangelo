package blobstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// --- in-memory BlobStoreClient for testing ---

type memClient struct {
	scheme  string
	objects map[string][]byte
}

func newMemClient(scheme string) *memClient {
	return &memClient{scheme: scheme, objects: make(map[string][]byte)}
}

func (m *memClient) Get(_ context.Context, uri string) ([]byte, error) {
	data, ok := m.objects[uri]
	if !ok {
		return nil, fmt.Errorf("not found: %s", uri)
	}
	return data, nil
}

func (m *memClient) Put(_ context.Context, uri string, data []byte) error {
	m.objects[uri] = data
	return nil
}

func (m *memClient) Delete(_ context.Context, uri string) error {
	delete(m.objects, uri)
	return nil
}

func (m *memClient) Scheme() string { return m.scheme }

// --- MockMetadataStorage ---

type mockMetadataStorage struct {
	mock.Mock
}

func (m *mockMetadataStorage) Upsert(ctx context.Context, obj runtime.Object, direct bool, fields []storage.IndexedField) error {
	return m.Called(ctx, obj, direct, fields).Error(0)
}

func (m *mockMetadataStorage) GetByID(ctx context.Context, uid string, obj runtime.Object) error {
	return m.Called(ctx, uid, obj).Error(0)
}

func (m *mockMetadataStorage) GetByName(ctx context.Context, ns, name string, obj runtime.Object) error {
	return m.Called(ctx, ns, name, obj).Error(0)
}

func (m *mockMetadataStorage) Delete(ctx context.Context, typeMeta *metav1.TypeMeta, ns, name string) error {
	return m.Called(ctx, typeMeta, ns, name).Error(0)
}

func (m *mockMetadataStorage) DeleteCollection(ctx context.Context, ns string, delOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error {
	return m.Called(ctx, ns, delOpts, listOpts).Error(0)
}

func (m *mockMetadataStorage) List(ctx context.Context, typeMeta *metav1.TypeMeta, ns string, listOpts *metav1.ListOptions, listOptsExt *apipb.ListOptionsExt, listResp *storage.ListResponse) error {
	return m.Called(ctx, typeMeta, ns, listOpts, listOptsExt, listResp).Error(0)
}

func (m *mockMetadataStorage) QueryByTemplateID(ctx context.Context, typeMeta *metav1.TypeMeta, templateID string, listOptsExt *apipb.ListOptionsExt, listResp *storage.ListResponse) error {
	return m.Called(ctx, typeMeta, templateID, listOptsExt, listResp).Error(0)
}

func (m *mockMetadataStorage) Backfill(ctx context.Context, createFn storage.PrepareBackfillParams, opts storage.BackfillOptions) (*time.Time, error) {
	args := m.Called(ctx, createFn, opts)
	return nil, args.Error(1)
}

func (m *mockMetadataStorage) Close() {}

// --- Helpers ---

func newTestHandler(mem *memClient, cfg Config) *handler {
	store := &blobstore.BlobStore{}
	store.RegisterClient(mem)
	return &handler{store: store, config: cfg}
}

func testModel(name, uid, resVer string) *v2.Model {
	return &v2.Model{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Model",
			APIVersion: "michelangelo.uber.com/v2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       "default",
			UID:             types.UID(uid),
			ResourceVersion: resVer,
		},
	}
}

// --- handler tests ---

func TestHandler_IsObjectInteresting(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{
		BucketName:  "test-bucket",
		EnabledCRDs: map[string]bool{"model": true},
	})

	model := testModel("m1", "uid1", "1")
	assert.True(t, h.IsObjectInteresting(model))

	pipeline := &v2.Pipeline{TypeMeta: metav1.TypeMeta{Kind: "Pipeline", APIVersion: "michelangelo.uber.com/v2"}}
	assert.False(t, h.IsObjectInteresting(pipeline))
}

func TestHandler_IsObjectInteresting_EmptyConfig(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "test-bucket"})

	model := testModel("m1", "uid1", "1")
	assert.False(t, h.IsObjectInteresting(model), "should be false when EnabledCRDs is empty")
}

func TestHandler_UploadToBlobStorage(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "test-bucket", EnabledCRDs: map[string]bool{"model": true}})

	model := testModel("m1", "uid1", "rv1")
	key, err := h.UploadToBlobStorage(context.Background(), model)
	require.NoError(t, err)
	assert.NotEmpty(t, key)

	// Data should be in the store.
	data, ok := mem.objects[key]
	require.True(t, ok, "expected blob to be stored")
	assert.NotEmpty(t, data)

	// Stored JSON should unmarshal back into a Model.
	var stored v2.Model
	require.NoError(t, json.Unmarshal(data, &stored))
	assert.Equal(t, "m1", stored.Name)
}

func TestHandler_MergeWithExternalBlob(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "test-bucket"})

	original := testModel("m1", "uid1", "rv1")
	original.Spec.Description = "original description"

	// Upload first.
	_, err := h.UploadToBlobStorage(context.Background(), original)
	require.NoError(t, err)

	// Now create an empty model with the same UID and merge.
	target := &v2.Model{
		TypeMeta:   original.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "default", UID: "uid1"},
	}
	require.NoError(t, h.MergeWithExternalBlob(context.Background(), target))
	assert.Equal(t, "original description", target.Spec.Description)
}

func TestHandler_MergeWithExternalBlob_NoUID(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "test-bucket"})

	model := &v2.Model{
		TypeMeta:   metav1.TypeMeta{Kind: "Model"},
		ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "default"}, // no UID
	}
	// No UID — should be a no-op.
	require.NoError(t, h.MergeWithExternalBlob(context.Background(), model))
}

func TestHandler_DeleteFromBlobStorage(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "test-bucket"})

	model := testModel("m1", "uid1", "rv1")
	key, err := h.UploadToBlobStorage(context.Background(), model)
	require.NoError(t, err)
	_, exists := mem.objects[key]
	require.True(t, exists)

	require.NoError(t, h.DeleteFromBlobStorage(context.Background(), model))
	_, exists = mem.objects[key]
	assert.False(t, exists, "blob should be deleted")
}

// --- HandleUpdate tests ---

func TestHandleUpdate_BlobStorageNil_CallsUpsert(t *testing.T) {
	ms := new(mockMetadataStorage)
	model := testModel("m1", "uid1", "rv1")
	ms.On("Upsert", mock.Anything, model, false, mock.Anything).Return(nil)

	err := HandleUpdate(context.Background(), model, ms, false, nil, nil)
	require.NoError(t, err)
	ms.AssertCalled(t, "Upsert", mock.Anything, model, false, mock.Anything)
}

func TestHandleUpdate_NotInteresting_CallsUpsert(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "bucket"}) // no EnabledCRDs → nothing interesting
	ms := new(mockMetadataStorage)
	model := testModel("m1", "uid1", "rv1")
	ms.On("Upsert", mock.Anything, model, false, mock.Anything).Return(nil)

	err := HandleUpdate(context.Background(), model, ms, false, nil, h)
	require.NoError(t, err)
	ms.AssertCalled(t, "Upsert", mock.Anything, model, false, mock.Anything)
	assert.Empty(t, mem.objects, "nothing should be uploaded when kind is not enabled")
}

func TestHandleUpdate_Interesting_UploadsAndUpserts(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "bucket", EnabledCRDs: map[string]bool{"model": true}})
	ms := new(mockMetadataStorage)
	model := testModel("m1", "uid1", "rv1")

	ms.On("Upsert", mock.Anything, mock.Anything, false, mock.Anything).Return(nil)

	err := HandleUpdate(context.Background(), model, ms, false, nil, h)
	require.NoError(t, err)
	assert.NotEmpty(t, mem.objects, "object should be uploaded to blob storage")
	ms.AssertCalled(t, "Upsert", mock.Anything, mock.Anything, false, mock.Anything)
}

// --- HandleDelete tests ---

func TestHandleDelete_BlobStorageNil_CallsDelete(t *testing.T) {
	ms := new(mockMetadataStorage)
	model := testModel("m1", "uid1", "rv1")
	typeMeta := &metav1.TypeMeta{Kind: "Model", APIVersion: "michelangelo.uber.com/v2"}
	ms.On("Delete", mock.Anything, typeMeta, "default", "m1").Return(nil)

	err := HandleDelete(context.Background(), typeMeta, model, ms, nil)
	require.NoError(t, err)
	ms.AssertCalled(t, "Delete", mock.Anything, typeMeta, "default", "m1")
}

func TestHandleDelete_Interesting_DeletesFromBothStores(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "bucket", EnabledCRDs: map[string]bool{"model": true}})
	ms := new(mockMetadataStorage)

	model := testModel("m1", "uid1", "rv1")
	typeMeta := &metav1.TypeMeta{Kind: "Model", APIVersion: "michelangelo.uber.com/v2"}

	// Pre-upload so there is something to delete.
	_, err := h.UploadToBlobStorage(context.Background(), model)
	require.NoError(t, err)
	require.NotEmpty(t, mem.objects)

	ms.On("Delete", mock.Anything, typeMeta, "default", "m1").Return(nil)

	err = HandleDelete(context.Background(), typeMeta, model, ms, h)
	require.NoError(t, err)
	ms.AssertCalled(t, "Delete", mock.Anything, typeMeta, "default", "m1")
	assert.Empty(t, mem.objects, "blob should be deleted from blob storage")
}

// --- helpers for PipelineRun blob-field tests ---

func testPipelineRun(name, uid, resVer string) *v2.PipelineRun {
	return &v2.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PipelineRun",
			APIVersion: "michelangelo.api/v2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       "default",
			UID:             types.UID(uid),
			ResourceVersion: resVer,
		},
	}
}

// TestHandler_MergeWithExternalBlob_BlobFieldObject verifies that when the stored object
// implements ObjectWithBlobFields, MergeWithExternalBlob uses FillBlobFields instead of
// a full JSON unmarshal so that ETCD/MySQL metadata (e.g. ResourceVersion) is preserved.
func TestHandler_MergeWithExternalBlob_BlobFieldObject_PreservesMetadata(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "test-bucket", EnabledCRDs: map[string]bool{"pipelinerun": true}})

	// Build original PipelineRun with blob fields populated.
	original := testPipelineRun("pr1", "uid1", "rv1")
	original.Status.Steps = []*v2.PipelineRunStepInfo{
		{Name: "step1", State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED},
		{Name: "step2", State: v2.PIPELINE_RUN_STEP_STATE_FAILED, Message: "oom"},
	}

	_, err := h.UploadToBlobStorage(context.Background(), original)
	require.NoError(t, err)

	// Simulate what ETCD returns: same object but Steps cleared, newer ResourceVersion.
	target := testPipelineRun("pr1", "uid1", "rv2")
	// Steps is nil — as if ClearBlobFields ran before the ETCD write.

	require.NoError(t, h.MergeWithExternalBlob(context.Background(), target))

	// Blob fields should be restored from S3.
	require.Len(t, target.Status.Steps, 2)
	assert.Equal(t, "step1", target.Status.Steps[0].Name)
	assert.Equal(t, v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED, target.Status.Steps[0].State)
	assert.Equal(t, "step2", target.Status.Steps[1].Name)
	assert.Equal(t, "oom", target.Status.Steps[1].Message)

	// ETCD metadata must NOT be overwritten by the blob copy.
	assert.Equal(t, "rv2", target.ResourceVersion, "ResourceVersion from ETCD must be preserved")
	assert.Equal(t, "pr1", target.Name)
}

// TestHandler_MergeWithExternalBlob_BlobFieldObject_WithInput verifies that step
// Input/Output structs stored in blob are restored correctly.
func TestHandler_MergeWithExternalBlob_BlobFieldObject_WithInputOutput(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "test-bucket", EnabledCRDs: map[string]bool{"pipelinerun": true}})

	original := testPipelineRun("pr2", "uid2", "rv1")
	original.Status.Steps = []*v2.PipelineRunStepInfo{
		{
			Name:  "train",
			State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
			Output: &pbtypes.Struct{
				Fields: map[string]*pbtypes.Value{
					"accuracy": {Kind: &pbtypes.Value_StringValue{StringValue: "0.95"}},
				},
			},
			Input: &pbtypes.Struct{
				Fields: map[string]*pbtypes.Value{
					"lr": {Kind: &pbtypes.Value_StringValue{StringValue: "0.01"}},
				},
			},
		},
	}

	_, err := h.UploadToBlobStorage(context.Background(), original)
	require.NoError(t, err)

	target := testPipelineRun("pr2", "uid2", "rv3")

	require.NoError(t, h.MergeWithExternalBlob(context.Background(), target))

	require.Len(t, target.Status.Steps, 1)
	step := target.Status.Steps[0]
	require.NotNil(t, step.Output)
	assert.Equal(t, "0.95", step.Output.Fields["accuracy"].GetStringValue())
	require.NotNil(t, step.Input)
	assert.Equal(t, "0.01", step.Input.Fields["lr"].GetStringValue())

	// Metadata preserved.
	assert.Equal(t, "rv3", target.ResourceVersion)
}

// TestHandler_MergeWithExternalBlob_PlainObject_FullUnmarshal verifies that for a plain
// object (no ObjectWithBlobFields), a full JSON unmarshal is performed.
func TestHandler_MergeWithExternalBlob_PlainObject_FullUnmarshal(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "test-bucket"})

	original := testModel("m1", "uid1", "rv1")
	original.Spec.Description = "from blob"

	_, err := h.UploadToBlobStorage(context.Background(), original)
	require.NoError(t, err)

	target := &v2.Model{
		TypeMeta:   original.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "default", UID: "uid1"},
	}
	require.NoError(t, h.MergeWithExternalBlob(context.Background(), target))
	assert.Equal(t, "from blob", target.Spec.Description)
}

// TestHandler_ClearBlobFields_Steps verifies that ClearBlobFields clears Input and Output
// from each step so those large Struct fields are not persisted in ETCD/MySQL.
// Step metadata (name, state, etc.) is kept so ETCD retains lightweight step status.
func TestHandler_ClearBlobFields_Steps(t *testing.T) {
	pr := testPipelineRun("pr1", "uid1", "rv1")
	pr.Status.Steps = []*v2.PipelineRunStepInfo{
		{
			Name:  "step1",
			State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
			Input: &pbtypes.Struct{
				Fields: map[string]*pbtypes.Value{
					"lr": {Kind: &pbtypes.Value_StringValue{StringValue: "0.01"}},
				},
			},
			Output: &pbtypes.Struct{
				Fields: map[string]*pbtypes.Value{
					"accuracy": {Kind: &pbtypes.Value_StringValue{StringValue: "0.95"}},
				},
			},
		},
	}
	pr.Status.Conditions = []*apipb.Condition{
		{Type: "Ready", Message: "all good"},
	}

	pr.ClearBlobFields()

	// Step metadata must survive so ETCD retains lightweight step status.
	require.Len(t, pr.Status.Steps, 1)
	assert.Equal(t, "step1", pr.Status.Steps[0].Name)
	assert.Equal(t, v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED, pr.Status.Steps[0].State)
	// Large Struct payloads must be cleared before the ETCD write.
	assert.Nil(t, pr.Status.Steps[0].Input, "Input must be cleared by ClearBlobFields")
	assert.Nil(t, pr.Status.Steps[0].Output, "Output must be cleared by ClearBlobFields")
	assert.Nil(t, pr.Status.Conditions, "Conditions must be cleared by ClearBlobFields")
}

// TestHandleUpdate_PipelineRun_StepInputOutputClearedInMySQL_RestoredOnGet is an
// end-to-end integration test for the blob-storage write+read cycle:
//
//  1. HandleUpdate writes a PipelineRun with step Input/Output to blob storage and MySQL.
//     The version written to MySQL must have Input/Output cleared (blob fields only in S3).
//  2. A simulated API GET reads the MySQL version (cleared fields) and calls
//     MergeWithExternalBlob to restore Input/Output from S3.
//  3. The final result must contain the original Input/Output values.
func TestHandleUpdate_PipelineRun_StepInputOutputClearedInMySQL_RestoredOnGet(t *testing.T) {
	mem := newMemClient("s3")
	h := newTestHandler(mem, Config{BucketName: "test-bucket", EnabledCRDs: map[string]bool{"pipelinerun": true}})
	ms := new(mockMetadataStorage)

	// Build a PipelineRun whose steps carry large Input/Output payloads.
	pr := testPipelineRun("pr-e2e", "uid-e2e", "rv1")
	pr.Status.Steps = []*v2.PipelineRunStepInfo{
		{
			Name:  "train",
			State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
			Input: &pbtypes.Struct{
				Fields: map[string]*pbtypes.Value{
					"learning_rate": {Kind: &pbtypes.Value_StringValue{StringValue: "0.001"}},
				},
			},
			Output: &pbtypes.Struct{
				Fields: map[string]*pbtypes.Value{
					"accuracy": {Kind: &pbtypes.Value_StringValue{StringValue: "0.97"}},
				},
			},
		},
	}

	// --- Write path ---

	// Capture the object that HandleUpdate passes to MySQL.Upsert.
	var mysqlObj *v2.PipelineRun
	ms.On("Upsert", mock.Anything, mock.Anything, false, mock.Anything).
		Run(func(args mock.Arguments) {
			obj := args.Get(1).(*v2.PipelineRun)
			mysqlObj = obj.DeepCopy()
		}).Return(nil)

	require.NoError(t, HandleUpdate(context.Background(), pr, ms, false, nil, h))

	// ── Assertion 1: blob (S3) has the full object ──────────────────────────────
	require.NotEmpty(t, mem.objects, "full object must be uploaded to S3")

	// Download and unmarshal the blob to verify Input/Output are present in S3.
	var blobKey string
	for k := range mem.objects {
		blobKey = k
	}
	var blobPR v2.PipelineRun
	require.NoError(t, json.Unmarshal(mem.objects[blobKey], &blobPR))
	require.Len(t, blobPR.Status.Steps, 1)
	require.NotNil(t, blobPR.Status.Steps[0].Input, "Input must be present in S3 blob")
	require.NotNil(t, blobPR.Status.Steps[0].Output, "Output must be present in S3 blob")
	assert.Equal(t, "0.001", blobPR.Status.Steps[0].Input.Fields["learning_rate"].GetStringValue())
	assert.Equal(t, "0.97", blobPR.Status.Steps[0].Output.Fields["accuracy"].GetStringValue())

	// ── Assertion 2: MySQL version has steps but NO Input/Output ────────────────
	require.NotNil(t, mysqlObj, "Upsert must have been called")
	require.Len(t, mysqlObj.Status.Steps, 1)
	assert.Equal(t, "train", mysqlObj.Status.Steps[0].Name, "step metadata must survive in MySQL")
	assert.Equal(t, v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED, mysqlObj.Status.Steps[0].State)
	assert.Nil(t, mysqlObj.Status.Steps[0].Input, "Input must NOT be stored in MySQL")
	assert.Nil(t, mysqlObj.Status.Steps[0].Output, "Output must NOT be stored in MySQL")

	// --- Read path (simulated API GET) ---

	// The object retrieved from MySQL has cleared step fields; UID is used to locate the blob.
	fromMySQL := testPipelineRun("pr-e2e", "uid-e2e", "rv2")
	fromMySQL.Status.Steps = []*v2.PipelineRunStepInfo{
		{
			Name:  "train",
			State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
			// Input and Output are absent — as stored in MySQL.
		},
	}

	require.NoError(t, h.MergeWithExternalBlob(context.Background(), fromMySQL))

	// ── Assertion 3: GET response has Input/Output restored from S3 ─────────────
	require.Len(t, fromMySQL.Status.Steps, 1)
	step := fromMySQL.Status.Steps[0]
	require.NotNil(t, step.Input, "Input must be restored from S3 on GET")
	require.NotNil(t, step.Output, "Output must be restored from S3 on GET")
	assert.Equal(t, "0.001", step.Input.Fields["learning_rate"].GetStringValue())
	assert.Equal(t, "0.97", step.Output.Fields["accuracy"].GetStringValue())
	// ETCD metadata (ResourceVersion) must be preserved — not overwritten by blob copy.
	assert.Equal(t, "rv2", fromMySQL.ResourceVersion)
}
