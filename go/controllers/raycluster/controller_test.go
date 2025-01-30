package raycluster

import (
	"context"
	"testing"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/controllers/utils/testutils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	v1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	rayv1scheme "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/scheme"
	rayv1fake "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/client-go/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	rayClusterName = "test-cluster"
	testNamespace  = "default"
)

func TestReconciler_Reconcile(t *testing.T) {
	ctx := context.Background()

	// Mock environment
	scheme := runtime.NewScheme()
	rayv1scheme.AddToScheme(scheme)
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	// Test cases
	tests := []struct {
		name            string
		setup           func() []client.Object
		expectedState   v2pb.RayClusterState
		expectedMessage []*v2pb.PodErrors
		errorAssertion  require.ErrorAssertionFunc
		postCheck       func(res ctrl.Result)
		rayIOSetup      *v1.RayCluster
	}{
		{
			name: "No ray cluster",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				return objects
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_INVALID,
			expectedMessage: nil,
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, res.RequeueAfter, time.Duration(0))
			},
		},
		{
			name: "No ray cluster but with ray io cluster",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				return objects
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_INVALID,
			expectedMessage: nil,
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, res.RequeueAfter, time.Duration(0))
			},
			rayIOSetup: &v1.RayCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rayClusterName,
					Namespace: testNamespace,
				},
				Spec: v1.RayClusterSpec{
					EnableInTreeAutoscaling: nil,
					HeadGroupSpec: v1.HeadGroupSpec{
						ServiceType:    corev1.ServiceType("clusterIP"),
						RayStartParams: map[string]string{},
					},
				},
			},
		},
		{
			name: "cluster is provisioning",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rayClusterName,
						Namespace: testNamespace,
					},
					Spec: v2pb.RayClusterSpec{
						RayVersion: "2.3.1",
						Head: &v2pb.RayHeadSpec{
							ServiceType: "clusterIP",
							Pod: &corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:  "test",
											Image: "test",
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("1"),
													corev1.ResourceMemory: resource.MustParse("1Gi"),
												},
											},
										},
									},
								},
							},
						},
						Workers: []*v2pb.RayWorkerSpec{
							{
								Pod: &corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Containers: []corev1.Container{
											{
												Name:  "test",
												Image: "test",
												Resources: corev1.ResourceRequirements{
													Requests: corev1.ResourceList{
														corev1.ResourceCPU:    resource.MustParse("1"),
														corev1.ResourceMemory: resource.MustParse("1Gi"),
													},
												},
											},
										},
									},
								},
								MinInstances: 1,
								MaxInstances: 1,
							},
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_PROVISIONING,
			expectedMessage: nil,
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
		},
		{
			name: "cluster is ready",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rayClusterName,
						Namespace: testNamespace,
					},
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_READY,
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_READY,
			expectedMessage: []*v2pb.PodErrors(nil),
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			rayIOSetup: &v1.RayCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rayClusterName,
					Namespace: testNamespace,
				},
				Spec: v1.RayClusterSpec{
					EnableInTreeAutoscaling: nil,
					HeadGroupSpec: v1.HeadGroupSpec{
						ServiceType:    corev1.ServiceType("clusterIP"),
						RayStartParams: map[string]string{},
					},
				},
				Status: v1.RayClusterStatus{
					State: v1.Ready,
				},
			},
		},
		{
			name: "cluster is terminating",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rayClusterName,
						Namespace: testNamespace,
					},
					Spec: v2pb.RayClusterSpec{
						Termination: &v2pb.TerminationSpec{
							Type:   v2pb.TERMINATION_TYPE_SUCCEEDED,
							Reason: "job completed successfully",
						},
					},
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_READY,
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_TERMINATING,
			expectedMessage: []*v2pb.PodErrors(nil),
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			rayIOSetup: &v1.RayCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rayClusterName,
					Namespace: testNamespace,
				},
				Spec: v1.RayClusterSpec{
					EnableInTreeAutoscaling: nil,
					HeadGroupSpec: v1.HeadGroupSpec{
						ServiceType:    corev1.ServiceType("clusterIP"),
						RayStartParams: map[string]string{},
					},
				},
				Status: v1.RayClusterStatus{
					State: v1.Ready,
				},
			},
		},
		{
			name: "cluster is terminated",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rayClusterName,
						Namespace: testNamespace,
					},
					Spec: v2pb.RayClusterSpec{
						Termination: &v2pb.TerminationSpec{
							Type:   v2pb.TERMINATION_TYPE_SUCCEEDED,
							Reason: "job completed successfully",
						},
					},
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_READY,
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_TERMINATED,
			expectedMessage: []*v2pb.PodErrors(nil),
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			rayIOSetup: nil,
		},
		{
			name: "cluster is failed",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rayClusterName,
						Namespace: testNamespace,
					},
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_READY,
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			expectedState: v2pb.RAY_CLUSTER_STATE_FAILED,
			expectedMessage: []*v2pb.PodErrors{
				{
					Name:          "",
					ContainerName: rayClusterName,
					ExitCode:      0,
					Reason:        "cluster failed",
					Message:       "",
				},
			},
			errorAssertion: require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			rayIOSetup: &v1.RayCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rayClusterName,
					Namespace: testNamespace,
				},
				Spec: v1.RayClusterSpec{
					EnableInTreeAutoscaling: nil,
					HeadGroupSpec: v1.HeadGroupSpec{
						ServiceType:    corev1.ServiceType("clusterIP"),
						RayStartParams: map[string]string{},
					},
				},
				Status: v1.RayClusterStatus{
					State:  v1.Failed,
					Reason: "cluster failed",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			objects := tc.setup()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			requestRayCluster := types.NamespacedName{
				Name:      rayClusterName,
				Namespace: testNamespace,
			}
			fakeClientWrapper := testutils.NewFakeClientWrapper(fakeClient)
			// Set up a fake RayV1 client.
			fakeRayV1Client := &rayv1fake.FakeRayV1{
				Fake: &k8stesting.Fake{},
			}
			reactorManager := &testutils.ReactorManager{}

			// Add reusable reactors for "create" and "get"
			fakeRayV1Client.Fake.AddReactor("create", "rayclusters", reactorManager.CreateReactor())
			fakeRayV1Client.Fake.AddReactor("get", "rayclusters", reactorManager.GetReactor())

			r := &Reconciler{
				Client:         fakeClientWrapper,
				RayV1Interface: fakeRayV1Client,
			}

			if tc.rayIOSetup != nil {
				_, _ = r.RayClusters(testNamespace).Create(context.Background(), tc.rayIOSetup, metav1.CreateOptions{})
			}
			res, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: requestRayCluster,
			})
			tc.errorAssertion(t, err)
			tc.postCheck(res)

			// Assert
			var updatedRayCluster v2pb.RayCluster
			_ = r.Get(ctx, requestRayCluster, &updatedRayCluster)

			assert.Equal(t, tc.expectedState, updatedRayCluster.Status.State)
			assert.Equal(t, tc.expectedMessage, updatedRayCluster.Status.PodErrors)
		})
	}
}
