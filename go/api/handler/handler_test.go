package handler

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/michelangelo-ai/michelangelo/go/storage/storagemocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	job1 = v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "job01",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "job01",
		},
	}

	job2 = v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "job02",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "job02",
		},
	}

	job3 = v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "project01",
			Name:      "job03",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "job03",
		},
	}

	initObjs = []ctrlRTClient.Object{
		&job1,
		&job2,
		&job3,
		&v2pb.Project{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "project01",
				Name:      "project01",
			},
			Spec: v2pb.ProjectSpec{
				Owner:       &v2pb.OwnerInfo{OwningTeam: "4b0ff595-0d8c-4081-923c-cc322448c1d5"},
				Description: "michelangelo@uber.com",
				Tier:        3,
				GitRepo:     "repo",
				RootDir:     "root",
			},
		},
	}

	initLists = []ctrlRTClient.ObjectList{
		&v2pb.RayJobList{
			Items: []v2pb.RayJob{
				*initObjs[0].(*v2pb.RayJob),
				*initObjs[1].(*v2pb.RayJob),
				*initObjs[2].(*v2pb.RayJob),
			},
		},
		&v2pb.ProjectList{
			Items: []v2pb.Project{
				*initObjs[3].(*v2pb.Project),
			},
		},
	}
)

func setupK8s() (ctrlRTClient.Client, error) {
	err := v2pb.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithLists(initLists...).
		WithStatusSubresource(initObjs...).Build()

	return fakeClient, nil
}

