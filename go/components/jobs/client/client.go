package client

import (
	"context"
	"errors"
	"fmt"
	"os"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta1"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client/uke"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/secrets"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/compute"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/ray/kuberay"
	"code.uber.internal/uberai/michelangelo/shared/gateways/hadooptokenservice"
	"go.uber.org/fx"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	v2beta1pb "michelangelo/api/v2beta1"
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

func newClusterReadyCondition(t metav1.Time) *v2beta1pb.Condition {
	return &v2beta1pb.Condition{
		Type:                 constants.ClusterReady,
		Status:               v2beta1pb.CONDITION_STATUS_TRUE,
		Reason:               ClusterReadyReason,
		Message:              ClusterReadyMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

func newClusterNotReadyCondition(t metav1.Time) *v2beta1pb.Condition {
	return &v2beta1pb.Condition{
		Type:                 constants.ClusterReady,
		Status:               v2beta1pb.CONDITION_STATUS_FALSE,
		Reason:               ClusterNotReadyReason,
		Message:              ClusterNotReadyMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

func newClusterOfflineCondition(t metav1.Time) *v2beta1pb.Condition {
	return &v2beta1pb.Condition{
		Type:                 constants.ClusterOffline,
		Status:               v2beta1pb.CONDITION_STATUS_TRUE,
		Reason:               ClusterNotReachableReason,
		Message:              ClusterNotReachableMsg,
		LastUpdatedTimestamp: t.Unix(),
	}
}

func newClusterNotOfflineCondition(t metav1.Time) *v2beta1pb.Condition {
	return &v2beta1pb.Condition{
		Type:                 constants.ClusterOffline,
		Status:               v2beta1pb.CONDITION_STATUS_FALSE,
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

// NewClient is the constructor
func NewClient(p Params) *Client {
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

// This is the key in the config map. This is just the file name so that it can be referenced in conjunction with the configmap mount path
var _prometheusConfigMapKeyName = "prometheus.yml"

// CreatePromConfigMap creates the prom configmap
func (c *Client) CreatePromConfigMap(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2beta1pb.Cluster,
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
	cluster *v2beta1pb.Cluster) error {
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
	cluster *v2beta1pb.Cluster) error {
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
	cluster *v2beta1pb.Cluster) error {
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
func (c *Client) GetJobStatus(ctx context.Context, jobObject runtime.Object, cluster *v2beta1pb.Cluster) (constants.SparkJobStatus, error) {
	switch job := jobObject.(type) {
	case *v2beta1pb.RayJob:
		return "", fmt.Errorf("GetStatus of RayJob is not supported")
	case *v2beta1pb.SparkJob:
		return c.sparkClient.GetJobStatus(ctx, job, cluster)
	}
	return "", fmt.Errorf("the object must be a RayJob or a SparkJob, got:%T", jobObject)
}

// DeleteJob deletes a batch job from the local cluster
func (c *Client) DeleteJob(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2beta1pb.Cluster) error {
	switch job := jobObject.(type) {
	case *v2beta1pb.RayJob:
		cs, err := c.factory.GetClientSetForCluster(cluster)
		if err != nil {
			return err
		}
		localNamespace, localName := c.mapper.GetLocalName(jobObject)
		return c.helper.DeleteResource(ctx, cs.Ray, constants.KubeRayResource, localNamespace, localName, metav1.DeleteOptions{})
	case *v2beta1pb.SparkJob:
		return c.sparkClient.CancelJob(ctx, job, cluster)
	}
	return fmt.Errorf("unrecognized job type")
}

// CreateJob sends request to Compute to create Ray job on the cluster
// This will create pod spec with volume and then create raycluster
func (c *Client) CreateJob(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2beta1pb.Cluster) error {
	switch job := jobObject.(type) {
	case *v2beta1pb.RayJob:
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
	case *v2beta1pb.SparkJob:
		err := c.sparkClient.SubmitJob(ctx, job, cluster)
		if err != nil {
			if errors.Is(err, hadooptokenservice.ErrPartialTokens) {
				return fmt.Errorf("%w: %v", ErrRetryable, err)
			}
			return err
		}
		return nil
	}
	return nil
}

// Watcher creates a pod watch on Compute API server
func (c *Client) Watcher(watcherParams []*WatcherParams, cluster *v2beta1pb.Cluster) (
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
func (c *Client) GetClusterStatus(ctx context.Context, cluster *v2beta1pb.Cluster) (*v2beta1pb.ClusterStatus, error) {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return nil, err
	}

	return c.helper.GetClusterHealth(ctx, cs.CoreV1)
}

// GetResourcePools fetches the resource pools from the cluster
func (c *Client) GetResourcePools(ctx context.Context, cluster *v2beta1pb.Cluster) (
	infraCrds.ResourcePoolList, error) {
	cs, err := c.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return infraCrds.ResourcePoolList{}, err
	}

	result := infraCrds.ResourcePoolList{}
	err = c.helper.ListResources(ctx, cs.ComputeV1, "resourcepools", metav1.NamespaceNone, &result)
	if err != nil {
		return infraCrds.ResourcePoolList{}, err
	}

	c.logger.Info("fetched resource pools from cluster",
		zap.Int("count", len(result.Items)),
		zap.String("cluster", cluster.Name))

	return result, nil
}

var _gpuSkuListNamespace = "special-resource-list"

// GetSkuConfigMaps fetch the config maps from the cluster
func (c *Client) GetSkuConfigMaps(ctx context.Context, cluster *v2beta1pb.Cluster) (
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
