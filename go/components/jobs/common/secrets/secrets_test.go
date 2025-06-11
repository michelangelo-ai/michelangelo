package secrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"code.uber.internal/uberai/michelangelo/shared/gateways/hadooptokenservice"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1 "michelangelo/api/v2beta1"
	"mock/code.uber.internal/uberai/michelangelo/shared/gateways/hadooptokenservice/hadooptokenservicemock"
)

func TestGenerateHadoopSecret(t *testing.T) {
	testCreds := "creds"
	encodedCred := base64.RawURLEncoding.EncodeToString([]byte(testCreds))

	tests := []struct {
		msg            string
		cluster        *v2beta1.Cluster
		job            runtime.Object
		setupProvider  func(t *testing.T, ctrl *gomock.Controller) Provider
		assertDataFunc func(data map[string][]byte)
		wantError      error
	}{
		{
			msg: "batch ray job in PHX",
			cluster: &v2beta1.Cluster{
				Spec: v2beta1.ClusterSpec{
					Region: "phx",
					Zone:   "phx2",
				},
			},
			job: &v2beta1.RayJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.RayJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			setupProvider: func(t *testing.T, ctrl *gomock.Controller) Provider {
				mockTokenService := hadooptokenservicemock.NewMockGateway(ctrl)
				provider := New(Params{
					TokenGateway: mockTokenService,
				}).Provider

				mockTokenService.
					EXPECT().
					GenerateTokens(
						gomock.Any(),
						gomock.Any()).DoAndReturn(func(_ context.Context, req *hadooptokenservice.GenerateTokensRequest) (*hadooptokenservice.GenerateTokensResponse, error) {
					params := req.Params
					require.Equal(t, 7, len(params))

					return &hadooptokenservice.GenerateTokensResponse{
						Credentials: encodedCred,
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "test-service-type",
								ServiceRegion: "test-service-region",
								ServiceAlias:  "test-service-alias",
								Token:         "test-token",
							},
						},
					}, nil
				})
				return provider
			},
			assertDataFunc: func(secretData map[string][]byte) {
				expectedData := map[string]any{"test-job": map[string]any{
					"token": encodedCred,
					"hdfs": map[string]any{
						"cluster":    "phx2",
						"nameserver": "ns-platinum-prod-phx",
						"schemes":    "hdfs",
					},
					"tokens": []any{
						map[string]any{
							"serviceType":   "test-service-type",
							"serviceRegion": "test-service-region",
							"serviceAlias":  "test-service-alias",
							"token":         "test-token",
						},
					},
				}}
				require.Equal(t, len(expectedData), len(secretData))

				for k, v := range expectedData {
					val, ok := secretData[k]
					require.True(t, ok)
					var actual any
					require.NoError(t, json.Unmarshal(val, &actual))
					require.Equal(t, v, actual)
				}
			},
		},
		{
			msg: "batch ray job in DCA11",
			cluster: &v2beta1.Cluster{
				Spec: v2beta1.ClusterSpec{
					Zone:   "dca11",
					Region: "dca",
				},
			},
			job: &v2beta1.RayJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.RayJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			setupProvider: func(t *testing.T, ctrl *gomock.Controller) Provider {
				mockTokenService := hadooptokenservicemock.NewMockGateway(ctrl)
				provider := New(Params{
					TokenGateway: mockTokenService,
				}).Provider

				mockTokenService.
					EXPECT().
					GenerateTokens(
						gomock.Any(),
						gomock.Any()).DoAndReturn(func(_ context.Context, req *hadooptokenservice.GenerateTokensRequest) (*hadooptokenservice.GenerateTokensResponse, error) {
					params := req.Params
					require.Equal(t, 7, len(params))

					return &hadooptokenservice.GenerateTokensResponse{
						Credentials: encodedCred,
					}, nil
				})
				return provider
			},
			assertDataFunc: func(secretData map[string][]byte) {
				expectedData := map[string]any{"test-job": map[string]any{
					"token": encodedCred,
					"hdfs": map[string]any{
						"cluster":    "dca11",
						"nameserver": "ns-neon-prod-dca1",
						"schemes":    "hdfs",
					},
					"tokens": nil,
				}}
				require.Equal(t, len(expectedData), len(secretData))

				for k, v := range expectedData {
					val, ok := secretData[k]
					require.True(t, ok)
					var actual any
					require.NoError(t, json.Unmarshal(val, &actual))
					require.Equal(t, v, actual)
				}
			},
		},
		{
			msg: "batch ray job in DCA1",
			cluster: &v2beta1.Cluster{
				Spec: v2beta1.ClusterSpec{
					Region: "dca",
					Zone:   "dca1",
				},
			},
			job: &v2beta1.RayJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.RayJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			setupProvider: func(t *testing.T, ctrl *gomock.Controller) Provider {
				mockTokenService := hadooptokenservicemock.NewMockGateway(ctrl)
				provider := New(Params{
					TokenGateway: mockTokenService,
				}).Provider

				mockTokenService.
					EXPECT().
					GenerateTokens(
						gomock.Any(),
						gomock.Any()).DoAndReturn(func(_ context.Context, req *hadooptokenservice.GenerateTokensRequest) (*hadooptokenservice.GenerateTokensResponse, error) {
					params := req.Params
					require.Equal(t, 7, len(params))

					return &hadooptokenservice.GenerateTokensResponse{
						Credentials: encodedCred,
					}, nil
				})
				return provider
			},
			assertDataFunc: func(secretData map[string][]byte) {
				expectedData := map[string]any{"test-job": map[string]any{
					"token": encodedCred,
					"hdfs": map[string]any{
						"cluster":    "dca1",
						"nameserver": "ns-neon-prod-dca1",
						"schemes":    "hdfs",
					},
					"tokens": nil,
				}}
				require.Equal(t, len(expectedData), len(secretData))

				for k, v := range expectedData {
					val, ok := secretData[k]
					require.True(t, ok)
					var actual any
					require.NoError(t, json.Unmarshal(val, &actual))
					require.Equal(t, v, actual)
				}
			},
		},
		{
			msg: "batch ray job in DCA60",
			cluster: &v2beta1.Cluster{
				Spec: v2beta1.ClusterSpec{
					Region: "dca",
					Zone:   "dca60",
					Dc:     v2beta1.DC_TYPE_CLOUD_GCP,
				},
			},
			job: &v2beta1.RayJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.RayJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			setupProvider: func(t *testing.T, ctrl *gomock.Controller) Provider {
				mockTokenService := hadooptokenservicemock.NewMockGateway(ctrl)
				provider := New(Params{
					TokenGateway: mockTokenService,
				}).Provider

				mockTokenService.
					EXPECT().
					GenerateTokens(
						gomock.Any(),
						gomock.Any()).DoAndReturn(func(_ context.Context, req *hadooptokenservice.GenerateTokensRequest) (*hadooptokenservice.GenerateTokensResponse, error) {
					params := req.Params
					require.Equal(t, 7, len(params))

					return &hadooptokenservice.GenerateTokensResponse{
						Credentials: encodedCred,
					}, nil
				})
				return provider
			},
			assertDataFunc: func(secretData map[string][]byte) {
				expectedData := map[string]any{"test-job": map[string]any{
					"token": encodedCred,
					"hdfs": map[string]any{
						"cluster":    "dca60",
						"nameserver": "ns-neon-prod-dca1",
						"schemes":    "hdfs",
					},
					"tokens": nil,
				}}
				require.Equal(t, len(expectedData), len(secretData))

				for k, v := range expectedData {
					val, ok := secretData[k]
					require.True(t, ok)
					var actual any
					require.NoError(t, json.Unmarshal(val, &actual))
					require.Equal(t, v, actual)
				}
			},
		},
		{
			msg: "batch job with malformed cluster spec",
			cluster: &v2beta1.Cluster{
				Spec: v2beta1.ClusterSpec{
					Region: "xyz", // malformed region
				},
			},
			setupProvider: func(t *testing.T, ctrl *gomock.Controller) Provider {
				mockTokenService := hadooptokenservicemock.NewMockGateway(ctrl)
				provider := New(Params{
					TokenGateway: mockTokenService,
				}).Provider
				return provider
			},
			job: &v2beta1.SparkJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.SparkJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			wantError: fmt.Errorf("failed to get service alias for region:xyz"),
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			provider := test.setupProvider(t, ctrl)
			secretData, err := provider.GenerateHadoopSecret(context.Background(), test.job, test.cluster)
			if test.wantError != nil {
				require.EqualError(t, test.wantError, err.Error())
				return
			}
			require.NoError(t, err)
			test.assertDataFunc(secretData)
		})
	}
}