func TestK8sOnly(t *testing.T) {
	client, err := setupK8s()
	assert.NoError(t, err)
	handler := NewFakeAPIHandler(client)

	baseJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "rayjob100",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "rayjob-100",
		},
	}

	// Create
	createJob := baseJob.DeepCopy()
	err = handler.Create(context.Background(), createJob, &metav1.CreateOptions{})
	assert.NoError(t, err)

	createJobFailed := baseJob.DeepCopy()
	err = handler.Create(context.Background(), createJobFailed, &metav1.CreateOptions{})
	checkGrpcStatusCode(t, codes.AlreadyExists, err)

	// Read
	job := &v2pb.RayJob{}
	err = handler.Get(context.Background(), "default", "rayjob100", nil, job)
	assert.NoError(t, err)
	assert.Equal(t, "rayjob-100", job.Spec.JobId)
	assert.NotNil(t, job.Labels[api.UpdateTimestampLabel])
	assert.NotNil(t, job.Labels[api.SpecUpdateTimestampLabel])
	lastUpdateTimestamp := job.Labels[api.UpdateTimestampLabel]
	lastSpecUpdateTimestamp := job.Labels[api.SpecUpdateTimestampLabel]

	getJobFailed := &v2pb.RayJob{}
	err = handler.Get(context.Background(), "default", "rayjob", nil, getJobFailed)
	checkGrpcStatusCode(t, codes.NotFound, err)

	// Update
	job.Spec.User = &v2pb.UserInfo{
		Name: "test",
	}
	// sleep 2ms before update. So the new update timestamp will be larger than the old one.
	time.Sleep(2 * time.Millisecond)
	err = handler.Update(context.Background(), job, &metav1.UpdateOptions{})
	assert.NoError(t, err)

	updatedJob := &v2pb.RayJob{}
	err = handler.Get(context.Background(), "default", "rayjob100", nil, updatedJob)
	assert.NoError(t, err)
	assert.Equal(t, "test", updatedJob.Spec.User.Name)
	assert.Greater(t, updatedJob.Labels[api.UpdateTimestampLabel], lastUpdateTimestamp)
	assert.Greater(t, updatedJob.Labels[api.SpecUpdateTimestampLabel], lastSpecUpdateTimestamp)
	lastSpecUpdateTimestamp = updatedJob.Labels[api.SpecUpdateTimestampLabel]
	lastUpdateTimestamp = updatedJob.Labels[api.UpdateTimestampLabel]

	// Update non-Spec field and verify that SpecUpdateTimestamp is not changed
	updatedJob.Annotations = map[string]string{"somekey": "somevalue"}
	// sleep 2ms before update. So the new update timestamp will be larger than the old one.
	time.Sleep(2 * time.Millisecond)
	err = handler.Update(context.Background(), updatedJob, &metav1.UpdateOptions{})
	assert.NoError(t, err)
	updatedJob = &v2pb.RayJob{}
	err = handler.Get(context.Background(), "default", "rayjob100", nil, updatedJob)
	assert.NoError(t, err)
	assert.Equal(t, lastSpecUpdateTimestamp, updatedJob.Labels[api.SpecUpdateTimestampLabel])
	assert.Greater(t, updatedJob.Labels[api.UpdateTimestampLabel], lastUpdateTimestamp)

	updateModelFailed := job.DeepCopy()
	updateModelFailed.Name = "NotFound"
	err = handler.Update(context.Background(), updateModelFailed, &metav1.UpdateOptions{})
	checkGrpcStatusCode(t, codes.NotFound, err)

	// UpdateStatus
	getProject := &v2pb.Project{}
	err = handler.Get(context.Background(), "project01", "project01", nil, getProject)
	assert.NoError(t, err)
	getProject.Status = v2pb.ProjectStatus{State: v2pb.PROJECT_STATE_READY}
	err = handler.UpdateStatus(context.Background(), getProject, &metav1.UpdateOptions{
		FieldManager: "testFieldManager",
	})
	assert.NoError(t, err)
	projectAfterUpdate := &v2pb.Project{}
	err = handler.Get(context.Background(), "project01", "project01", nil, projectAfterUpdate)
	assert.NoError(t, err)
	assert.Equal(t, v2pb.PROJECT_STATE_READY, projectAfterUpdate.Status.State)

	// Delete
	deleteJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "job01",
		},
	}
	err = handler.Delete(context.Background(), deleteJob, &metav1.DeleteOptions{})
	assert.NoError(t, err)
	err = handler.Get(context.Background(), "default", "model01", nil, job)
	assert.Error(t, err)

	deleteJobFailed := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "job01",
		},
	}
	err = handler.Delete(context.Background(), deleteJobFailed, &metav1.DeleteOptions{})
	checkGrpcStatusCode(t, codes.NotFound, err)

	// List
	listJobs := &v2pb.RayJobList{}
	err = handler.List(context.Background(), "default", &metav1.ListOptions{}, nil, listJobs)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(listJobs.Items))
	assert.Equal(t, "job02", listJobs.Items[0].Name)

	err = handler.List(context.Background(), "default",
		&metav1.ListOptions{LabelSelector: "bad label selector"}, nil, listJobs)
	checkGrpcStatusCode(t, codes.InvalidArgument, err)

	// ListOptionsExt is not supported
	err = handler.List(context.Background(), "default", &metav1.ListOptions{}, &apipb.ListOptionsExt{
		OrderBy: []*apipb.OrderBy{
			{
				Field: "test",
			},
		},
	}, listJobs)
	assert.Error(t, err)
	checkGrpcStatusCode(t, codes.Unimplemented, err)

	// Delete Collection
	err = handler.DeleteCollection(context.Background(), &v2pb.RayJob{}, "project01", &metav1.DeleteOptions{}, &metav1.ListOptions{})
	assert.NoError(t, err)
	err = handler.List(context.Background(), "project01", &metav1.ListOptions{}, nil, listJobs)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(listJobs.Items))
}

func checkGrpcStatusCode(t *testing.T, expectedCode codes.Code, err error) {
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, expectedCode, grpcStatus.Code())
}

var (
	jobA = v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "project01",
			Name:      "jobA",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "job-A",
		},
	}

	jobB = v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "project01",
			Name:      "jobB",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "job-B",
		},
	}

	jobsInMetadataStorage = []ctrlRTClient.Object{
		&jobA,
		&jobB,
	}
)

