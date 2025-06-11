package framework

import (
	"testing"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	sharedConstants "code.uber.internal/uberai/michelangelo/shared/constants"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1pb "michelangelo/api/v2beta1"
)

func TestGetResourceRequirement(t *testing.T) {
	tt := []struct {
		name      string
		job       BatchJob
		want      v1.ResourceList
		wantError string
	}{
		{
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{},
							},
						},
						Scheduling: &v2beta1pb.SchedulingSpec{Preemptible: true},
					},
				},
			},
			name:      "empty ray job spec",
			want:      v1.ResourceList{"cpu": resource.Quantity{Format: "DecimalSI"}},
			wantError: "",
		},
		{
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Gpu:    1,
									Memory: "100Mi",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Gpu:    1,
									Memory: "100Mi",
								},
							},
							MinInstances: 2,
							MaxInstances: 2,
						},
						Scheduling: &v2beta1pb.SchedulingSpec{Preemptible: true},
					},
				},
			},
			name: "valid ray job spec",
			want: v1.ResourceList{
				"cpu":            *resource.NewQuantity(12, resource.DecimalSI),
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
				"memory":         *resource.NewScaledQuantity(300, 6),
			},
			wantError: "",
		},
		{
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Memory: "100Mi",
								},
							},
						},
						Workers: []*v2beta1pb.WorkerSpec{
							{
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    4,
										Memory: "100Mi",
									},
								},
								MinInstances: 4,
								MaxInstances: 4,
								NodeType:     "DATA_NODE",
							},
							{
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    8,
										Gpu:    1,
										Memory: "150Mi",
									},
								},
								MinInstances: 1,
								MaxInstances: 1,
								NodeType:     "TRAINER_NODE",
							},
						},
						Scheduling: &v2beta1pb.SchedulingSpec{Preemptible: true},
					},
				},
			},
			name: "valid heterogeneous ray job spec",
			want: v1.ResourceList{
				"cpu":            *resource.NewQuantity(28, resource.DecimalSI),
				"nvidia.com/gpu": *resource.NewQuantity(1, resource.DecimalSI),
				"memory":         *resource.NewScaledQuantity(650, 6),
			},
			wantError: "",
		},
		{
			job: BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					Spec: v2beta1pb.SparkJobSpec{
						Scheduling: &v2beta1pb.SchedulingSpec{Preemptible: true},
					},
				},
			},
			name:      "empty spark job spec",
			want:      v1.ResourceList{"cpu": resource.Quantity{Format: "DecimalSI"}},
			wantError: "",
		},
	}

	for _, test := range tt {
		res, err := test.job.GetResourceRequirement()
		if test.wantError != "" {
			require.NotNil(t, err)
			require.Equal(t, test.wantError, err)
		} else {
			require.Nil(t, err)
			require.Equal(t, test.want.Cpu(), res.Cpu())
		}
	}
}

