package uke

import (
	"context"
	"fmt"
	"testing"

	"code.uber.internal/go/envfx.git"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/secrets"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/testutils"
	drogongateway "code.uber.internal/uberai/michelangelo/controllermgr/pkg/gateways/drogon"
	"code.uber.internal/uberai/michelangelo/shared/gateways/hadooptokenservice"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/rt/flipr-client-go.git/flipr/fliprmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types/typesmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/gateways/drogon/drogonmock"
	"mock/code.uber.internal/uberai/michelangelo/shared/gateways/hadooptokenservice/hadooptokenservicemock"
)

func TestSubmitJob(t *testing.T) {
	sparkJob := &v2beta1pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-spark",
			Namespace: "ma-dev-test",
		},
		Spec: v2beta1pb.SparkJobSpec{
			SparkConf: map[string]string{
				"spark.hadoop.fs.defaultFS": "hdfs://dca-neon-1:8020",
			},
			User: &v2beta1pb.UserInfo{
				Name: "test-user",
			},
			MainApplicationFile: "hdfs://dca-neon-1:8020/user/test-user/test-app.jar",
			MainArgs:            []string{"--input", "hdfs://dca-neon-1:8020/user/test-user/input", "--output", "hdfs://dca-neon-1:8020/user/test-user/output"},
			Deps: &v2beta1pb.Dependencies{
				Jars:  []string{"hdfs://dca-neon-1:8020/user/test-user/test-app.jar"},
				Files: []string{"hdfs://dca-neon-1:8020/user/test-user/test-app.jar"},
			},
			SparkVersion: "spark_33",
			MainClass:    "com.uber.test",
			Driver: &v2beta1pb.DriverSpec{
				Pod: &v2beta1pb.PodSpec{
					Image: "uber/spark:spark_33",
					Resource: &v2beta1pb.ResourceSpec{
						Cpu:    1,
						Memory: "1GB",
						Gpu:    0,
					},
				},
			},
			Executor: &v2beta1pb.ExecutorSpec{
				Pod: &v2beta1pb.PodSpec{
					Image: "uber/spark:spark_33",
					Resource: &v2beta1pb.ResourceSpec{
						Cpu:    1,
						Memory: "1GB",
						Gpu:    0,
					},
				},
				Instances: 1,
			},
			Scheduling: &v2beta1pb.SchedulingSpec{
				Preemptible: false,
			},
		},
		Status: v2beta1pb.SparkJobStatus{
			Assignment: &v2beta1pb.AssignmentInfo{
				ResourcePool: "/UberAI/Michelangelo/IntegrationTests",
			},
		},
	}
	cluster := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dca60-batch01",
			Namespace: "ma-system",
		},
		Spec: v2beta1pb.ClusterSpec{
			Cluster: &v2beta1pb.ClusterSpec_Peloton{
				Peloton: &v2beta1pb.PelotonSpec{
					Pool: "/UberAI/Michelangelo/IntegrationTests",
				},
			},
			Region: "DCA",
			Zone:   "DCA60",
			Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
		},
	}

	testCases := []struct {
		msg           string
		mockFunc      func(*hadooptokenservicemock.MockGateway, *drogonmock.MockGateway)
		expectedError error
	}{
		{
			msg: "Submitting Job with no error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().SubmitJob(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, req *drogongateway.SubmitJobRequest) (*drogongateway.JobResponse, error) {
						p := req.RuntimeParameters
						require.Equal(t, "cloudlake-dca", p.UberRegionRouting)

						return &drogongateway.JobResponse{
							ID: 3366998,
						}, nil
					},
				)
			},
			expectedError: nil,
		},
		{
			msg: "Submitting Job with token error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{}, fmt.Errorf("failed to generate token"))
			},
			expectedError: fmt.Errorf("unable to get delegtaion tokens from token service, failed to generate token"),
		},
		{
			msg: "Submitting Job with drogon error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().SubmitJob(gomock.Any(), gomock.Any()).Return(&drogongateway.JobResponse{}, fmt.Errorf("failed to submit job"))
			},
			expectedError: fmt.Errorf("failed to submit job"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.msg, func(t *testing.T) {
			ctx := context.Background()
			gctrl := gomock.NewController(t)
			drogonMock := drogonmock.NewMockGateway(gctrl)
			hadoopTokenServiceMock := hadooptokenservicemock.NewMockGateway(gctrl)
			mockFlipr := fliprmock.NewMockFliprClient(gctrl)
			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(gctrl)
			mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)

			sparkClient := NewSparkClient(SparkClientParams{
				DrogonGateway: drogonMock,
				Secrets: secrets.New(secrets.Params{
					TokenGateway: hadoopTokenServiceMock,
				}).Provider,
				MetricsScope: tally.NoopScope,
				MTLSHandler:  mTLSHandler,
			})
			if tc.mockFunc == nil {
				tc.mockFunc = func(*hadooptokenservicemock.MockGateway, *drogonmock.MockGateway) {}
			}
			tc.mockFunc(hadoopTokenServiceMock, drogonMock)
			err := sparkClient.SubmitJob(ctx, sparkJob, cluster)
			if tc.expectedError != nil {
				require.Equal(t, tc.expectedError.Error(), err.Error())
			}
		})
	}
}

