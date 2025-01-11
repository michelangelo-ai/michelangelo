package handler

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var initObjs = []ctrlRTClient.Object{
	&v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "job01",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "job01",
		},
	},
	&v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "job02",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "job02",
		},
	},
	&v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "job03",
		},
		Spec: v2pb.RayJobSpec{
			JobId: "job03",
		},
	},
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

var initLists = []ctrlRTClient.ObjectList{
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

func newK8sOnlyHandler(k8sClinet ctrlRTClient.Client) (api.Handler, error) {
	return newK8sOnlyFactory(Params{
		Logger:  logr.Logger{},
		Metrics: tally.NoopScope,
	}).GetAPIHandler(k8sClinet)
}

func TestK8sOnly(t *testing.T) {
	client, err := setupK8s()
	assert.NoError(t, err)
	handler, err := newK8sOnlyHandler(client)
	assert.NoError(t, err)

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
	assert.Equal(t, 3, len(listJobs.Items))
	assert.Equal(t, "job02", listJobs.Items[0].Name)

	err = handler.List(context.Background(), "default",
		&metav1.ListOptions{LabelSelector: "bad label selector"}, nil, listJobs)
	checkGrpcStatusCode(t, codes.InvalidArgument, err)

	// Delete Collection
	err = handler.DeleteCollection(context.Background(), &v2pb.RayJob{}, "project01", &metav1.DeleteOptions{}, &metav1.ListOptions{})
	assert.Nil(t, err)
	err = handler.List(context.Background(), "project01", &metav1.ListOptions{}, nil, listJobs)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(listJobs.Items))
}

func checkGrpcStatusCode(t *testing.T, expectedCode codes.Code, err error) {
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, expectedCode, grpcStatus.Code())
}
