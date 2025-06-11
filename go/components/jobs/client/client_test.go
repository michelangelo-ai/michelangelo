package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"code.uber.internal/base/testing/contextmatcher"
	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta1"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client/uke"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/secrets"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/testutils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/compute"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/gateways/drogon"
	"code.uber.internal/uberai/michelangelo/shared/gateways/hadooptokenservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/rest/fake"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/rt/flipr-client-go.git/flipr/fliprmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types/typesmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/compute/computemock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/gateways/drogon/drogonmock"
	"mock/code.uber.internal/uberai/michelangelo/shared/gateways/hadooptokenservice/hadooptokenservicemock"
)

func TestGetClusterStatus(t *testing.T) {
	tests := []struct {
		msg                     string
		healthOk                bool
		sendErr                 error
		statusCode              int
		expectedConditionsTrue  []string
		expectedConditionsFalse []string
		expectErr               func(err error) bool
		getClientSetError       error
	}{
		{
			msg:                    "dummy error",
			healthOk:               true,
			sendErr:                nil,
			statusCode:             200,
			expectedConditionsTrue: []string{constants.ClusterReady},
			getClientSetError:      errors.New("dummy error"),
			expectErr:              func(err error) bool { return strings.Contains(err.Error(), "dummy error") },
		},
		{
			msg:                    "cluster online and healthy",
			healthOk:               true,
			sendErr:                nil,
			statusCode:             200,
			expectedConditionsTrue: []string{constants.ClusterReady},
		},
		{
			msg:                     "cluster online and unhealthy",
			healthOk:                false,
			sendErr:                 nil,
			statusCode:              200,
			expectedConditionsFalse: []string{constants.ClusterReady, constants.ClusterOffline},
		},
		{
			msg:                    "cluster offline",
			sendErr:                errors.New("connection refused"),
			expectedConditionsTrue: []string{constants.ClusterOffline},
			expectErr:              func(err error) bool { return strings.Contains(err.Error(), "connection refused") },
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
				if test.sendErr != nil {
					return nil, test.sendErr
				}
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				var resp string
				if test.healthOk {
					resp = "ok"
				}
				return &http.Response{StatusCode: test.statusCode, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
			})

			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			c.RESTClient().(*restclient.RESTClient).Client = fc

			g := gomock.NewController(t)
			f := computemock.NewMockFactory(g)

			testCluster := v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				CoreV1: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
			}

			// test
			status, err := k8sc.GetClusterStatus(context.Background(), &testCluster)
			if err == nil && test.expectErr != nil {
				t.Errorf("expected error, got nil for")
			}
			if err != nil {
				if test.expectErr == nil || !test.expectErr(err) {
					t.Errorf("unexpected error for %v", err)
				}
				return
			}

			// assert conditions
			require.Equal(t, len(status.StatusConditions), len(test.expectedConditionsFalse)+len(test.expectedConditionsTrue), "unexpected conditions")
			assertConditions(t, status.StatusConditions, test.expectedConditionsTrue, v2beta1pb.CONDITION_STATUS_TRUE)
			assertConditions(t, status.StatusConditions, test.expectedConditionsFalse, v2beta1pb.CONDITION_STATUS_FALSE)
		})
	}
}

func TestGetResourcePools(t *testing.T) {
	tests := []struct {
		msg                       string
		want                      infraCrds.ResourcePoolList
		wantError                 string
		getClientSetError         error
		fakeRestClientRequestMock func(request *http.Request) (*http.Response, error)
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			want:              infraCrds.ResourcePoolList{},
			wantError:         "dummy error",
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "failure due to k8s rest client error",
			want:      infraCrds.ResourcePoolList{},
			wantError: "Get \"http://localhost/resourcepools\": dummy error",
			fakeRestClientRequestMock: func(request *http.Request) (*http.Response, error) {
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				return nil, errors.New("dummy error")
			},
		},
		{
			msg:  "success",
			want: infraCrds.ResourcePoolList{},
			fakeRestClientRequestMock: func(request *http.Request) (*http.Response, error) {
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(`{"Kind":"test", "APIVersion":"test"}`)))}, nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			fc := fake.CreateHTTPClient(test.fakeRestClientRequestMock)

			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			c.RESTClient().(*restclient.RESTClient).Client = fc

			g := gomock.NewController(t)
			f := computemock.NewMockFactory(g)

			testCluster := v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				ComputeV1: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
				logger:  zaptest.NewLogger(t),
			}

			// test
			resourcePoolList, err := k8sc.GetResourcePools(context.Background(), &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Equal(t, test.wantError, err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, test.want, resourcePoolList)
			}
		})
	}
}