func TestGetJobStatus(t *testing.T) {
	sparkJob := &v2beta1pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-spark",
			Namespace: "ma-dev-test",
		},
		Spec: v2beta1pb.SparkJobSpec{
			User: &v2beta1pb.UserInfo{
				Name: "test-user",
			},
		},
		Status: v2beta1pb.SparkJobStatus{
			ApplicationId: "3366998",
		},
	}
	pelotonCluster := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dca11-batch01",
			Namespace: "ma-system",
		},
		Spec: v2beta1pb.ClusterSpec{
			Cluster: &v2beta1pb.ClusterSpec_Peloton{
				Peloton: &v2beta1pb.PelotonSpec{
					Pool: "/UberAI/Michelangelo/IntegrationTests",
				},
			},
			Region: "DCA",
			Zone:   "DCA60",
			Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
		},
	}
	k8sCluster := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dca60-kubernetes-batch01",
			Namespace: "ma-system",
		},
		Spec: v2beta1pb.ClusterSpec{
			Cluster: &v2beta1pb.ClusterSpec_Peloton{
				Peloton: &v2beta1pb.PelotonSpec{
					Pool: "/UberAI/Michelangelo/IntegrationTests",
				},
			},
			Region: "DCA",
			Zone:   "DCA60",
			Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
		},
	}
	k8sClusterInvalid := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dca60-kubernetes-batch01",
			Namespace: "ma-system",
		},
		Spec: v2beta1pb.ClusterSpec{
			Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
				Kubernetes: &v2beta1pb.KubernetesSpec{},
			},
			Region: "DCA",
			Zone:   "DCA60",
			Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
		},
	}

	testCases := []struct {
		msg            string
		mockFunc       func(*hadooptokenservicemock.MockGateway, *drogonmock.MockGateway)
		expectedError  error
		expectedResult constants.SparkJobStatus
		cluster        *v2beta1pb.Cluster
		sparkJob       *v2beta1pb.SparkJob
	}{
		{
			msg: "Get Job Status with no error - failed",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().GetJobStatus(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, req *drogongateway.JobStatusRequest) (*drogongateway.JobResponse, error) {
						p := req.RuntimeParameters
						require.Equal(t, "cloudlake-dca", p.UberRegionRouting)

						return &drogongateway.JobResponse{
							ID:    3366998,
							State: drogongateway.Dead,
						}, nil
					},
				)
			},
			expectedError:  nil,
			expectedResult: constants.JobStatusFailed,
			cluster:        pelotonCluster,
		},
		{
			msg: "Get Job Status with no error - pending",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().GetJobStatus(gomock.Any(), gomock.Any()).Return(&drogongateway.JobResponse{
					ID:    3366998,
					State: drogongateway.Starting,
				}, nil)
			},
			expectedError:  nil,
			expectedResult: constants.JobStatusPending,
			cluster:        pelotonCluster,
		},
		{
			msg: "Get Job Status with no error - running",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().GetJobStatus(gomock.Any(), gomock.Any()).Return(&drogongateway.JobResponse{
					ID:    3366998,
					State: drogongateway.Running,
				}, nil)
			},
			expectedError:  nil,
			expectedResult: constants.JobStatusRunning,
			cluster:        pelotonCluster,
		},
		{
			msg: "Get Job Status with no error - succeeded",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().GetJobStatus(gomock.Any(), gomock.Any()).Return(&drogongateway.JobResponse{
					ID:    3366998,
					State: drogongateway.Success,
				}, nil)
			},
			expectedError:  nil,
			expectedResult: constants.JobStatusSucceeded,
			cluster:        pelotonCluster,
		},
		{
			msg: "Get Job Status with no error on k8s - succeeded",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().GetJobStatus(gomock.Any(), gomock.Any()).Return(&drogongateway.JobResponse{
					ID:    3366998,
					State: drogongateway.Success,
				}, nil)
			},
			expectedError:  nil,
			expectedResult: constants.JobStatusSucceeded,
			cluster:        k8sCluster,
		},
		{
			msg: "Get Job Status with no error - default",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().GetJobStatus(gomock.Any(), gomock.Any()).Return(&drogongateway.JobResponse{
					ID:    3366998,
					State: drogongateway.NotStarted,
				}, nil)
			},
			expectedError:  nil,
			expectedResult: constants.JobStatusPending,
			cluster:        pelotonCluster,
		},
		{
			msg: "Getting Job Status with token error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{}, fmt.Errorf("failed to generate token"))
			},
			expectedError: fmt.Errorf("SparkClient.GetJobStatus: error getting hdfs delegation token, unable to get delegtaion tokens from token service, failed to generate token"),
			cluster:       pelotonCluster,
		},
		{
			msg: "Getting Job Status with drogon error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().GetJobStatus(gomock.Any(), gomock.Any()).Return(&drogongateway.JobResponse{}, fmt.Errorf("failed to get job"))
			},
			expectedError: fmt.Errorf("SparkClient.GetJobStatus: error getting job status from drogon, failed to get job"),
			cluster:       pelotonCluster,
		},
		{
			msg: "Getting Job Status with when cluster is not yet created",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
			},
			expectedError: fmt.Errorf("SparkClient.GetJobStatus: error getting drogon cluster name, k8s cluster are not implemented yet"),
			cluster:       k8sClusterInvalid,
		},
		{
			msg: "Getting Job Status with when cluster is not yet created",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
			},
			expectedError: fmt.Errorf("SparkClient.GetJobStatus: error getting drogon cluster name, k8s cluster are not implemented yet"),
			cluster:       k8sClusterInvalid,
		},
		{
			msg: "Getting Job Status with application id error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
			},
			expectedError: fmt.Errorf("SparkClient.GetJobStatus: error parsing application id, strconv.ParseInt: parsing \"\": invalid syntax"),
			cluster:       k8sClusterInvalid,
			sparkJob: &v2beta1pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-spark",
					Namespace: "ma-dev-test",
				},
				Spec: v2beta1pb.SparkJobSpec{
					User: &v2beta1pb.UserInfo{
						Name: "test-user",
					},
				},
				Status: v2beta1pb.SparkJobStatus{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.msg, func(t *testing.T) {
			ctx := context.Background()
			gctrl := gomock.NewController(t)
			drogonMock := drogonmock.NewMockGateway(gctrl)
			hadoopTokenServiceMock := hadooptokenservicemock.NewMockGateway(gctrl)
			mockFlipr := fliprmock.NewMockFliprClient(gctrl)
			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(gctrl)
			mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)

			sparkClient := NewSparkClient(SparkClientParams{
				DrogonGateway: drogonMock,
				Secrets: secrets.New(secrets.Params{
					TokenGateway: hadoopTokenServiceMock,
				}).Provider,
				MetricsScope: tally.NoopScope,
				MTLSHandler:  mTLSHandler,
			})
			if tc.mockFunc == nil {
				tc.mockFunc = func(*hadooptokenservicemock.MockGateway, *drogonmock.MockGateway) {}
			}
			tc.mockFunc(hadoopTokenServiceMock, drogonMock)
			testSparkJob := sparkJob
			if tc.sparkJob != nil {
				testSparkJob = tc.sparkJob
			}
			jobStatus, err := sparkClient.GetJobStatus(ctx, testSparkJob, tc.cluster)
			if tc.expectedError != nil {
				require.Equal(t, tc.expectedError.Error(), err.Error())
			}
			if tc.expectedResult != "" {
				require.Equal(t, tc.expectedResult, jobStatus)
			}
		})
	}
}

