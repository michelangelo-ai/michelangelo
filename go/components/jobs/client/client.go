//go:generate mamockgen FederatedClient
package client

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"go.uber.org/fx"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ClusterConditions Reasons
const (
	ClusterReadyReason        = "ClusterReadyReason"
	ClusterNotReadyReason     = "ClusterNotReadyReason"
	ClusterReachableReason    = "ClusterReachable"
	ClusterNotReachableReason = "ClusterNotReachable"
)

// ClusterConditions messages
const (
	ClusterReadyMsg        = "/healthz responded with ok"
	ClusterNotReadyMsg     = "/healthz responded without ok"
	ClusterReachableMsg    = "cluster is reachable"
	ClusterNotReachableMsg = "cluster is not reachable"
)

const (
	_zeroGracePeriodSeconds = int64(0)
)

// ErrRetryable indicates that the operation failed but can be retried.
// Callers should check using errors.Is(err, ErrRetryable) to determine if they should retry.
var ErrRetryable = errors.New("operation failed but is retryable")

func newClusterReadyCondition(t metav1.Time) *apipb.Condition {
	return &apipb.Condition{
		Type:                 constants.ClusterReady,
		Status:               apipb.CONDITION_STATUS_TRUE,
		Reason:               ClusterReadyReason,
		Message:              ClusterReadyMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

func newClusterNotReadyCondition(t metav1.Time) *apipb.Condition {
	return &apipb.Condition{
		Type:                 constants.ClusterReady,
		Status:               apipb.CONDITION_STATUS_FALSE,
		Reason:               ClusterNotReadyReason,
		Message:              ClusterNotReadyMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

func newClusterOfflineCondition(t metav1.Time) *apipb.Condition {
	return &apipb.Condition{
		Type:                 constants.ClusterOffline,
		Status:               apipb.CONDITION_STATUS_TRUE,
		Reason:               ClusterNotReachableReason,
		Message:              ClusterNotReachableMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

func newClusterNotOfflineCondition(t metav1.Time) *apipb.Condition {
	return &apipb.Condition{
		Type:                 constants.ClusterOffline,
		Status:               apipb.CONDITION_STATUS_FALSE,
		Reason:               ClusterReachableReason,
		Message:              ClusterReachableMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

// ResourceWatcher contains the result watcher for a resource
type ResourceWatcher struct {
	Store      cache.Store
	Controller cache.Controller
	StopCh     chan struct{}
}

// Params is the input to the constructor
type Params struct {
	fx.In

	Factory compute.Factory
	Logger  *zap.Logger
	Secrets secrets.SecretProvider
	Mapper  k8sengine.MapperInterface `name:"k8sengineMapper"`
	Helper  Helper
}

// This is the key in the config map. This is just the file name so that it can be referenced in conjunction with the configmap mount path
var _prometheusConfigMapKeyName = "prometheus.yml"

// FederatedClient defines the interface for managing resources across multiple clusters
type FederatedClient interface {
	// CreatePromConfigMap creates the prometheus configmap in the specified cluster
	CreatePromConfigMap(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster, configFile string) error

	// CreateSecret creates the secret in the specified cluster
	CreateSecret(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) error

	// DeletePromConfigMap deletes the prometheus configmap from the specified cluster
	DeletePromConfigMap(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) error

	// DeleteSecret deletes the secret from the specified cluster
	DeleteSecret(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) error

	// GetJobStatus gets the Ray job status from the specified cluster
	GetJobStatus(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) (*matypes.JobStatus, error)

	// DeleteJob deletes a batch job from the specified cluster
	DeleteJob(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) error

	// CreateJob creates a job on the specified cluster
	CreateJob(ctx context.Context, jobObject, jobClusterObject runtime.Object, cluster *v2pb.Cluster) error

	// CreateJobCluster creates a cluster on the specified cluster
	CreateJobCluster(ctx context.Context, jobClusterObject runtime.Object, cluster *v2pb.Cluster) error

	// DeleteJobCluster deletes a job cluster from the specified cluster
	DeleteJobCluster(ctx context.Context, jobClusterObject runtime.Object, cluster *v2pb.Cluster) error

	// GetJobClusterStatus gets the job cluster status from the specified cluster
	GetJobClusterStatus(ctx context.Context, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (*matypes.JobClusterStatus, error)

	// Watcher creates resource watchers on the specified cluster
	Watcher(watcherParams []*WatcherParams, cluster *v2pb.Cluster) ([]*ResourceWatcher, error)

	// GetClusterStatus gets the kubernetes cluster's health and version status
	GetClusterStatus(ctx context.Context, cluster *v2pb.Cluster) (*v2pb.ClusterStatus, error)

	// GetSkuConfigMaps fetch the config maps from the specified cluster
	GetSkuConfigMaps(ctx context.Context, cluster *v2pb.Cluster) (corev1.ConfigMapList, error)
}

// NewClient is the constructor
func NewClient(p Params) FederatedClient {
	return &Client{
		factory:         p.Factory,
		logger:          p.Logger.With(zap.String("component", "client")),
		secretsProvider: p.Secrets,
		mapper:          p.Mapper,
		helper:          p.Helper,
	}
}

// Client for Jobs clusters
type Client struct {
	factory         compute.Factory
	logger          *zap.Logger
	secretsProvider secrets.SecretProvider
	mapper          k8sengine.MapperInterface
	helper          Helper
}

// CreatePromConfigMap creates the prom configmap
func (c *Client) CreatePromConfigMap(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2pb.Cluster,
	configFile string,
) error {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return err
	}

	// create config from file
	yamlFile, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", configFile, err)
	}

	localNamespace, localName := c.mapper.GetLocalName(jobObject)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: localNamespace,
			Name:      GetPrometheusConfigMapName(localName),
		},
		Data: map[string]string{
			_prometheusConfigMapKeyName: string(yamlFile),
		},
	}

	err = c.helper.CreateResource(
		ctx,
		cs.CoreV1,
		cm,
		string(corev1.ResourceConfigMaps),
		localNamespace)
	if err != nil {
		return fmt.Errorf("create prom config err:%w", err)
	}
	c.logger.Info("Successfully created prom config in cluster", zap.String("cluster", cluster.Name))
	return nil
}

// GetPrometheusConfigMapName provides the constructed prom configmap name for a given job
func GetPrometheusConfigMapName(jobName string) string {
	return fmt.Sprintf("prom-%s", jobName)
}

// CreateSecret creates the secret in the cluster
func (c *Client) CreateSecret(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2pb.Cluster,
) error {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return err
	}

	secretData, err := c.secretsProvider.GetSecretsForDataStore(
		ctx,
		jobObject,
		cluster)
	if err != nil {
		return fmt.Errorf("get secrets for job err:%w", err)
	}

	labels := map[string]string{
		constants.SecretAppNameKey: constants.SecretAppNameValue,
	}

	localNamespace, localName := c.mapper.GetLocalName(jobObject)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secrets.GetKubeSecretName(localName),
			Namespace: localNamespace,
			Labels:    labels,
		},
		Data: secretData,
	}

	err = c.helper.CreateResource(
		ctx,
		cs.CoreV1,
		secret,
		string(corev1.ResourceSecrets),
		localNamespace)
	if err != nil {
		return fmt.Errorf("create secret err:%w", err)
	}
	c.logger.Info("Successfully created secret in cluster", zap.String("cluster", cluster.Name))
	return nil
}

// DeletePromConfigMap deletes the prom configmap with gracePeriodSeconds=0
func (c *Client) DeletePromConfigMap(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2pb.Cluster,
) error {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return err
	}

	// Giving grace period as 0 secs force deletes the configmap immediately and doesn't wait for graceful delete
	gracePeriodSeconds := _zeroGracePeriodSeconds
	opts := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}

	localNamespace, localName := c.mapper.GetLocalName(jobObject)

	return c.helper.DeleteResource(ctx, cs.CoreV1, string(corev1.ResourceConfigMaps), localNamespace, GetPrometheusConfigMapName(localName), opts)
}