func TestDeleteJob(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			jobInput:          &v2beta1pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "resource name not empty condition for ray job",
			wantError: "resource name may not be empty",
			jobInput:  &v2beta1pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				resp := "ok"
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
			})

			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			c.RESTClient().(*restclient.RESTClient).Client = fc

			g := gomock.NewController(t)
			f := computemock.NewMockFactory(g)

			testCluster := v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				Ray:   c.RESTClient(),
				Spark: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
			}

			// test
			err := k8sc.DeleteJob(context.Background(), test.jobInput, &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Equal(t, test.wantError, err.Error())
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestDeletePromConfigMap(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			jobInput:          &v2beta1pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "unknown type used for body for ray job",
			wantError: "unknown type used for body:",
			jobInput:  &v2beta1pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				resp := "ok"
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
			})

			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			c.RESTClient().(*restclient.RESTClient).Client = fc

			g := gomock.NewController(t)
			f := computemock.NewMockFactory(g)

			testCluster := v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				CoreV1: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
			}

			// test
			err := k8sc.DeletePromConfigMap(context.Background(), test.jobInput, &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Error(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestDeleteSecret(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			jobInput:          &v2beta1pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "unknown type used for body for ray job",
			wantError: "unknown type used for body:",
			jobInput:  &v2beta1pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				resp := "ok"
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
			})

			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			c.RESTClient().(*restclient.RESTClient).Client = fc

			g := gomock.NewController(t)
			f := computemock.NewMockFactory(g)

			testCluster := v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				CoreV1: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
			}

			// test
			err := k8sc.DeleteSecret(context.Background(), test.jobInput, &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Error(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestWatcher(t *testing.T) {
	tests := []struct {
		msg               string
		watcherParams     []*WatcherParams
		want              []*ResourceWatcher
		wantError         string
		getClientSetError error
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg: "test empty watcher params",
		},
		{
			msg:           "test unknown watcher resource",
			wantError:     "unable to create watcher for unknown resource ",
			watcherParams: []*WatcherParams{{}},
		},
		{
			msg:           "test non empty watcher params",
			want:          []*ResourceWatcher{{}, {}, {}},
			watcherParams: []*WatcherParams{{ResourceName: constants.KubeRayResource}, {ResourceName: constants.KubeSparkResource}, {ResourceName: "pods"}},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			f := computemock.NewMockFactory(g)

			testCluster := v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{}, test.getClientSetError)

			p := Params{
				Factory: f,
				Helper:  NewHelper(),
				Logger:  zaptest.NewLogger(t),
			}

			k8sc1 := NewClient(p)

			// test
			res, err := k8sc1.Watcher(test.watcherParams, &testCluster)
			if test.wantError != "" {
				require.Nil(t, res)
				require.NotNil(t, err)
				require.Error(t, err)
			} else {
				require.Nil(t, err)
				require.Equal(t, len(test.want), len(res))
			}
		})
	}
}