// TestGetJobStatusTwice tests against to get status for multiple jobs
func TestGetJobStatusTwice(t *testing.T) {
	sparkJob := &v2beta1pb.SparkJob{
		Spec: v2beta1pb.SparkJobSpec{
			User: &v2beta1pb.UserInfo{
				Name: "test-user",
			},
		},
		Status: v2beta1pb.SparkJobStatus{
			ApplicationId: "3366998",
		},
	}
	cluster := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dca11-batch01",
			Namespace: "ma-system",
		},
		Spec: v2beta1pb.ClusterSpec{
			Cluster: &v2beta1pb.ClusterSpec_Peloton{
				Peloton: &v2beta1pb.PelotonSpec{
					Pool: "/UberAI/Michelangelo/IntegrationTests",
				},
			},
			Region: "DCA",
			Dc:     v2beta1pb.DC_TYPE_ON_PREM,
		},
	}

	testCases := []struct {
		msg            string
		mockFunc       func(*hadooptokenservicemock.MockGateway, *drogonmock.MockGateway)
		expectedError  error
		expectedResult constants.SparkJobStatus
	}{
		{
			msg: "Get Job Status with no error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().GetJobStatus(gomock.Any(), gomock.Any()).Return(&drogongateway.JobResponse{
					ID:    3366998,
					State: drogongateway.Dead,
				}, nil).Times(2)
			},
			expectedError:  nil,
			expectedResult: constants.JobStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.msg, func(t *testing.T) {
			ctx := context.Background()
			gctrl := gomock.NewController(t)
			drogonMock := drogonmock.NewMockGateway(gctrl)
			hadoopTokenServiceMock := hadooptokenservicemock.NewMockGateway(gctrl)
			mockFlipr := fliprmock.NewMockFliprClient(gctrl)
			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(gctrl)
			mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)
			sparkClient := NewSparkClient(SparkClientParams{
				DrogonGateway: drogonMock,
				Secrets: secrets.New(secrets.Params{
					TokenGateway: hadoopTokenServiceMock,
				}).Provider,
				MetricsScope: tally.NoopScope,
				MTLSHandler:  mTLSHandler,
			})
			if tc.mockFunc == nil {
				tc.mockFunc = func(*hadooptokenservicemock.MockGateway, *drogonmock.MockGateway) {}
			}
			tc.mockFunc(hadoopTokenServiceMock, drogonMock)
			jobStatus, err := sparkClient.GetJobStatus(ctx, sparkJob, cluster)
			jobStatus, err = sparkClient.GetJobStatus(ctx, sparkJob, cluster)
			if tc.expectedError != nil {
				require.Equal(t, tc.expectedError.Error(), err.Error())
			}
			if tc.expectedResult != "" {
				require.Equal(t, tc.expectedResult, jobStatus)
			}
		})
	}

}

