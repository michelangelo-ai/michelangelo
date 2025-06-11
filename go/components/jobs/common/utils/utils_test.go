package utils

import (
	"context"
	"fmt"
	"testing"

	"code.uber.internal/base/testing/contextmatcher"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/utils/cloud"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/github.com/michelangelo-ai/michelangelo/go/api/apimock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestIsTerminationInfoSet(t *testing.T) {
	tt := []struct {
		job                 runtime.Object
		validTerminationSet bool
		wantError           bool
		msg                 string
	}{
		{
			job: &v2beta1pb.RayJob{
				Spec: v2beta1pb.RayJobSpec{},
			},
			validTerminationSet: false,
			msg:                 "empty job spec",
		},
		{
			job: &v2beta1pb.RayJob{
				Spec: v2beta1pb.RayJobSpec{
					Termination: &v2beta1pb.TerminationSpec{},
				},
			},
			validTerminationSet: false,
			msg:                 "empty termination spec",
		},
		{
			job: &v2beta1pb.RayJob{
				Spec: v2beta1pb.RayJobSpec{
					Termination: &v2beta1pb.TerminationSpec{
						Type: v2beta1pb.TERMINATION_TYPE_INVALID,
					},
				},
			},
			validTerminationSet: false,
			msg:                 "invalid termination type",
		},
		{
			job: &v2beta1pb.RayJob{
				Spec: v2beta1pb.RayJobSpec{
					Termination: &v2beta1pb.TerminationSpec{
						Type: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
					},
				},
			},
			validTerminationSet: true,
			msg:                 "succeeded termination type",
		},
		{
			job: &v2beta1pb.RayJob{
				Spec: v2beta1pb.RayJobSpec{
					Termination: &v2beta1pb.TerminationSpec{
						Type: v2beta1pb.TERMINATION_TYPE_FAILED,
					},
				},
			},
			validTerminationSet: true,
			msg:                 "failed termination type",
		},
		{
			job: &v2beta1pb.SparkJob{
				Spec: v2beta1pb.SparkJobSpec{},
			},
			validTerminationSet: false,
			msg:                 "empty job spec",
		},
		{
			job: &v2beta1pb.SparkJob{
				Spec: v2beta1pb.SparkJobSpec{
					Termination: &v2beta1pb.TerminationSpec{},
				},
			},
			validTerminationSet: false,
			msg:                 "empty termination spec",
		},
		{
			job: &v2beta1pb.SparkJob{
				Spec: v2beta1pb.SparkJobSpec{
					Termination: &v2beta1pb.TerminationSpec{
						Type: v2beta1pb.TERMINATION_TYPE_INVALID,
					},
				},
			},
			validTerminationSet: false,
			msg:                 "invalid termination type",
		},
		{
			job: &v2beta1pb.SparkJob{
				Spec: v2beta1pb.SparkJobSpec{
					Termination: &v2beta1pb.TerminationSpec{
						Type: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
					},
				},
			},
			validTerminationSet: true,
			msg:                 "succeeded termination type",
		},
		{
			job: &v2beta1pb.SparkJob{
				Spec: v2beta1pb.SparkJobSpec{
					Termination: &v2beta1pb.TerminationSpec{
						Type: v2beta1pb.TERMINATION_TYPE_FAILED,
					},
				},
			},
			validTerminationSet: true,
			msg:                 "failed termination type",
		},
		{
			job:       nil,
			wantError: true,
			msg:       "nil runtime object",
		},
	}

	for _, test := range tt {
		var msgPrefix string
		if _, ok := test.job.(*v2beta1pb.RayJob); ok {
			msgPrefix = "ray"
		}
		if _, ok := test.job.(*v2beta1pb.SparkJob); ok {
			msgPrefix = "spark"
		}

		t.Run(msgPrefix+" "+test.msg, func(t *testing.T) {
			term, err := IsTerminationInfoSet(test.job)
			if test.wantError {
				require.Error(t, err)
				require.Equal(t, "invalid job type", err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, test.validTerminationSet, term)
			}
		})
	}
}

func TestGetProjectNameFromLabels(t *testing.T) {
	tt := []struct {
		labels              map[string]string
		expectedProjectName string
		expectedError       bool
		msg                 string
	}{
		{
			labels: map[string]string{
				"ma/project-name": "test-project",
			},
			expectedProjectName: "test-project",
			msg:                 "found project name in labels",
		},
		{
			labels: map[string]string{
				"random-label": "test-project",
			},
			expectedError: true,
			msg:           "project name not found in labels",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			name, err := GetProjectNameFromLabels(test.labels)
			require.Equal(t, test.expectedProjectName, name)
			require.Equal(t, test.expectedError, err != nil)
		})
	}
}