func TestCreatePromConfigMap(t *testing.T) {
	tt := []struct {
		msg            string
		job            runtime.Object
		setupMock      func(g *gomock.Controller) Client
		configFilePath string
		wantError      bool
	}{
		{
			msg: "error from GetClientSetForCluster",
			setupMock: func(g *gomock.Controller) Client {
				f := computemock.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, assert.AnError)
				return Client{
					factory: f,
				}
			},
			wantError: true,
		},
		{
			msg: "prom config file not found",
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
				},
			},
			setupMock: func(g *gomock.Controller) Client {
				f := computemock.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, nil)
				h := NewMockHelper(g)
				return Client{
					factory: f,
					helper:  h,
				}
			},
			wantError: true,
		},
		{
			msg: "error creating configmap - create error",
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
				},
			},
			setupMock: func(g *gomock.Controller) Client {
				f := computemock.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
				h := NewMockHelper(g)
				h.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any(), string(corev1.ResourceConfigMaps), uke.RayLocalNamespace).
					Return(assert.AnError)
				return Client{
					factory: f,
					helper:  h,
				}
			},
			// use relative path for config file as per https://engwiki.uberinternal.com/display/GOMONOREPO/Read+files+from+a+test
			configFilePath: "prometheus.yml",
			wantError:      true,
		},
		{
			msg: "created configmap",
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
				},
			},
			setupMock: func(g *gomock.Controller) Client {
				f := computemock.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
				h := NewMockHelper(g)
				h.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any(), string(corev1.ResourceConfigMaps), uke.RayLocalNamespace).
					Return(nil)
				return Client{
					factory: f,
					helper:  h,
					logger:  zaptest.NewLogger(t),
				}
			},
			// use relative path for config file as per https://engwiki.uberinternal.com/display/GOMONOREPO/Read+files+from+a+test
			configFilePath: "prometheus.yml",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			c := test.setupMock(g)

			cluster := v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}

			err := c.CreatePromConfigMap(context.Background(), test.job, &cluster, test.configFilePath)
			if test.wantError {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestCreateSecret(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "failure - unknown type used for body",
			wantError: "unknown type used for body:",
			jobInput: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
				Spec: v2beta1pb.RayJobSpec{
					User: &v2beta1pb.UserInfo{
						Name: "test-user",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})

			g := gomock.NewController(t)
			f := computemock.NewMockFactory(g)

			testCluster := v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				CoreV1: c.RESTClient(),
			}, test.getClientSetError)

			mockTokenService := hadooptokenservicemock.NewMockGateway(g)
			mockTokenService.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
				&hadooptokenservice.GenerateTokensResponse{
					Tokens: []hadooptokenservice.Token{
						{
							ServiceType: "HDFS",
							Token:       "token",
						},
					},
					Credentials: base64.RawURLEncoding.EncodeToString([]byte("creds")),
				}, nil).AnyTimes()

			provider := secrets.New(secrets.Params{
				TokenGateway: mockTokenService,
			}).Provider

			k8sc := Client{
				factory:         f,
				helper:          NewHelper(),
				secretsProvider: provider,
			}

			// test
			err := k8sc.CreateSecret(context.Background(), test.jobInput, &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Error(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

// TestGetJobStatus tests the GetJobStatus method
func TestGetJobStatus(t *testing.T) {
	sparkJob := &v2beta1pb.SparkJob{
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
			ApplicationId: "123",
		},
	}

	testCluster := &v2beta1pb.Cluster{
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
		},
	}
	tests := []struct {
		msg          string
		jobInput     runtime.Object
		clusterInput *v2beta1pb.Cluster
		mockFunc     func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway)
		expectErr    string
	}{
		{
			msg:          "Get status for Ray Job",
			jobInput:     &v2beta1pb.RayJob{},
			clusterInput: testCluster,
			expectErr:    "GetStatus of RayJob is not supported",
		},
		{
			msg:          "Get status for spark job failed with token error",
			jobInput:     sparkJob,
			clusterInput: testCluster,
			mockFunc: func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway) {
				tokenGateway.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{}, fmt.Errorf("token error"),
				)
			},
			expectErr: "SparkClient.GetJobStatus: error getting hdfs delegation token, unable to get delegtaion tokens from token service, token error",
		},
		{
			msg:          "Get status for spark job failed with drogon error",
			jobInput:     sparkJob,
			clusterInput: testCluster,
			mockFunc: func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway) {
				tokenGateway.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEON",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonGateway.EXPECT().GetJobStatus(gomock.Any(), gomock.Any()).Return(&drogon.JobResponse{}, fmt.Errorf("drogon error"))
			},
			expectErr: "SparkClient.GetJobStatus: error getting job status from drogon, drogon error",
		},
		{
			msg:          "Get status for spark job failed with unsupported job",
			jobInput:     testCluster,
			clusterInput: testCluster,
			expectErr:    "the object must be a RayJob or a SparkJob, got:*v2beta1pb.Cluster",
		},
	}

	for _, test := range tests {
		gctrl := gomock.NewController(t)
		drogongateway := drogonmock.NewMockGateway(gctrl)
		hadoopTokenService := hadooptokenservicemock.NewMockGateway(gctrl)
		provider := secrets.New(secrets.Params{
			TokenGateway: hadoopTokenService,
		})
		mockFactory := computemock.NewMockFactory(gctrl)
		mockFlipr := fliprmock.NewMockFliprClient(gctrl)
		mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(gctrl)
		mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)
		mockSparkClient := uke.NewSparkClient(
			uke.SparkClientParams{
				DrogonGateway: drogongateway,
				Secrets:       provider.Provider,
				MetricsScope:  tally.NoopScope,
				MTLSHandler:   mTLSHandler,
			},
		)
		client := NewClient(Params{
			SparkClient: mockSparkClient,
			Factory:     mockFactory,
			Helper:      NewHelper(),
			Logger:      zaptest.NewLogger(t),
		})
		if test.mockFunc != nil {
			test.mockFunc(hadoopTokenService, drogongateway)
		}
		_, err := client.GetJobStatus(context.Background(), test.jobInput, test.clusterInput)
		if test.expectErr != "" {
			require.Equal(t, test.expectErr, err.Error())
		} else {
			require.Nil(t, err)
		}
	}
}