func TestGetters(t *testing.T) {
	tt := []struct {
		desc                      string
		job                       BatchJob
		wantAffinity              *v2beta1pb.Affinity
		wantAssignment            *v2beta1pb.AssignmentInfo
		wantConditions            *[]*v2beta1pb.Condition
		wantGeneration            int64
		wantName                  string
		wantNamespace             string
		wantUser                  string
		wantSchedulingPreemptible bool
		wantJobEnv                string
		wantLabels                map[string]string
		wantAnnotations           map[string]string
		wantTerminationType       v2beta1pb.TerminationType
	}{
		{
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ClusterAffinity: &v2beta1pb.ClusterAffinity{},
						},
						User: &v2beta1pb.UserInfo{
							Name: "dummyUser",
						},
						Scheduling: &v2beta1pb.SchedulingSpec{
							Preemptible: true,
						},
						Termination: &v2beta1pb.TerminationSpec{
							Type: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
						},
					},
					Status: v2beta1pb.RayJobStatus{StatusConditions: []*v2beta1pb.Condition{{}}},
					ObjectMeta: v1meta.ObjectMeta{
						Generation: 1,
						Namespace:  "dummyNamespace",
						Name:       "dummyName",
						Labels: map[string]string{
							sharedConstants.EnvironmentLabel: constants.Production,
						},
						Annotations: map[string]string{
							"runnable": "test_runnable",
						},
					},
				},
			},
			desc:                      "valid ray job spec",
			wantAffinity:              &v2beta1pb.Affinity{ClusterAffinity: &v2beta1pb.ClusterAffinity{}},
			wantAssignment:            &v2beta1pb.AssignmentInfo{},
			wantConditions:            &[]*v2beta1pb.Condition{{}},
			wantGeneration:            1,
			wantName:                  "dummyName",
			wantNamespace:             "dummyNamespace",
			wantUser:                  "dummyUser",
			wantSchedulingPreemptible: true,
			wantJobEnv:                v2beta1pb.ENV_TYPE_PRODUCTION.String(),
			wantLabels: map[string]string{
				sharedConstants.EnvironmentLabel: constants.Production,
			},
			wantAnnotations: map[string]string{
				"runnable": "test_runnable",
			},
			wantTerminationType: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
		},
		{
			job: BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					Spec: v2beta1pb.SparkJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ClusterAffinity: &v2beta1pb.ClusterAffinity{},
						},
						User: &v2beta1pb.UserInfo{
							Name: "dummyUser",
						},
						Scheduling: &v2beta1pb.SchedulingSpec{
							Preemptible: false,
						},
						Termination: &v2beta1pb.TerminationSpec{
							Type: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
						},
					},
					Status: v2beta1pb.SparkJobStatus{StatusConditions: []*v2beta1pb.Condition{}},
					ObjectMeta: v1meta.ObjectMeta{
						Generation: 1,
						Namespace:  "dummyNamespace",
						Name:       "dummyName",
						Labels: map[string]string{
							sharedConstants.EnvironmentLabel: v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
						},
						Annotations: map[string]string{
							"runnable": "test_runnable",
						},
					},
				},
			},
			desc:                      "valid spark job spec",
			wantAffinity:              &v2beta1pb.Affinity{ClusterAffinity: &v2beta1pb.ClusterAffinity{}},
			wantAssignment:            &v2beta1pb.AssignmentInfo{},
			wantConditions:            &[]*v2beta1pb.Condition{},
			wantGeneration:            1,
			wantName:                  "dummyName",
			wantNamespace:             "dummyNamespace",
			wantUser:                  "dummyUser",
			wantSchedulingPreemptible: false,
			wantJobEnv:                v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
			wantLabels: map[string]string{
				sharedConstants.EnvironmentLabel: v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
			},
			wantAnnotations: map[string]string{
				"runnable": "test_runnable",
			},
			wantTerminationType: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
		},
	}

	for _, test := range tt {
		affinity := test.job.GetAffinity()
		require.Equal(t, test.wantAffinity, affinity)

		assignment := test.job.GetAssignmentInfo()
		require.Equal(t, test.wantAssignment, assignment)

		conditions := test.job.GetConditions()
		require.Equal(t, test.wantConditions, conditions)

		generation := test.job.GetGeneration()
		require.Equal(t, test.wantGeneration, generation)

		name := test.job.GetName()
		require.Equal(t, test.wantName, name)

		namespace := test.job.GetNamespace()
		require.Equal(t, test.wantNamespace, namespace)

		user := test.job.GetUserName()
		require.Equal(t, test.wantUser, user)

		isPreemptibleJob := test.job.IsPreemptibleJob()
		require.Equal(t, test.wantSchedulingPreemptible, isPreemptibleJob)

		env := test.job.GetEnvironmentLabel()
		require.Equal(t, test.wantJobEnv, env)

		require.Equal(t, test.wantLabels, test.job.GetLabels())

		require.Equal(t, test.wantAnnotations, test.job.GetAnnotations())

		require.Equal(t, test.wantTerminationType, test.job.GetTerminationSpec().Type)
	}
}