func TestGetErrorsFromPodStatus(t *testing.T) {
	tt := []struct {
		msg             string
		pod             corev1.Pod
		containerFilter func(containerStatus corev1.ContainerStatus) bool
		expectedError   *v2beta1pb.PodErrors
	}{
		{
			msg: "pod placement timed out",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:    "PlacementTimedOut",
							Status:  corev1.ConditionTrue,
							Reason:  "PlacementTimedOut",
							Message: "Killing the pod as placement timed out after it was admitted",
						},
					},
				},
			},
			containerFilter: func(containerStatus corev1.ContainerStatus) bool { return true },
			expectedError: &v2beta1pb.PodErrors{
				Name:    "test-pod",
				Reason:  "PlacementTimedOut",
				Message: "Killing the pod as placement timed out after it was admitted",
			},
		},
		{
			msg: "container failed",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:    corev1.ContainersReady,
							Status:  corev1.ConditionFalse,
							Reason:  "ContainersReady",
							Message: "Container ray-head failed",
						},
					},
				},
			},
			containerFilter: func(containerStatus corev1.ContainerStatus) bool { return true },
			expectedError: &v2beta1pb.PodErrors{
				Name:    "test-pod",
				Reason:  "ContainersReady",
				Message: "Container ray-head failed",
			},
		},
		{
			msg: "failure condition with no reason is not picked up",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:    "ResourcesPreempted",
							Status:  corev1.ConditionTrue,
							Message: "Pod was preempted",
						},
					},
				},
			},
			containerFilter: func(containerStatus corev1.ContainerStatus) bool { return true },
		},
		{
			msg: "failure condition with expected status is not picked up",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   "NodeMaintenanceDrain",
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			containerFilter: func(containerStatus corev1.ContainerStatus) bool { return true },
		},
		{
			msg: "container zero exit code",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test-container",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 0,
									Message:  "success",
								},
							},
						},
					},
				},
			},
			containerFilter: func(containerStatus corev1.ContainerStatus) bool {
				return true
			},
		},
		{
			msg: "container non root cause exit code",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test-container",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 143,
									Reason:   "Error",
								},
							},
						},
					},
				},
			},
			containerFilter: func(containerStatus corev1.ContainerStatus) bool {
				return true
			},
		},
		{
			msg: "container non-zero exit code",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test-container",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 137,
									Reason:   "OOMKilled",
									Message:  "Out of memory",
								},
							},
						},
					},
				},
			},
			containerFilter: func(containerStatus corev1.ContainerStatus) bool {
				return true
			},
			expectedError: &v2beta1pb.PodErrors{
				Name:          "test-pod",
				ContainerName: "test-container",
				ExitCode:      137,
				Reason:        "OOMKilled",
				Message:       "Out of memory",
			},
		},
		{
			msg: "sidecar container non-zero exit code",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "ray-sidecar-container",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 137,
									Reason:   "OOMKilled",
									Message:  "Out of memory",
								},
							},
						},
					},
				},
			},
			containerFilter: func(containerStatus corev1.ContainerStatus) bool {
				return containerStatus.Name == constants.HeadContainerName || containerStatus.Name == constants.WorkerContainerName
			},
		},
		{
			msg: "main container non-zero exit code",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "ray-head",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 137,
									Reason:   "OOMKilled",
									Message:  "Out of memory",
								},
							},
						},
					},
				},
			},
			containerFilter: func(containerStatus corev1.ContainerStatus) bool {
				return containerStatus.Name == constants.HeadContainerName || containerStatus.Name == constants.WorkerContainerName
			},
			expectedError: &v2beta1pb.PodErrors{
				Name:          "test-pod",
				ContainerName: "ray-head",
				ExitCode:      137,
				Reason:        "OOMKilled",
				Message:       "Out of memory",
			},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			require.Equal(t, test.expectedError, GetErrorFromPodStatus(&test.pod, test.containerFilter))
		})
	}
}

// TestGetDCTypeFromClusterName tests GetDCType
func TestGetDCTypeFromClusterName(t *testing.T) {
	// Test cases
	testCases := []struct {
		clusterName string
		dcType      v2beta1pb.DCType
	}{
		{
			clusterName: "dca11-batch01",
			dcType:      v2beta1pb.DC_TYPE_ON_PREM,
		},
		{
			clusterName: "phx02-prod10",
			dcType:      v2beta1pb.DC_TYPE_ON_PREM,
		},
		{
			clusterName: "phx60-batch01",
			dcType:      v2beta1pb.DC_TYPE_CLOUD_GCP,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.clusterName, func(t *testing.T) {
			require.Equal(t, tc.dcType, GetDCType(cloud.Cluster(tc.clusterName)))
		})
	}
}