// DeleteSecret deletes the secret with gracePeriodSeconds=0
func (c *Client) DeleteSecret(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2pb.Cluster,
) error {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return err
	}

	// Giving grace period as 0 secs force deletes the secret immediately and doesn't wait for graceful delete
	gracePeriodSeconds := _zeroGracePeriodSeconds
	opts := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}

	localNamespace, localName := c.mapper.GetLocalName(jobObject)

	return c.helper.DeleteResource(ctx, cs.CoreV1, string(corev1.ResourceSecrets), localNamespace, secrets.GetKubeSecretName(localName), opts)
}

// GetJobStatus retrieves the status of a Ray job from a remote cluster
func (c *Client) GetJobStatus(
	ctx context.Context,
	jobObject runtime.Object,
	kubeCluster *v2pb.Cluster,
) (*matypes.JobStatus, error) {
	switch jobObject.(type) {
	case *v2pb.RayJob:
		cs, err := c.factory.GetClientSetForCluster(kubeCluster)
		if err != nil {
			return nil, fmt.Errorf("get client for cluster err:%v", err)
		}

		localNamespace, localName := c.mapper.GetLocalName(jobObject)

		// Get the KubeRay RayJob resource
		rayV1Job := &rayv1.RayJob{}
		err = c.helper.GetResource(
			ctx,
			cs.Ray,
			constants.KubeRayJobResource,
			localNamespace,
			localName,
			rayV1Job,
		)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return &matypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: "Unknown",
						State:     v2pb.RAY_JOB_STATE_FAILED,
						Message:   "Resource not found in compute cluster",
					},
				}, nil
			}
			return nil, fmt.Errorf("get ray job %q status: %w", localName, err)
		}

		jobStatus, err := c.mapper.MapLocalJobStatusToGlobal(rayV1Job)
		if err != nil {
			return nil, fmt.Errorf("map job status: %w", err)
		}
		return jobStatus, nil
	case *v2pb.SparkJob:
		// TODO(#616): Implement Spark job status
		return nil, fmt.Errorf("Spark job status not implemented")
	}
	return nil, fmt.Errorf("unsupported job type")
}