// TestCreateJobWithSparkClient tests the CreateJob method with SparkClient
func TestCreateJobWithSparkClient(t *testing.T) {
	sparkJob := &v2beta1pb.SparkJob{
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
				Cluster:      "dca11-batch01",
				ResourcePool: "/UberAI/Michelangelo/IntegrationTests",
			},
		},
	}

	testCluster := &v2beta1pb.Cluster{
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
		},
	}
	tests := []struct {
		msg          string
		jobInput     runtime.Object
		clusterInput *v2beta1pb.Cluster
		mockFunc     func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway)
		expectErr    string
	}{
		{
			msg:          "Create spark job failed with token error",
			jobInput:     sparkJob,
			clusterInput: testCluster,
			mockFunc: func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway) {
				tokenGateway.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{}, fmt.Errorf("token error"),
				)
			},
			expectErr: "unable to get delegtaion tokens from token service, token error",
		},
		{
			msg:          "Create spark job failed with partial token error",
			jobInput:     sparkJob,
			clusterInput: testCluster,
			mockFunc: func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway) {
				tokenGateway.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{}, hadooptokenservice.ErrPartialTokens,
				)
			},
			expectErr: fmt.Errorf("%w: %v", ErrRetryable, hadooptokenservice.ErrPartialTokens).Error(),
		},
		{
			msg:          "Create spark job failed with drogon error",
			jobInput:     sparkJob,
			clusterInput: testCluster,
			mockFunc: func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway) {
				tokenGateway.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEON",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonGateway.EXPECT().SubmitJob(gomock.Any(), gomock.Any()).Return(&drogon.JobResponse{}, fmt.Errorf("drogon error"))
			},
			expectErr: "drogon error",
		},
		{
			msg:          "Create spark job succeeded",
			jobInput:     sparkJob,
			clusterInput: testCluster,
			mockFunc: func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway) {
				tokenGateway.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEON",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonGateway.EXPECT().SubmitJob(gomock.Any(), gomock.Any()).Return(&drogon.JobResponse{
					ID: 123,
					AppInfo: drogon.AppInfo{
						DriverLogURL: "test-url",
					},
				}, nil)
			},
			expectErr: "",
		},
	}

	for _, test := range tests {
		gctrl := gomock.NewController(t)
		drogongateway := drogonmock.NewMockGateway(gctrl)
		hadoopTokenService := hadooptokenservicemock.NewMockGateway(gctrl)
		provider := secrets.New(secrets.Params{
			TokenGateway: hadoopTokenService,
		})
		mockFactory := computemock.NewMockFactory(gctrl)
		mockFlipr := fliprmock.NewMockFliprClient(gctrl)
		mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(gctrl)
		mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)

		mockSparkClient := uke.NewSparkClient(
			uke.SparkClientParams{
				DrogonGateway: drogongateway,
				Secrets:       provider.Provider,
				MetricsScope:  tally.NoopScope,
				MTLSHandler:   mTLSHandler,
			},
		)
		client := NewClient(Params{
			SparkClient: mockSparkClient,
			Factory:     mockFactory,
			Helper:      NewHelper(),
			Logger:      zaptest.NewLogger(t),
		})
		if test.mockFunc != nil {
			test.mockFunc(hadoopTokenService, drogongateway)
		}
		err := client.CreateJob(context.Background(), test.jobInput, test.clusterInput)
		if test.expectErr != "" {
			require.EqualError(t, err, test.expectErr)
		} else {
			require.Nil(t, err)
		}
	}
}