func TestCancelJob(t *testing.T) {
	sparkJob := &v2beta1pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-spark",
			Namespace: "ma-dev-test",
		},
		Spec: v2beta1pb.SparkJobSpec{
			User: &v2beta1pb.UserInfo{
				Name: "test-user",
			},
		},
		Status: v2beta1pb.SparkJobStatus{
			ApplicationId: "3366998",
		},
	}
	cluster := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dca60-batch01",
			Namespace: "ma-system",
		},
		Spec: v2beta1pb.ClusterSpec{
			Cluster: &v2beta1pb.ClusterSpec_Peloton{
				Peloton: &v2beta1pb.PelotonSpec{
					Pool: "/UberAI/Michelangelo/IntegrationTests",
				},
			},
			Region: "DCA",
			Zone:   "DCA60",
			Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
		},
	}

	testCases := []struct {
		msg           string
		mockFunc      func(*hadooptokenservicemock.MockGateway, *drogonmock.MockGateway)
		expectedError error
	}{
		{
			msg: "Cancel Job with no error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().CancelJob(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, req *drogongateway.CancelJobRequest) (*drogongateway.JobResponse, error) {
						p := req.RuntimeParameters
						require.Equal(t, "cloudlake-dca", p.UberRegionRouting)

						return &drogongateway.JobResponse{
							ID:    3366998,
							State: drogongateway.Dead,
						}, nil
					},
				)
			},
			expectedError: nil,
		},
		{
			msg: "Getting Job Status with token error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{}, fmt.Errorf("failed to generate token"))
			},
			expectedError: fmt.Errorf("unable to get delegtaion tokens from token service, failed to generate token"),
		},
		{
			msg: "Getting Job Status with drogon error",
			mockFunc: func(hadoopTokenServiceMock *hadooptokenservicemock.MockGateway, drogonMock *drogonmock.MockGateway) {
				hadoopTokenServiceMock.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS_DELEGATION_TOKEN",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEO",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonMock.EXPECT().CancelJob(gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to cancel job"))
			},
			expectedError: fmt.Errorf("failed to cancel job"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.msg, func(t *testing.T) {
			ctx := context.Background()
			gctrl := gomock.NewController(t)
			drogonMock := drogonmock.NewMockGateway(gctrl)
			hadoopTokenServiceMock := hadooptokenservicemock.NewMockGateway(gctrl)
			mockFlipr := fliprmock.NewMockFliprClient(gctrl)
			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(gctrl)
			mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)
			sparkClient := NewSparkClient(SparkClientParams{
				DrogonGateway: drogonMock,
				Secrets: secrets.New(secrets.Params{
					TokenGateway: hadoopTokenServiceMock,
				}).Provider,
				MetricsScope: tally.NoopScope,
				MTLSHandler:  mTLSHandler,
			})
			if tc.mockFunc == nil {
				tc.mockFunc = func(*hadooptokenservicemock.MockGateway, *drogonmock.MockGateway) {}
			}
			tc.mockFunc(hadoopTokenServiceMock, drogonMock)
			err := sparkClient.CancelJob(ctx, sparkJob, cluster)
			if tc.expectedError != nil {
				require.Equal(t, tc.expectedError.Error(), err.Error())
			}
		})
	}
}

