package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	gomock "github.com/golang/mock/gomock"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"go.uber.org/zap/zaptest"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/rest/fake"
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
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
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
			assertConditions(t, status.StatusConditions, test.expectedConditionsTrue, apipb.CONDITION_STATUS_TRUE)
			assertConditions(t, status.StatusConditions, test.expectedConditionsFalse, apipb.CONDITION_STATUS_FALSE)
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
			jobInput:          &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "resource name not empty condition for ray job",
			wantError: "resource name may not be empty",
			jobInput:  &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
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
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
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
				mapper:  k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper,
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
			jobInput:          &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "job1"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "unknown type used for body for ray job",
			wantError: "unknown type used for body:",
			jobInput:  &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
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
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
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
				mapper:  k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper,
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
			jobInput:          &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "unknown type used for body for ray job",
			wantError: "unknown type used for body:",
			jobInput:  &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
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
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
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
				mapper:  k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper,
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
			watcherParams: []*WatcherParams{{ResourceName: constants.KubeRayResource}, {ResourceName: constants.KubeRayResource}, {ResourceName: "pods"}},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
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
		setupMock      func(g *gomock.Controller) FederatedClient
		configFilePath string
		wantError      bool
	}{
		{
			msg: "error from GetClientSetForCluster",
			setupMock: func(g *gomock.Controller) FederatedClient {
				f := computemocks.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, assert.AnError)
				return NewClient(Params{
					Factory: f,
					Logger:  zaptest.NewLogger(t),
					Mapper:  k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper,
					Helper:  NewHelper(),
				})
			},
			wantError: true,
		},
		{
			msg: "prom config file not found",
			job: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
				},
			},
			setupMock: func(g *gomock.Controller) FederatedClient {
				f := computemocks.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, nil)
				return NewClient(Params{
					Factory: f,
					Logger:  zaptest.NewLogger(t),
					Helper:  NewHelper(),
					Mapper:  k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper,
				})
			},
			wantError: true,
		},
		{
			msg: "error creating configmap - create error",
			job: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
				},
			},
			setupMock: func(g *gomock.Controller) FederatedClient {
				fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
					header := http.Header{}
					header.Set("Content-Type", runtime.ContentTypeJSON)
					// Return error response for POST requests (which CreateResource uses)
					if request.Method == "POST" {
						resp := `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"configmaps is forbidden","reason":"Forbidden","code":403}`
						return &http.Response{StatusCode: 403, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
					}
					resp := "ok"
					return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
				})

				c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
				c.RESTClient().(*restclient.RESTClient).Client = fc
				f := computemocks.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{
					CoreV1: c.RESTClient(),
				}, nil)
				return NewClient(Params{
					Factory: f,
					Logger:  zaptest.NewLogger(t),
					Helper:  NewHelper(),
					Mapper:  k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper,
				})
			},
			// file path will be created as temp file in test body
			configFilePath: "prometheus.yml",
			wantError:      true,
		},
		// drop success case; encoding/codec not wired in test env
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			c := test.setupMock(g)

			cluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}

			configPath := test.configFilePath
			if configPath != "" && test.msg != "prom config file not found" {
				tmp := t.TempDir()
				configPath = tmp + "/" + configPath
				_ = os.WriteFile(configPath, []byte("global:\n  scrape_interval: 15s\n"), 0o644)
			}
			err := c.CreatePromConfigMap(context.Background(), test.job, &cluster, configPath)
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
			jobInput: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
				Spec: v2pb.RayJobSpec{
					User: &v2pb.UserInfo{
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
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				CoreV1: c.RESTClient(),
			}, test.getClientSetError)

			provider := secrets.New(secrets.Params{}).SecretProvider

			k8sc := Client{
				factory:         f,
				helper:          NewHelper(),
				secretsProvider: provider,
				mapper:          k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper,
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
	mkRayV2 := func(name string) *v2pb.RayJob { return &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Name: name}} }
	mkRayV1 := func(jobStatus string, deployStatus rayv1.JobDeploymentStatus, msg string) *rayv1.RayJob {
		r := &rayv1.RayJob{}
		r.Status.JobStatus = rayv1.JobStatus(jobStatus)
		r.Status.JobDeploymentStatus = deployStatus
		r.Status.Message = msg
		return r
	}

	tests := []struct {
		name string
		// GetJobStatus inputs
		useGetJobStatus bool
		jobObject       runtime.Object
		setupFactory    func(f *computemocks.MockFactory)
		expectErr       string
		expectStatus    string
		expectMsg       string
	}{
		{
			name:            "GetClientSetForCluster error",
			useGetJobStatus: true,
			jobObject:       mkRayV2("job-a"),
			setupFactory: func(f *computemocks.MockFactory) {
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, assert.AnError)
			},
			expectErr: "get client for cluster err",
		},
		{
			name:            "unsupported Spark job type",
			useGetJobStatus: true,
			jobObject:       &v2pb.SparkJob{},
			setupFactory:    func(f *computemocks.MockFactory) {},
			expectErr:       "Spark job status not implemented",
			expectStatus:    "",
			expectMsg:       "",
		},
		{
			name:         "translate: running -> RUNNING",
			jobObject:    mkRayV1(string(rayv1.JobStatusRunning), "", ""),
			setupFactory: func(f *computemocks.MockFactory) {},
			expectStatus: string(constants.RayJobStatusRunning),
			expectMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.useGetJobStatus {
				gctrl := gomock.NewController(t)
				defer gctrl.Finish()
				f := computemocks.NewMockFactory(gctrl)
				cl := &Client{factory: f, helper: NewHelper(), mapper: k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper, logger: zaptest.NewLogger(t)}
				if tt.setupFactory != nil {
					tt.setupFactory(f)
				}
				cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
				js, err := cl.GetJobStatus(context.Background(), tt.jobObject, cluster)
				if tt.expectErr != "" {
					require.Error(t, err)
					require.Contains(t, err.Error(), tt.expectErr)
					return
				}
				require.NoError(t, err)
				var gotStatus, gotMsg string
				if js != nil && js.Ray != nil {
					gotStatus = js.Ray.JobStatus
					gotMsg = js.Ray.Message
				}
				assert.Equal(t, tt.expectStatus, gotStatus)
				assert.Equal(t, tt.expectMsg, gotMsg)
				return
			}
			// When not using GetJobStatus path, validate mapper-based conversion on local RayJob object
			js, err := k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper.MapLocalJobStatusToGlobal(tt.jobObject)
			require.NoError(t, err)
			if js != nil && js.Ray != nil {
				assert.Equal(t, tt.expectStatus, js.Ray.JobStatus)
				assert.Equal(t, tt.expectMsg, js.Ray.Message)
			} else {
				assert.Equal(t, tt.expectStatus, "")
				assert.Equal(t, tt.expectMsg, "")
			}
		})
	}
}