// TestGetDCTypeFromRegionProvider tests GetDCType
func TestGetDCTypeFromRegionProvider(t *testing.T) {
	// Test cases
	testCases := []struct {
		clusterName string
		dcType      v2beta1pb.DCType
	}{
		{
			clusterName: "phx-gcp",
			dcType:      v2beta1pb.DC_TYPE_CLOUD_GCP,
		},
		{
			clusterName: "dca-onprem",
			dcType:      v2beta1pb.DC_TYPE_ON_PREM,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.clusterName, func(t *testing.T) {
			require.Equal(t, tc.dcType, GetDCType(cloud.RegionProvider(tc.clusterName)))
		})
	}
}

func TestUpdateStatusWithRetries(t *testing.T) {
	tt := []struct {
		job             client.Object
		applyFunc       func(job client.Object)
		setupController func(g *gomock.Controller) api.Handler
		expectedError   error
		msg             string
	}{
		{
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-ns",
				},
			},
			applyFunc: func(job client.Object) {
				rayJob := job.(*v2beta1pb.RayJob)
				rayJob.Status.HeadNode = &v2beta1pb.RayHeadNodeInfo{
					Ip:         "a.b.c.d",
					ClientPort: 100,
				}
				rayJob.Status.Assignment = &v2beta1pb.AssignmentInfo{
					ResourcePool: "test-pool",
					Cluster:      "test-cluster",
				}
				rayJob.Status.StatusConditions = []*v2beta1pb.Condition{
					{
						Type:   constants.ScheduledCondition,
						Status: v2beta1pb.CONDITION_STATUS_TRUE,
					},
				}
			},
			setupController: func(g *gomock.Controller) api.Handler {
				mockHandler := apimock.NewMockHandler(g)
				// in success case, both calls should be made only once
				mockHandler.EXPECT().Get(contextmatcher.Any(), "test-ns", "test-job", gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
				return mockHandler
			},
			msg: "ray success case",
		},
		{
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-ns",
				},
			},
			applyFunc: func(job client.Object) {
				rayJob := job.(*v2beta1pb.RayJob)
				rayJob.Status.StatusConditions = []*v2beta1pb.Condition{
					{
						Type:   constants.ScheduledCondition,
						Status: v2beta1pb.CONDITION_STATUS_TRUE,
					},
				}
			},
			setupController: func(g *gomock.Controller) api.Handler {
				mockHandler := apimock.NewMockHandler(g)
				// if get fails, update should not be called and the error should not be retried
				mockHandler.EXPECT().Get(contextmatcher.Any(), "test-ns", "test-job", gomock.Any(), gomock.Any()).Return(
					fmt.Errorf("could not get job %w", assert.AnError)).Times(1)
				return mockHandler
			},
			expectedError: assert.AnError,
			msg:           "ray job get job fails",
		},
		{
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-ns",
				},
			},
			applyFunc: func(job client.Object) {
				rayJob := job.(*v2beta1pb.RayJob)
				rayJob.Status.HeadNode = &v2beta1pb.RayHeadNodeInfo{
					Ip:         "a.b.c.d",
					ClientPort: 100,
				}
			},
			setupController: func(g *gomock.Controller) api.Handler {
				mockHandler := apimock.NewMockHandler(g)
				// if update fails with a non-conflict error, it should not be retried
				mockHandler.EXPECT().Get(contextmatcher.Any(), "test-ns", "test-job", gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("could not update status %w", assert.AnError)).Times(1)
				return mockHandler
			},
			expectedError: assert.AnError,
			msg:           "ray job update status fails with non-conflict error",
		},
		{
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-ns",
				},
			},
			applyFunc: func(job client.Object) {
				rayJob := job.(*v2beta1pb.RayJob)
				rayJob.Status.HeadNode = &v2beta1pb.RayHeadNodeInfo{
					Ip:         "a.b.c.d",
					ClientPort: 100,
				}
			},
			setupController: func(g *gomock.Controller) api.Handler {
				mockHandler := apimock.NewMockHandler(g)
				// if update fails with a conflict error, it should be retried 5 times by default retry
				mockHandler.EXPECT().Get(contextmatcher.Any(), "test-ns", "test-job", gomock.Any(), gomock.Any()).Return(nil).Times(5)
				mockHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed with %s", _statusUpdateConflictCode)).Times(5)
				return mockHandler
			},
			expectedError: ErrStatusUpdate,
			msg:           "ray job update status fails with conflict error",
		},
		{
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-ns",
				},
			},
			applyFunc: func(job client.Object) {
				rayJob := job.(*v2beta1pb.RayJob)
				rayJob.Status.HeadNode = &v2beta1pb.RayHeadNodeInfo{
					Ip:         "a.b.c.d",
					ClientPort: 100,
				}
			},
			setupController: func(g *gomock.Controller) api.Handler {
				mockHandler := apimock.NewMockHandler(g)
				// if update fails with a conflict error, it should be retried 5 times by default retry
				mockHandler.EXPECT().Get(contextmatcher.Any(), "test-ns", "test-job", gomock.Any(), gomock.Any()).Return(nil).Times(5)
				mockHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf(
					// actual error from uMonitor log
					"failed resource pool assignment for job name:ma-sp-weixin-250502-221105-c5cfq0h7-00cdc75c" +
						" namespace:rider-structural-pricing err:rpc error: code = Unknown desc = failed to updateStatus API object." +
						" namespace: rider-structural-pricing, name: ma-sp-weixin-250502-221105-c5cfq0h7-00cdc75c: etcdserver: request timed out")).Times(5)
				return mockHandler
			},
			expectedError: ErrStatusUpdate,
			msg:           "ray job update status fails with etcd request time out",
		},
		{
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-ns",
				},
			},
			applyFunc: func(job client.Object) {
				rayJob := job.(*v2beta1pb.RayJob)
				rayJob.Status.HeadNode = &v2beta1pb.RayHeadNodeInfo{
					Ip:         "a.b.c.d",
					ClientPort: 100,
				}
			},
			setupController: func(g *gomock.Controller) api.Handler {
				mockHandler := apimock.NewMockHandler(g)
				// if update fails with a conflict error, it should be retried 5 times by default retry
				mockHandler.EXPECT().Get(contextmatcher.Any(), "test-ns", "test-job", gomock.Any(), gomock.Any()).Return(nil).Times(5)
				mockHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf(
					// actual error from uMonitor log
					"failed resource pool assignment for job name:ma-ra-takami-sato-250504-112755-jmcj8pi4 namespace:optic-eta" +
						" err:code:unavailable message:proxy forward failed: read tcp 10.158.22.88:49406->10.158.23.235:31368: read: connection reset by peer")).Times(5)
				return mockHandler
			},
			expectedError: ErrStatusUpdate,
			msg:           "ray job update status fails with connection reset",
		},
		{
			job: &v2beta1pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-ns",
				},
			},
			applyFunc: func(job client.Object) {
				sparkJob := job.(*v2beta1pb.SparkJob)
				sparkJob.Status.Assignment = &v2beta1pb.AssignmentInfo{
					ResourcePool: "test-pool",
					Cluster:      "test-cluster",
				}
				sparkJob.Status.StatusConditions = []*v2beta1pb.Condition{
					{
						Type:   constants.ScheduledCondition,
						Status: v2beta1pb.CONDITION_STATUS_TRUE,
					},
				}
				sparkJob.Status.ApplicationId = "123"
				sparkJob.Status.JobUrl = "test-url"
			},
			setupController: func(g *gomock.Controller) api.Handler {
				mockHandler := apimock.NewMockHandler(g)
				// in success case, both calls should be made only once
				mockHandler.EXPECT().Get(contextmatcher.Any(), "test-ns", "test-job", gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
				return mockHandler
			},
			msg: "spark success case",
		},
		{
			job: &v2beta1pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-ns",
				},
			},
			applyFunc: func(job client.Object) {
				sparkJob := job.(*v2beta1pb.SparkJob)
				sparkJob.Status.StatusConditions = []*v2beta1pb.Condition{
					{
						Type:   constants.ScheduledCondition,
						Status: v2beta1pb.CONDITION_STATUS_TRUE,
					},
				}
			},
			setupController: func(g *gomock.Controller) api.Handler {
				mockHandler := apimock.NewMockHandler(g)
				// if get fails, update should not be called and the error should not be retried
				mockHandler.EXPECT().Get(contextmatcher.Any(), "test-ns", "test-job", gomock.Any(), gomock.Any()).Return(
					fmt.Errorf("could not get job %w", assert.AnError)).Times(1)
				return mockHandler
			},
			expectedError: assert.AnError,
			msg:           "spark job get job fails",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			c := test.setupController(g)
			err := UpdateStatusWithRetries(context.TODO(), c, test.job, test.applyFunc, nil)
			if test.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.ErrorIs(t, err, test.expectedError)
			}
		})
	}
}

func TestIsRegionalCluster(t *testing.T) {
	tt := []struct {
		name     string
		cluster  *v2beta1pb.Cluster
		expected bool
	}{
		{
			name:     "nil cluster",
			cluster:  nil,
			expected: false,
		},
		{
			name: "zonal cluster",
			cluster: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
				},
			},
			expected: false,
		},
		{
			name: "regional cluster",
			cluster: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
				},
			},
			expected: true,
		},
		{
			name: "cluster with empty zone",
			cluster: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "dca",
					Zone:   "",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
				},
			},
			expected: true,
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			result := IsRegionalCluster(test.cluster)
			require.Equal(t, test.expected, result)
		})
	}
}