// TestDeleteJobWithSparkClient tests the DeleteJob method with SparkClient
func TestDeleteJobWithSparkClient(t *testing.T) {
	sparkJob := &v2beta1pb.SparkJob{
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
			ApplicationId: "123",
		},
	}

	testCluster := &v2beta1pb.Cluster{
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
		},
	}
	tests := []struct {
		msg          string
		jobInput     runtime.Object
		clusterInput *v2beta1pb.Cluster
		mockFunc     func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway)
		expectErr    string
	}{
		{
			msg:          "Cancel spark job failed with token error",
			jobInput:     sparkJob,
			clusterInput: testCluster,
			mockFunc: func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway) {
				tokenGateway.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{}, fmt.Errorf("token error"),
				)
			},
			expectErr: "unable to get delegtaion tokens from token service, token error",
		},
		{
			msg:          "Cancel spark job failed with drogon error",
			jobInput:     sparkJob,
			clusterInput: testCluster,
			mockFunc: func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway) {
				tokenGateway.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEON",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonGateway.EXPECT().CancelJob(gomock.Any(), gomock.Any()).Return(fmt.Errorf("drogon error"))
			},
			expectErr: "drogon error",
		},
		{
			msg:          "Cancel spark job succeeded",
			jobInput:     sparkJob,
			clusterInput: testCluster,
			mockFunc: func(tokenGateway *hadooptokenservicemock.MockGateway, drogonGateway *drogonmock.MockGateway) {
				tokenGateway.EXPECT().GenerateTokens(gomock.Any(), gomock.Any()).Return(
					&hadooptokenservice.GenerateTokensResponse{
						Tokens: []hadooptokenservice.Token{
							{
								ServiceType:   "HDFS",
								ServiceRegion: "DCA",
								ServiceAlias:  "NEON",
								Token:         "test-token",
							},
						},
					}, nil)
				drogonGateway.EXPECT().CancelJob(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectErr: "",
		},
	}

	for _, test := range tests {
		gctrl := gomock.NewController(t)
		drogongateway := drogonmock.NewMockGateway(gctrl)
		hadoopTokenService := hadooptokenservicemock.NewMockGateway(gctrl)
		provider := secrets.New(secrets.Params{
			TokenGateway: hadoopTokenService,
		})
		mockFactory := computemock.NewMockFactory(gctrl)
		mockFlipr := fliprmock.NewMockFliprClient(gctrl)
		mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(gctrl)
		mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)
		mockSparkClient := uke.NewSparkClient(
			uke.SparkClientParams{
				DrogonGateway: drogongateway,
				Secrets:       provider.Provider,
				MetricsScope:  tally.NoopScope,
				MTLSHandler:   mTLSHandler,
			},
		)
		client := NewClient(Params{
			SparkClient: mockSparkClient,
			Factory:     mockFactory,
			Helper:      NewHelper(),
			Logger:      zaptest.NewLogger(t),
		})
		if test.mockFunc != nil {
			test.mockFunc(hadoopTokenService, drogongateway)
		}
		err := client.DeleteJob(context.Background(), test.jobInput, test.clusterInput)
		if test.expectErr != "" {
			require.Equal(t, test.expectErr, err.Error())
		} else {
			require.Nil(t, err)
		}
	}
}