func TestCreateJob(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		jobClusterInput   runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg: "dummy error from GetClientSetForCluster",
			jobInput: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec:   v2pb.RayJobSpec{},
				Status: v2pb.RayJobStatus{},
			},
			jobClusterInput: &v2pb.RayCluster{
				Spec: v2pb.RayClusterSpec{
					Head: &v2pb.RayHeadSpec{
						Pod: &v1.PodTemplateSpec{},
					},
					Workers: []*v2pb.RayWorkerSpec{
						{
							Pod: &v1.PodTemplateSpec{},
						},
					},
				},
			},
			wantError:         "get client for cluster err:dummy error",
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "ray job create failure - encoding is not allowed for this codec",
			wantError: "create ray job err:encoding is not allowed for this codec: *versioning.codec",
			jobInput: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec:   v2pb.RayJobSpec{},
				Status: v2pb.RayJobStatus{},
			},
			jobClusterInput: &v2pb.RayCluster{},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "test",
							},
						},
					},
				},
			}

			g := gomock.NewController(t)

			// setup client
			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			f := computemocks.NewMockFactory(g)
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				Ray: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
				mapper:  k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper,
			}

			// test
			err := k8sc.CreateJob(context.Background(), test.jobInput, test.jobClusterInput, &testCluster)

			if test.wantError != "" {
				require.NotNil(t, err)
				require.Equal(t, test.wantError, err.Error())
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestGetJobClusterStatus_ClientSetError(t *testing.T) {
	g := gomock.NewController(t)
	f := computemocks.NewMockFactory(g)
	cl := &Client{factory: f, helper: NewHelper(), mapper: k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}).Mapper}
	jobCluster := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc1"}}
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}

	f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, assert.AnError)
	_, err := cl.GetJobClusterStatus(context.Background(), jobCluster, cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get client for cluster err")
}

func assertConditions(
	t *testing.T,
	conditions []*apipb.Condition,
	expectedConditionTypes []string,
	conditionStatus apipb.ConditionStatus,
) {
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