func TestGetKubeSecretName(t *testing.T) {
	tests := []struct {
		name    string
		jobName string
		want    string
	}{
		{
			name:    "get kube secret name success",
			jobName: "test",
			want:    "ma-hadoop-test",
		},
	}

	for _, test := range tests {
		out := GetKubeSecretName(test.jobName)
		require.Equal(t, test.want, out)
	}
}

func TestGetJobUser(t *testing.T) {
	tt := []struct {
		job              runtime.Object
		wantError        bool
		expectedUserName string
	}{
		{
			job: &v2beta1.RayJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.RayJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			expectedUserName: "test-user",
		},
		{
			job: &v2beta1.SparkJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.SparkJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			expectedUserName: "test-user",
		},
		{
			job:       nil,
			wantError: true,
		},
	}

	for _, test := range tt {
		p := Provider{}
		user, err := p.getJobUser(test.job)
		if test.wantError {
			require.Error(t, err)
			require.Equal(t, "invalid job type", err.Error())
		} else {
			require.NoError(t, err)
			require.Equal(t, test.expectedUserName, user)
		}
	}
}

func TestGetAccessTokenForDrogon(t *testing.T) {
	testCreds := "creds"
	encodedCred := base64.RawURLEncoding.EncodeToString([]byte(testCreds))

	tests := []struct {
		msg           string
		cluster       *v2beta1.Cluster
		job           runtime.Object
		setupProvider func(t *testing.T, ctrl *gomock.Controller) Provider
		wantError     error
	}{
		{
			msg: "Spark job in PHX",
			cluster: &v2beta1.Cluster{
				Spec: v2beta1.ClusterSpec{
					Region: "phx",
				},
			},
			job: &v2beta1.SparkJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.SparkJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			setupProvider: func(t *testing.T, ctrl *gomock.Controller) Provider {
				mockTokenService := hadooptokenservicemock.NewMockGateway(ctrl)
				provider := New(Params{TokenGateway: mockTokenService}).Provider
				mockTokenService.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, req *hadooptokenservice.GenerateTokensRequest) (*hadooptokenservice.GenerateTokensResponse, error) {
						params := req.Params
						require.Equal(t, 7, len(params))
						return &hadooptokenservice.GenerateTokensResponse{
							Credentials: encodedCred,
							Tokens: []hadooptokenservice.Token{
								{
									ServiceType:   _hdfsService,
									ServiceRegion: "phx",
									ServiceAlias:  _routerServiceAlias,
									Token:         "test-token",
								},
							},
						}, nil
					})
				return provider
			},
		},
		{
			msg: "Generate token error",
			cluster: &v2beta1.Cluster{
				Spec: v2beta1.ClusterSpec{
					Region: "phx",
				},
			},
			job: &v2beta1.SparkJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.SparkJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			setupProvider: func(t *testing.T, ctrl *gomock.Controller) Provider {
				mockTokenService := hadooptokenservicemock.NewMockGateway(ctrl)
				provider := New(Params{
					TokenGateway: mockTokenService,
				}).Provider

				mockTokenService.
					EXPECT().
					GenerateTokens(
						gomock.Any(),
						gomock.Any()).DoAndReturn(func(_ context.Context, req *hadooptokenservice.GenerateTokensRequest) (*hadooptokenservice.GenerateTokensResponse, error) {
					return nil, fmt.Errorf("error")
				})
				return provider
			},
			wantError: fmt.Errorf("unable to get delegtaion tokens from token service, error"),
		},
		{
			msg: "Generate token partial token error",
			cluster: &v2beta1.Cluster{
				Spec: v2beta1.ClusterSpec{
					Region: "phx",
				},
			},
			job: &v2beta1.SparkJob{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: v2beta1.SparkJobSpec{
					User: &v2beta1.UserInfo{
						Name: "test-user",
					},
				},
			},
			setupProvider: func(t *testing.T, ctrl *gomock.Controller) Provider {
				mockTokenService := hadooptokenservicemock.NewMockGateway(ctrl)
				provider := New(Params{
					TokenGateway: mockTokenService,
				}).Provider

				mockTokenService.
					EXPECT().
					GenerateTokens(
						gomock.Any(),
						gomock.Any()).DoAndReturn(func(_ context.Context, req *hadooptokenservice.GenerateTokensRequest) (*hadooptokenservice.GenerateTokensResponse, error) {
					return nil, hadooptokenservice.ErrPartialTokens
				})
				return provider
			},
			wantError: hadooptokenservice.ErrPartialTokens,
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			provider := test.setupProvider(t, ctrl)
			generatedToken, err := provider.GetAccessTokenForDrogon(context.Background(), test.job, test.cluster)
			if test.wantError != nil {
				require.EqualError(t, test.wantError, err.Error())
				return
			}
			require.NoError(t, err)
			require.NotEqual(t, hadooptokenservice.Token{}, generatedToken)
		})
	}
}

func TestGetSecreteData(t *testing.T) {

	t.Run("invalid job type", func(t *testing.T) {

		resp := &hadooptokenservice.GenerateTokensResponse{} // response stub
		job := &v2beta1.Pipeline{}                           // unsupported job type (not even a job)
		cluster := &v2beta1.Cluster{}                        // cluster stub

		provider := Provider{}
		data, err := provider.getSecreteData(resp, job, cluster)
		require.Nil(t, data)
		require.Error(t, err)
		require.Equal(t, "invalid job type", err.Error())
	})
}
