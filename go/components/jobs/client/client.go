package client

import (
	"context"
	"errors"
	"fmt"
	"os"

	hadoopclients "github.com/michelangelo-ai/michelangelo/go/components/hadoop/clients"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/uke"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/kuberay"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
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

func newClusterReadyCondition(t metav1.Time) *v2pb.Condition {
	return &v2pb.Condition{
		Type:                 constants.ClusterReady,
		Status:               v2pb.CONDITION_STATUS_TRUE,
		Reason:               ClusterReadyReason,
		Message:              ClusterReadyMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

func newClusterNotReadyCondition(t metav1.Time) *v2pb.Condition {
	return &v2pb.Condition{
		Type:                 constants.ClusterReady,
		Status:               v2pb.CONDITION_STATUS_FALSE,
		Reason:               ClusterNotReadyReason,
		Message:              ClusterNotReadyMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

func newClusterOfflineCondition(t metav1.Time) *v2pb.Condition {
	return &v2pb.Condition{
		Type:                 constants.ClusterOffline,
		Status:               v2pb.CONDITION_STATUS_TRUE,
		Reason:               ClusterNotReachableReason,
		Message:              ClusterNotReachableMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

func newClusterNotOfflineCondition(t metav1.Time) *v2pb.Condition {
	return &v2pb.Condition{
		Type:                 constants.ClusterOffline,
		Status:               v2pb.CONDITION_STATUS_FALSE,
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

	Factory     compute.Factory
	Logger      *zap.Logger
	Secrets     secrets.Provider
	Mapper      uke.Mapper `name:"ukeMapper"`
	Helper      Helper
	SparkClient *uke.SparkClient
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

	// GetJobStatus gets the job status from the specified cluster
	GetJobStatus(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) (constants.SparkJobStatus, error)

	// DeleteJob deletes a batch job from the specified cluster
	DeleteJob(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) error

	// CreateJob creates a job on the specified cluster
	CreateJob(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) error

	// Watcher creates resource watchers on the specified cluster
	Watcher(watcherParams []*WatcherParams, cluster *v2pb.Cluster) ([]*ResourceWatcher, error)

	// GetClusterStatus gets the kubernetes cluster's health and version status
	GetClusterStatus(ctx context.Context, cluster *v2pb.Cluster) (*v2pb.ClusterStatus, error)

	// GetResourcePools fetches the resource pools from the specified cluster
	GetResourcePools(ctx context.Context, cluster *v2pb.Cluster) (types.ResourcePoolList, error)

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
		sparkClient:     p.SparkClient,
	}
}

// Client for UKE clusters
type Client struct {
	factory         compute.Factory
	logger          *zap.Logger
	secretsProvider secrets.Provider
	mapper          uke.Mapper
	helper          Helper
	sparkClient     *uke.SparkClient
}

// CreatePromConfigMap creates the prom configmap
func (c *Client) CreatePromConfigMap(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2pb.Cluster,
	configFile string) error {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return err
	}

	//create config from file
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
	cluster *v2pb.Cluster) error {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return err
	}

	secretData, err := c.secretsProvider.GenerateHadoopSecret(
		ctx,
		jobObject,
		cluster)
	if err != nil {
		return fmt.Errorf("generate hadoop secret err:%w", err)
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
	cluster *v2pb.Cluster) error {
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
	cluster *v2pb.Cluster) error {
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

// GetJobStatus gets the job status from the cluster
func (c *Client) GetJobStatus(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) (constants.SparkJobStatus, error) {
	switch job := jobObject.(type) {
	case *v2pb.RayJob:
		return "", fmt.Errorf("GetStatus of RayJob is not supported")
	case *v2pb.SparkJob:
		return c.sparkClient.GetJobStatus(ctx, job, cluster)
	}
	return "", fmt.Errorf("the object must be a RayJob or a SparkJob, got:%T", jobObject)
}

// DeleteJob deletes a batch job from the local cluster
func (c *Client) DeleteJob(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2pb.Cluster) error {
	switch job := jobObject.(type) {
	case *v2pb.RayJob:
		cs, err := c.factory.GetClientSetForCluster(cluster)
		if err != nil {
			return err
		}
		localNamespace, localName := c.mapper.GetLocalName(jobObject)
		return c.helper.DeleteResource(ctx, cs.Ray, constants.KubeRayResource, localNamespace, localName, metav1.DeleteOptions{})
	case *v2pb.SparkJob:
		return c.sparkClient.CancelJob(ctx, job, cluster)
	}
	return fmt.Errorf("unrecognized job type")
}

// CreateJob sends request to Compute to create Ray job on the cluster
// This will create pod spec with volume and then create raycluster
func (c *Client) CreateJob(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2pb.Cluster) error {
	switch job := jobObject.(type) {
	case *v2pb.RayJob:
		cs, err := c.factory.GetClientSetForCluster(cluster)
		if err != nil {
			return fmt.Errorf("get client for cluster err:%v", err)
		}
		op, err := c.mapper.MapGlobalToLocal(jobObject, cluster)
		if err != nil {
			return err
		}
		var jobName string
		rayCluster := op.(*kuberay.RayCluster)
		jobName = rayCluster.Name
		err = c.helper.CreateResource(
			ctx,
			cs.Ray,
			op,
			constants.KubeRayResource,
			rayCluster.Namespace)
		if err != nil {
			return fmt.Errorf("create ray cluster err:%w", err)
		}
		c.logger.
			With(
				zap.String("job_name", jobName),
				zap.String("cluster", cluster.Name)).
			Info("Successfully created job in uke cluster")
		return nil
	case *v2pb.SparkJob:
		err := c.sparkClient.SubmitJob(ctx, job, cluster)
		if err != nil {
			if errors.Is(err, hadoopclients.ErrPartialTokens) {
				return fmt.Errorf("%w: %v", ErrRetryable, err)
			}
			return err
		}
		return nil
	}
	return nil
}

// Watcher creates a pod watch on Compute API server
func (c *Client) Watcher(watcherParams []*WatcherParams, cluster *v2pb.Cluster) (
	[]*ResourceWatcher, error) {
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

// GetResourcePools fetches the resource pools from the cluster
func (c *Client) GetResourcePools(ctx context.Context, cluster *v2pb.Cluster) (
	types.ResourcePoolList, error) {
	return types.ResourcePoolList{}, nil
}

var _gpuSkuListNamespace = "special-resource-list"

// GetSkuConfigMaps fetch the config maps from the cluster
func (c *Client) GetSkuConfigMaps(ctx context.Context, cluster *v2pb.Cluster) (
	corev1.ConfigMapList, error) {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return corev1.ConfigMapList{}, err
	}

	result := corev1.ConfigMapList{}
	err = c.helper.ListResources(ctx, cs.CoreV1, corev1.ResourceConfigMaps.String(), _gpuSkuListNamespace, &result)
	if err != nil {
		return corev1.ConfigMapList{}, err
	}

	c.logger.Info("fetched gpu config maps from cluster",
		zap.Int("count", len(result.Items)),
		zap.String("cluster", cluster.Name))

	return result, nil
}