func setupMocks(t *testing.T) (*storagemocks.MockMetadataStorage, *storagemocks.MockBlobStorage) {
	ctrl := gomock.NewController(t)
	mockMetadataStorage := storagemocks.NewMockMetadataStorage(ctrl)
	mockBlobStorage := storagemocks.NewMockBlobStorage(ctrl)
	mockBlobStorage.EXPECT().IsObjectInteresting(gomock.Any()).Return(true).AnyTimes()

	for _, obj := range jobsInMetadataStorage {
		mockMetadataStorage.EXPECT().
			GetByName(gomock.Any(), obj.GetNamespace(), obj.GetName(), gomock.Any()).
			Return(nil).AnyTimes().
			Do(func(ctx context.Context, namespace string, name string, object runtime.Object) {
				val := reflect.Indirect(reflect.ValueOf(object))
				val.Set(reflect.Indirect(reflect.ValueOf(obj)))
			})
	}

	return mockMetadataStorage, mockBlobStorage
}

func TestK8sAndMetadataStorage(t *testing.T) {
	k8sClient, err := setupK8s()
	assert.NoError(t, err)

	mockMetadataStorage, mockBlobStorage := setupMocks(t)

	factory := newK8sAndMetadataStorageFactory(Params{
		Scheme: scheme.Scheme,
		StorageConfig: storage.MetadataStorageConfig{
			EnableMetadataStorage:      true,
			DeletionDelay:              0,
			EnableResourceVersionCache: false,
		},
		MetadataStorage: mockMetadataStorage,
		BlobStorage:     mockBlobStorage,
		Logger:          zap.Must(zap.NewDevelopment()),
		Metrics:         tally.NoopScope,
	})
	handler, err := factory.GetAPIHandler(k8sClient)
	assert.NoError(t, err)

	// 1-1. Create a job that is not in either k8s/ETCD or metadata storage. The job will be created in K8s/ETCD.
	newJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "project01",
			Name:      "jobC",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "jobC",
		},
	}
	mockMetadataStorage.EXPECT().
		GetByName(gomock.Any(), newJob.Namespace, newJob.Name, gomock.Any()).
		Return(errors.New("object does not exist")).Times(1)

	err = handler.Create(context.Background(), newJob, &metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create a job that is not in k8s/ETCD, but is already in metadata storage. Expect create to fail.
	err = handler.Create(context.Background(), jobsInMetadataStorage[0], nil)
	checkGrpcStatusCode(t, codes.AlreadyExists, err)

	// Get a job from metadata storage
	j := v2pb.RayJob{}
	mockBlobStorage.EXPECT().MergeWithExternalBlob(gomock.Any(), &jobB).Times(1).Return(nil)
	err = handler.Get(context.Background(), jobB.GetNamespace(), jobB.GetName(), nil, &j)
	assert.NoError(t, err)
	assert.Equal(t, jobB, j)

	// Get a job from k8s/ETCD
	j1 := v2pb.RayJob{}
	err = handler.Get(context.Background(), initObjs[0].GetNamespace(), initObjs[0].GetName(), nil, &j1)
	assert.NoError(t, err)
	j1.ResourceVersion = "" // Remove the resource version set by fake k8s client
	assert.Equal(t, job1, j1)

	// Get a non-existing job
	getModeltmp := &v2pb.RayJob{}
	nonexistentErr := status.Errorf(codes.NotFound, "RayJob namespace=nonexistent AND name=nonexistent not found")
	mockMetadataStorage.EXPECT().
		GetByName(gomock.Any(), "default", "nonexistent", gomock.Any()).
		Return(nonexistentErr).Times(1)

	err = handler.Get(context.Background(), "default", "nonexistent", nil, getModeltmp)
	grpcStatus := status.Convert(err)
	assert.Equal(t, codes.NotFound, grpcStatus.Code())
	assert.ErrorContains(t, err, nonexistentErr.Error())

	// When metadata storage is enabled, List() only returns objects from metadata storage and blob fields are not set
	jobList := &v2pb.RayJobList{}
	mockMetadataStorage.EXPECT().
		List(gomock.Any(), gomock.Any(), "project01", &metav1.ListOptions{}, &apipb.ListOptionsExt{}, gomock.Any()).
		Do(func(ctx context.Context, typeMeta *metav1.TypeMeta, namespace string, listOptions *metav1.ListOptions,
			listOptionsExt *apipb.ListOptionsExt, listResponse *storage.ListResponse) {
			assert.Equal(t, typeMeta.Kind, "RayJob")
			listResponse.Items = []runtime.Object{
				&jobA,
				&jobB,
			}
			listResponse.Continue = ""
		}).Return(nil).Times(1)
	err = handler.List(context.Background(), "project01", &metav1.ListOptions{}, &apipb.ListOptionsExt{}, jobList)
	assert.NoError(t, err)
	assert.Len(t, jobList.Items, 2)

	// Delete Collection
	// Delete Collection will list objects in metadata storage
	mockMetadataStorage.EXPECT().
		List(gomock.Any(), gomock.Any(), "project01", &metav1.ListOptions{}, nil, gomock.Any()).
		Do(func(ctx context.Context, typeMeta *metav1.TypeMeta, namespace string, listOptions *metav1.ListOptions,
			listOptionsExt *apipb.ListOptionsExt, listResponse *storage.ListResponse) {
			assert.Equal(t, typeMeta.Kind, "RayJob")
			listResponse.Items = []runtime.Object{
				&jobA,
				&jobB,
				&job3,
			}
			listResponse.Continue = ""
		}).Return(nil).Times(1)
	// Delete jobA: Get UID of jobA
	mockMetadataStorage.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(ctx context.Context, uid string, object runtime.Object) {
			val := reflect.Indirect(reflect.ValueOf(object))
			val.Set(reflect.ValueOf(jobA))
		}).Return(nil).Times(1)
	// Delete jobA: Delete jobA from metadata storage and blob storage
	mockMetadataStorage.EXPECT().Delete(gomock.Any(), gomock.Any(), "project01", "jobA").Return(nil).Times(1)
	mockBlobStorage.EXPECT().DeleteFromBlobStorage(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	// Delete jobB: Get UID of jobB
	mockMetadataStorage.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(ctx context.Context, uid string, object runtime.Object) {
			val := reflect.Indirect(reflect.ValueOf(object))
			val.Set(reflect.ValueOf(jobB))
		}).Return(nil).Times(1)
	// Delete jobB: Delete jobB from metadata storage and blob storage
	mockMetadataStorage.EXPECT().Delete(gomock.Any(), gomock.Any(), "project01", "jobB").Return(nil).Times(1)
	mockBlobStorage.EXPECT().DeleteFromBlobStorage(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	err = handler.DeleteCollection(context.Background(), &v2pb.RayJob{}, "project01", &metav1.DeleteOptions{}, &metav1.ListOptions{})
	assert.NoError(t, err)
	// job3 is marked as deleting
	j3 := v2pb.RayJob{}
	err = handler.Get(context.Background(), job3.Namespace, job3.Name, nil, &j3)
	assert.NoError(t, err)
	assert.True(t, utils.IsDeleting(&j3))
}

func TestNewAPIServerHandler(t *testing.T) {
	err := v2pb.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	// check the dialer is called and the host address is correctly passed to the dialer
	testDialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		assert.Equal(t, "test.host:80", address)
		return nil, errors.New("test dialer failure")
	}

	handler, err := newAPIServerHandler(Params{
		K8sRestConfig: &rest.Config{
			Host:    "test.host",
			Timeout: 0,
			Dial:    testDialer,
		},
		Scheme: scheme.Scheme,
		StorageConfig: storage.MetadataStorageConfig{
			EnableMetadataStorage:      false,
			DeletionDelay:              0,
			EnableResourceVersionCache: false,
		},
		Logger:  zap.Must(zap.NewDevelopment()),
		Metrics: tally.NoopScope,
	})
	assert.NoError(t, err)
	job := v2pb.RayJob{}
	err = handler.Get(context.Background(), "project0", "job01", &metav1.GetOptions{}, &job)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "test dialer failure"))
}