func TestIsKubernetesCluster(t *testing.T) {
	tt := []struct {
		name          string
		expectedValue bool
	}{
		{
			name:          "phx60-batch01",
			expectedValue: false,
		},
		{
			name:          "phx60-kubernetes-batch01",
			expectedValue: true,
		},
	}

	for _, test := range tt {
		require.Equal(t, test.expectedValue, isKubernetesCluster(test.name))
	}
}

func TestIsJobKilledByUI(t *testing.T) {
	tests := []struct {
		name      string
		errString string
		want      bool
	}{
		{
			name:      "error contains both required strings",
			errString: "failed to get job status: bad status code: 500 - object not found",
			want:      true,
		},
		{
			name:      "error contains only bad status code",
			errString: "failed to get job status: bad status code: 500",
			want:      false,
		},
		{
			name:      "error contains only object not found",
			errString: "failed to get job status: object not found",
			want:      false,
		},
		{
			name:      "error contains neither required string",
			errString: "failed to get job status: internal error",
			want:      false,
		},
		{
			name:      "empty error string",
			errString: "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsJobKilledByUI(tt.errString)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGetDrogonSubmitRequestFromSparkJob(t *testing.T) {
	cluster := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dca60-batch01",
			Namespace: "ma-system",
		},
		Spec: v2beta1pb.ClusterSpec{
			Cluster: &v2beta1pb.ClusterSpec_Peloton{
				Peloton: &v2beta1pb.PelotonSpec{
					Pool: "/UberAI/Michelangelo/IntegrationTests",
				},
			},
			Region: "DCA",
			Zone:   "DCA60",
			Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
		},
	}

	testCases := []struct {
		msg                string
		namespace          string
		enableMTLS         bool
		mTLSError          error
		expectedMTLSConfig string
		expectedError      error
	}{
		{
			msg:                "mTLS enabled",
			namespace:          "test-namespace",
			enableMTLS:         true,
			mTLSError:          nil,
			expectedMTLSConfig: "true",
			expectedError:      nil,
		},
		{
			msg:                "mTLS disabled",
			namespace:          "test-namespace",
			enableMTLS:         false,
			mTLSError:          nil,
			expectedMTLSConfig: "",
			expectedError:      nil,
		},
	}

	for _, tc := range testCases {
		sparkJob := &v2beta1pb.SparkJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-namespace",
				Namespace: "test-namespace",
			},
			Spec: v2beta1pb.SparkJobSpec{
				SparkConf: map[string]string{
					"spark.hadoop.fs.defaultFS": "hdfs://dca-neon-1:8020",
				},
				User: &v2beta1pb.UserInfo{
					Name: "test-user",
				},
				MainApplicationFile: "hdfs://dca-neon-1:8020/user/test-user/test-app.jar",
				MainArgs:            []string{"--input", "hdfs://dca-neon-1:8020/user/test-user/input", "--output", "hdfs://dca-neon-1:8020/user/test-user/output"},
				Deps: &v2beta1pb.Dependencies{
					Jars:  []string{"hdfs://dca-neon-1:8020/user/test-user/test-app.jar"},
					Files: []string{"hdfs://dca-neon-1:8020/user/test-user/test-app.jar"},
				},
				SparkVersion: "spark_33",
				MainClass:    "com.uber.test",
				Driver: &v2beta1pb.DriverSpec{
					Pod: &v2beta1pb.PodSpec{
						Image: "uber/spark:spark_33",
						Resource: &v2beta1pb.ResourceSpec{
							Cpu:    1,
							Memory: "1GB",
							Gpu:    0,
						},
					},
				},
				Executor: &v2beta1pb.ExecutorSpec{
					Pod: &v2beta1pb.PodSpec{
						Image: "uber/spark:spark_33",
						Resource: &v2beta1pb.ResourceSpec{
							Cpu:    1,
							Memory: "1GB",
							Gpu:    0,
						},
					},
					Instances: 1,
				},
				Scheduling: &v2beta1pb.SchedulingSpec{
					Preemptible: false,
				},
			},
			Status: v2beta1pb.SparkJobStatus{
				Assignment: &v2beta1pb.AssignmentInfo{
					ResourcePool: "/UberAI/Michelangelo/IntegrationTests",
				},
			},
		}
		t.Run(tc.msg, func(t *testing.T) {
			gctrl := gomock.NewController(t)
			drogonMock := drogonmock.NewMockGateway(gctrl)
			hadoopTokenServiceMock := hadooptokenservicemock.NewMockGateway(gctrl)
			mockFlipr := fliprmock.NewMockFliprClient(gctrl)
			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(gctrl)
			mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, tc.enableMTLS, false, nil, nil)

			shouldEnable, err := mTLSHandler.EnableMTLS(tc.namespace)
			require.NoError(t, err)
			require.Equal(t, tc.enableMTLS, shouldEnable)

			sparkClient := NewSparkClient(SparkClientParams{
				DrogonGateway: drogonMock,
				Secrets: secrets.New(secrets.Params{
					TokenGateway: hadoopTokenServiceMock,
				}).Provider,
				MetricsScope: tally.NoopScope,
				MTLSHandler:  mTLSHandler,
				Env: envfx.Context{
					RuntimeEnvironment: envfx.EnvTest,
				},
			})

			req, err := sparkClient.getDrogonSubmitRequestFromSparkJob(sparkJob, cluster)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedMTLSConfig, req.JobDefinition.Conf["spark.drogon.k8s.mtls.enable"])
			require.Equal(t, "test-user", req.JobDefinition.ProxyUser)
			require.Equal(t, "uber/spark:spark_33", req.JobDefinition.Conf["spark.peloton.driver.docker.image"])
			require.Equal(t, "uber/spark:spark_33", req.JobDefinition.Conf["spark.peloton.executor.docker.image"])
			require.Equal(t, "uber/spark:spark_33", req.JobDefinition.Conf["spark.mesos.executor.docker.image"])
			require.Equal(t, "true", req.JobDefinition.Conf["spark.peloton.run-as-user"])
			require.Equal(t, "006", req.JobDefinition.Conf["spark.hadoop.fs.permissions.umask-mode"])
			require.Equal(t, "DCA", req.JobDefinition.Conf["spark.peloton.driverEnv.REGION"])
			require.Equal(t, "false", req.JobDefinition.Conf["spark.peloton.sla.preemptible"])
			require.Equal(t, "cloudlake-dca", req.RuntimeParameters.UberRegionRouting)
			require.Equal(t, "michelangelo-spark", req.JobDefinition.Conf["spark.drogon.k8s.label.ma/owner-service"])
			require.Equal(t, "test-namespace", req.JobDefinition.Conf["spark.drogon.k8s.label.ma/project-name"])
			require.Equal(t, "test", req.JobDefinition.Conf["spark.drogon.k8s.label.ma/control-plane-env"])
		})
	}
}