// DeleteJob deletes a batch job from the local cluster
func (c *Client) DeleteJob(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2pb.Cluster,
) error {
	switch jobObject.(type) {
	case *v2pb.RayJob:
		cs, err := c.factory.GetClientSetForCluster(cluster)
		if err != nil {
			return err
		}
		localNamespace, localName := c.mapper.GetLocalName(jobObject)
		return c.helper.DeleteResource(ctx, cs.Ray, constants.KubeRayJobResource, localNamespace, localName, metav1.DeleteOptions{})
	case *v2pb.SparkJob:
		return fmt.Errorf("DeleteJob of SparkJob is not supported")
	}
	return fmt.Errorf("unrecognized job type")
}

// CreateJob sends request to Compute to create Ray job on the cluster
// This will create pod spec with volume and then create raycluster
func (c *Client) CreateJob(
	ctx context.Context,
	jobObject runtime.Object,
	jobClusterObject runtime.Object,
	kubeCluster *v2pb.Cluster,
) error {
	switch job := jobObject.(type) {
	case *v2pb.RayJob:
		cs, err := c.factory.GetClientSetForCluster(kubeCluster)
		if err != nil {
			return fmt.Errorf("get client for cluster err:%v", err)
		}

		kubeRayJob, err := c.mapper.MapGlobalJobToLocal(jobObject, jobClusterObject, kubeCluster)
		if err != nil {
			return fmt.Errorf("map global to local err:%w", err)
		}
		// always use mapper's local namespace/name to avoid inconsistencies
		localNamespace, _ := c.mapper.GetLocalName(jobObject)
		err = c.helper.CreateResource(
			ctx,
			cs.Ray,
			kubeRayJob,
			constants.KubeRayJobResource,
			localNamespace)
		if err != nil {
			return fmt.Errorf("create ray job err:%w", err)
		}
		c.logger.Info("Successfully created job in ray cluster", zap.String(constants.Job, job.GetName()), zap.String("namespace", localNamespace), zap.String("cluster", job.Spec.GetCluster().GetName()))

		return nil
	case *v2pb.SparkJob:
		// NOTE: Implement Spark job creation
		panic("Spark job creation not implemented")
	}
	return nil
}