func TestCreateJob(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg: "dummy error from GetClientSetForCluster",
			jobInput: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec: v2beta1pb.RayJobSpec{
					Head: &v2beta1pb.HeadSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Memory: "32G",
							},
						},
					},
					Worker: &v2beta1pb.WorkerSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Memory: "32G",
							},
						},
					},
				},
				Status: v2beta1pb.RayJobStatus{
					Assignment: &v2beta1pb.AssignmentInfo{
						ResourcePool: "test-pool",
					},
				},
			},
			wantError:         "get client for cluster err:dummy error",
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "ray job create failure - encoding is not allowed for this codec",
			wantError: "create ray cluster err:encoding is not allowed for this codec: *versioning.codec",
			jobInput: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec: v2beta1pb.RayJobSpec{
					Head: &v2beta1pb.HeadSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Memory: "32G",
							},
						},
					},
					Worker: &v2beta1pb.WorkerSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Memory: "32G",
							},
						},
					},
				},
				Status: v2beta1pb.RayJobStatus{
					Assignment: &v2beta1pb.AssignmentInfo{
						ResourcePool: "test-pool",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			testCluster := v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
				Spec: v2beta1pb.ClusterSpec{
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "test",
							},
						},
					},
				},
			}

			g := gomock.NewController(t)

			// setup client
			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			f := computemock.NewMockFactory(g)
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				Ray:   c.RESTClient(),
				Spark: c.RESTClient(),
			}, test.getClientSetError)

			mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)
			mockFliprClient := fliprmock.NewMockFliprClient(g)
			if test.getClientSetError == nil {
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(map[string]interface{}{
					"gpu_sku": "",
				}).AnyTimes()
				mockFliprClient.EXPECT().GetStringValue(contextmatcher.Any(), "rayJobsDiskSpillSize", gomock.Any(), "").
					Return("140Gi", nil).AnyTimes()
			}

			project := &v2beta1pb.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Spec: v2beta1pb.ProjectSpec{
					Tier: 0,
					Owner: &v2beta1pb.OwnerInfo{
						OwningTeam: "test-team",
					},
				},
			}
			mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFliprClient, mockFliprConstraintsBuilder, false, false, nil, nil, project)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
				mapper: uke.NewUkeMapper(uke.MapperParams{
					FliprConstraintsBuilder: mockFliprConstraintsBuilder,
					FliprClient:             mockFliprClient,
					Scope:                   tally.NoopScope,
					MTLSHandler:             mTLSHandler,
				}).Mapper,
			}

			// test
			err := k8sc.CreateJob(context.Background(), test.jobInput, &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Equal(t, test.wantError, err.Error())
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func assertConditions(
	t *testing.T,
	conditions []*v2beta1pb.Condition,
	expectedConditionTypes []string,
	conditionStatus v2beta1pb.ConditionStatus) {
	wantCount := len(expectedConditionTypes)
	haveCount := 0
	for _, wantCond := range expectedConditionTypes {
		haveConditions := conditions
		for _, haveCond := range haveConditions {
			if wantCond == haveCond.Type {
				haveCount++
				require.Equal(t, conditionStatus, haveCond.Status)
			}
		}
	}
	require.Equal(t, wantCount, haveCount, "not all conditions were present")
}