func (c *Client) CreateJobCluster(ctx context.Context, jobClusterObject runtime.Object, kubeCluster *v2pb.Cluster) error {
	cs, err := c.factory.GetClientSetForCluster(kubeCluster)
	if err != nil {
		return fmt.Errorf("get client for cluster err:%v", err)
	}

	localNamespace, _ := c.mapper.GetLocalName(jobClusterObject)
	localCluster, err := c.mapper.MapGlobalJobClusterToLocal(jobClusterObject, kubeCluster)
	if err != nil {
		return fmt.Errorf("map global to local err:%w", err)
	}

	err = c.helper.CreateResource(
		ctx,
		cs.Ray,
		localCluster,
		constants.KubeRayResource,
		localNamespace)
	if err != nil {
		return fmt.Errorf("create ray cluster err:%w", err)
	}
	return nil
}

// DeleteJobCluster deletes a job cluster from the specified cluster
func (c *Client) DeleteJobCluster(ctx context.Context, jobClusterObject runtime.Object, kubeCluster *v2pb.Cluster) error {
	switch jobClusterObject.(type) {
	case *v2pb.RayCluster:
		cs, err := c.factory.GetClientSetForCluster(kubeCluster)
		if err != nil {
			return fmt.Errorf("get client for cluster err:%v", err)
		}

		localNamespace, localName := c.mapper.GetLocalName(jobClusterObject)

		err = c.helper.DeleteResource(ctx, cs.Ray, constants.KubeRayResource, localNamespace, localName, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("delete ray cluster err:%w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported cluster type: %T", jobClusterObject)
	}
}

// GetJobClusterStatus gets the job cluster status from the specified cluster
func (c *Client) GetJobClusterStatus(
	ctx context.Context,
	jobClusterObject runtime.Object,
	kubeCluster *v2pb.Cluster,
) (*matypes.JobClusterStatus, error) {
	switch jobClusterObject.(type) {
	case *v2pb.RayCluster:
		cs, err := c.factory.GetClientSetForCluster(kubeCluster)
		if err != nil {
			return nil, fmt.Errorf("get client for cluster err:%v", err)
		}

		localNamespace, localName := c.mapper.GetLocalName(jobClusterObject)

		// Get the KubeRay RayCluster resource
		rayV1Cluster := &rayv1.RayCluster{}
		err = c.helper.GetResource(
			ctx,
			cs.Ray,
			constants.KubeRayResource,
			localNamespace,
			localName,
			rayV1Cluster,
		)
		if err != nil {
			return nil, fmt.Errorf("get ray cluster %q status: %w", localName, err)
		}

		// Convert the KubeRay v1 status to our internal ClusterStatus type
		clusterStatus, err := c.mapper.MapLocalClusterStatusToGlobal(rayV1Cluster)
		if err != nil {
			return nil, fmt.Errorf("map cluster status: %w", err)
		}

		return clusterStatus, nil
	default:
		return nil, fmt.Errorf("unsupported cluster type: %T", jobClusterObject)
	}
}

// Watcher creates a pod watch on Compute API server
func (c *Client) Watcher(watcherParams []*WatcherParams, cluster *v2pb.Cluster) (
	[]*ResourceWatcher, error,
) {
	clientSet, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return nil, err
	}

	// Add appropriate client for each watcherParams
	for _, wp := range watcherParams {
		switch wp.ResourceName {
		case constants.KubeRayResource:
			wp.Client = clientSet.Ray
		case corev1.ResourcePods.String(), corev1.ResourceConfigMaps.String():
			wp.Client = clientSet.CoreV1
		case constants.KubeSparkResource:
			wp.Client = clientSet.Spark
		default:
			return nil, fmt.Errorf("unable to create watcher for unknow resource %v", wp.ResourceName)
		}
	}

	return c.helper.Watcher(watcherParams)
}

// GetClusterStatus gets the kubernetes cluster's health and version status
func (c *Client) GetClusterStatus(ctx context.Context, cluster *v2pb.Cluster) (*v2pb.ClusterStatus, error) {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return nil, err
	}

	return c.helper.GetClusterHealth(ctx, cs.CoreV1)
}

// GetSkuConfigMaps fetch the config maps from the cluster
func (c *Client) GetSkuConfigMaps(ctx context.Context, cluster *v2pb.Cluster) (
	corev1.ConfigMapList, error,
) {
	return corev1.ConfigMapList{}, fmt.Errorf("GetSkuConfigMaps is not supported")
}
