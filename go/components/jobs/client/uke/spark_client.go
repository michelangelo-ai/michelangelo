package uke

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"code.uber.internal/go/envfx.git"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/secrets"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/metrics"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/utils/cloud"
	drogongateway "code.uber.internal/uberai/michelangelo/controllermgr/pkg/gateways/drogon"
	sharedconst "code.uber.internal/uberai/michelangelo/shared/constants"
	"code.uber.internal/uberai/michelangelo/shared/gateways/hadooptokenservice"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	v2beta1pb "michelangelo/api/v2beta1"
)

const (
	_clientName                    = "sparkclient"
	_michelangeloServiceName       = "michelangelo"
	_defaultTokenExpirationTimeout = 21 * time.Hour
)

// SparkClientParams is the input to the constructor
type SparkClientParams struct {
	fx.In
	DrogonGateway drogongateway.Gateway
	Secrets       secrets.Provider
	MTLSHandler   types.MTLSHandler

	// Fx provided dependencies
	MetricsScope tally.Scope
	Env          envfx.Context
}

// NewSparkClient is the constructor
func NewSparkClient(p SparkClientParams) *SparkClient {
	return &SparkClient{
		drogongateway:   p.DrogonGateway,
		secretsProvider: p.Secrets,
		tokenCache:      make(map[string]cachedTokenEntry),
		mTLSHandler:     p.MTLSHandler,
		metrics:         metrics.NewControllerMetrics(p.MetricsScope, _clientName),
		env:             p.Env,
	}
}

// SparkClient is the client for spark jobs
type SparkClient struct {
	drogongateway   drogongateway.Gateway
	secretsProvider secrets.Provider
	tokenCache      map[string]cachedTokenEntry // map of token cache used to get job status. Key is region.
	mTLSHandler     types.MTLSHandler
	metrics         *metrics.ControllerMetrics
	env             envfx.Context
}

type cachedTokenEntry struct {
	token             hadooptokenservice.Token
	creationTimeStamp time.Time
}

// SubmitJob submits a spark job
// SparkJob is the CRD of the spark job.
// Cluster is the cluster where the job is submitted to. It can be a peloton cluster or a k8s cluster.
func (c *SparkClient) SubmitJob(ctx context.Context, sparkJob *v2beta1pb.SparkJob, cluster *v2beta1pb.Cluster) error {
	hdfsToken, err := c.getUserHDFSDelegationToken(ctx, sparkJob, cluster)
	if err != nil {
		return err
	}
	submitRequest, err := c.getDrogonSubmitRequestFromSparkJob(sparkJob, cluster)
	if err != nil {
		return err
	}
	submitRequest.RuntimeParameters.AuthToken = hdfsToken.Token
	submitResp, err := c.drogongateway.SubmitJob(ctx, &submitRequest)
	if err != nil {
		return err
	}
	sparkJob.Status.ApplicationId = strconv.FormatInt(submitResp.ID, 10)
	sparkJob.Status.JobUrl = submitResp.AppInfo.DriverLogURL
	return nil
}

// GetJobStatus gets the status of a spark job
// SparkJob is the CRD of the spark job.
// Cluster is the cluster where the job is submitted to. It can be a peloton cluster or a k8s cluster.
func (c *SparkClient) GetJobStatus(ctx context.Context, sparkJob *v2beta1pb.SparkJob, cluster *v2beta1pb.Cluster) (constants.SparkJobStatus, error) {
	hdfsToken, err := c.getServiceHDFSDelegationToken(ctx, sparkJob, cluster)
	if err != nil {
		return "", fmt.Errorf("SparkClient.GetJobStatus: error getting hdfs delegation token, %w", err)
	}
	appID, err := strconv.ParseInt(sparkJob.Status.GetApplicationId(), 10, 64)
	if err != nil {
		c.metrics.IncJobFailure(constants.FailureReasonErrorParsingApplicationID)
		return "", fmt.Errorf("SparkClient.GetJobStatus: error parsing application id, %w", err)
	}
	drogonClusterName, err := getDrogonClusterName(cluster)
	if err != nil {
		c.metrics.IncJobFailure(constants.FailureReasonErrorFetchingDrogonClusterName)
		return "", fmt.Errorf("SparkClient.GetJobStatus: error getting drogon cluster name, %w", err)
	}
	statusRequest := drogongateway.JobStatusRequest{
		ID: appID,
		RuntimeParameters: drogongateway.RuntimeParameters{
			AuthToken:         hdfsToken.Token,
			Cluster:           drogonClusterName,
			UberRegionRouting: getUberRegionRouting(cluster),
		},
	}
	statusResp, err := c.drogongateway.GetJobStatus(ctx, &statusRequest)
	if err != nil {
		if !IsJobKilledByUI(err.Error()) {
			c.metrics.IncJobFailure(constants.FailureReasonErrorFetchingDrogonJobStatus)
		}
		return "", fmt.Errorf("SparkClient.GetJobStatus: error getting job status from drogon, %w", err)
	}
	sparkJob.Status.JobUrl = statusResp.AppInfo.DriverLogURL
	switch statusResp.State {
	case drogongateway.Starting:
		return constants.JobStatusPending, nil
	case drogongateway.Running:
		return constants.JobStatusRunning, nil
	case drogongateway.Dead, drogongateway.Error:
		return constants.JobStatusFailed, nil
	case drogongateway.Success:
		return constants.JobStatusSucceeded, nil
	default:
		return constants.JobStatusPending, nil
	}
}

// CancelJob cancels a spark job
// SparkJob is the CRD of the spark job.
// Cluster is the cluster where the job is submitted to. It can be a peloton cluster or a k8s cluster.
func (c *SparkClient) CancelJob(ctx context.Context, sparkJob *v2beta1pb.SparkJob, cluster *v2beta1pb.Cluster) error {
	hdfsToken, err := c.getUserHDFSDelegationToken(ctx, sparkJob, cluster)
	if err != nil {
		return err
	}
	appID, err := strconv.ParseInt(sparkJob.Status.GetApplicationId(), 10, 64)
	if err != nil {
		return err
	}
	drogonClusterName, err := getDrogonClusterName(cluster)
	if err != nil {
		return err
	}
	cancelRequest := drogongateway.CancelJobRequest{
		ID: appID,
		RuntimeParameters: drogongateway.RuntimeParameters{
			AuthToken:         hdfsToken.Token,
			Cluster:           drogonClusterName,
			UberRegionRouting: getUberRegionRouting(cluster),
		},
	}
	err = c.drogongateway.CancelJob(ctx, &cancelRequest)
	return err
}

// getUserHDFSDelegationToken gets the HDFS delegation token for the current user
func (c *SparkClient) getUserHDFSDelegationToken(ctx context.Context, sparkJob *v2beta1pb.SparkJob, cluster *v2beta1pb.Cluster) (hadooptokenservice.Token, error) {
	return c.secretsProvider.GetAccessTokenForDrogon(ctx, sparkJob, cluster)
}

// getServiceHDFSDelegationToken gets the HDFS delegation token for Michelangelo
func (c *SparkClient) getServiceHDFSDelegationToken(ctx context.Context, sparkJob *v2beta1pb.SparkJob, cluster *v2beta1pb.Cluster) (hadooptokenservice.Token, error) {
	// If token is cached and valid, return the cached token.
	region := cluster.Spec.Region
	if cachedToken, ok := c.tokenCache[region]; ok {
		if time.Since(cachedToken.creationTimeStamp) < _defaultTokenExpirationTimeout {
			return cachedToken.token, nil
		}
	}
	// If token is not cached or expired, get a new token.
	// The backend token provider needs two information:
	// 1. the user name. In this case, it is the service account name, michelangelo.
	// 2. the region. In this case, it is stored in the cluster spec.

	// To pass the service account name to the backend token provider, we use an internal temporary
	// sparkJob whose user is the service account name.
	tmpSparkJob := sparkJob.DeepCopy()
	tmpSparkJob.Spec.User.Name = _michelangeloServiceName
	token, err := c.secretsProvider.GetAccessTokenForDrogon(ctx, tmpSparkJob, cluster)
	if err != nil {
		c.metrics.IncJobFailure(constants.FailureReasonErrorFetchingHDFSDelegationToken)
		return hadooptokenservice.Token{}, err
	}
	c.tokenCache[cluster.Spec.Region] = cachedTokenEntry{
		token:             token,
		creationTimeStamp: time.Now(),
	}
	return token, nil
}

// getDrogonSubmitRequestFromSparkJob converts a SparkJob to a Drogon SubmitJobRequest
func (c *SparkClient) getDrogonSubmitRequestFromSparkJob(sparkJob *v2beta1pb.SparkJob, cluster *v2beta1pb.Cluster) (drogongateway.SubmitJobRequest, error) {
	proxyUser := sparkJob.Spec.User.Name
	if len(sparkJob.Spec.User.ProxyUser) > 0 {
		proxyUser = sparkJob.Spec.User.ProxyUser
	}
	if cluster.Spec.GetKubernetes() != nil {
		return drogongateway.SubmitJobRequest{}, fmt.Errorf("k8s cluster are not implemented yet")
	}
	resourcePool := sparkJob.Status.GetAssignment().GetResourcePool()
	if resourcePool == "" {
		return drogongateway.SubmitJobRequest{}, fmt.Errorf("resource pool is not set")
	}

	drogonJobDefinition := drogongateway.JobDefinition{
		File:           sparkJob.Spec.MainApplicationFile,
		Args:           sparkJob.Spec.MainArgs,
		Conf:           sparkJob.Spec.SparkConf,
		Jars:           sparkJob.Spec.Deps.Jars,
		PyFiles:        sparkJob.Spec.Deps.PyFiles,
		Files:          sparkJob.Spec.Deps.Files,
		ProxyUser:      proxyUser,
		SparkEnv:       sparkJob.Spec.SparkVersion,
		Name:           sparkJob.GetName(),
		ClassName:      sparkJob.Spec.MainClass,
		DriverMemory:   sparkJob.Spec.Driver.Pod.Resource.Memory,
		DriverCores:    int(sparkJob.Spec.Driver.Pod.Resource.Cpu),
		ExecutorMemory: sparkJob.Spec.Executor.Pod.Resource.Memory,
		ExecutorCores:  int(sparkJob.Spec.Executor.Pod.Resource.Cpu),
		NumExecutors:   int(sparkJob.Spec.Executor.Instances),
		Queue:          resourcePool,
	}

	drogonClusterName, err := getDrogonClusterName(cluster)
	if err != nil {
		return drogongateway.SubmitJobRequest{}, err
	}
	drogonRuntimeParameters := drogongateway.RuntimeParameters{
		Cluster:           drogonClusterName,
		UberRegionRouting: getUberRegionRouting(cluster), // Read the labels from the spark job to check if this needs to run in the cloud. eg dca60
	}
	// Container
	drogonJobDefinition.Conf["spark.peloton.driver.docker.image"] = sparkJob.Spec.Driver.Pod.Image
	drogonJobDefinition.Conf["spark.peloton.executor.docker.image"] = sparkJob.Spec.Executor.Pod.Image
	drogonJobDefinition.Conf["spark.mesos.executor.docker.image"] = sparkJob.Spec.Executor.Pod.Image
	drogonJobDefinition.Conf["spark.peloton.run-as-user"] = "true"
	drogonJobDefinition.Conf["spark.hadoop.fs.permissions.umask-mode"] = "006"
	enableMTLS, err := c.mTLSHandler.EnableMTLS(sparkJob.Namespace)
	if err == nil && enableMTLS {
		drogonJobDefinition.Conf["spark.drogon.k8s.mtls.enable"] = "true"
	}
	// Save region as env variable to be consumed by JC
	drogonJobDefinition.Conf["spark.peloton.driverEnv.REGION"] = cluster.Spec.Region
	for _, env := range sparkJob.Spec.Driver.Pod.Env {
		envKey := "spark.peloton.driverEnv." + env.Name
		drogonJobDefinition.Conf[envKey] = env.Value
	}
	for _, env := range sparkJob.Spec.Executor.Pod.Env {
		envKey := "spark.peloton.executorEnv." + env.Name
		drogonJobDefinition.Conf[envKey] = env.Value
	}
	drogonJobDefinition.Conf["spark.peloton.sla.preemptible"] = strconv.FormatBool(sparkJob.Spec.Scheduling.Preemptible)

	// add configs that add a labels to Spark jobs and pods so that they can be watched for error analysis.
	addDrogonLabelForSparkJobAndPods(drogonJobDefinition.Conf, constants.OwnerServiceLabelKey, constants.MAOwnerSparkLabelValue)
	addDrogonLabelForSparkJobAndPods(drogonJobDefinition.Conf, constants.ProjectNameLabelKey, sparkJob.Namespace)
	addDrogonLabelForSparkJobAndPods(drogonJobDefinition.Conf, constants.JobControlPlaneEnvKey, c.env.RuntimeEnvironment)
	// The name of the spark job in the K8s cluster is defined by Drogon. So we need to have the MA job name persisted as a label.
	addDrogonLabelForSparkJobAndPods(drogonJobDefinition.Conf, constants.JobNameLabelKey, sparkJob.Name)

	return drogongateway.SubmitJobRequest{
		JobDefinition:     drogonJobDefinition,
		RuntimeParameters: drogonRuntimeParameters,
	}, nil
}

// Any config that is added with this prefix is propagated as a label to the Spark application CRD as well
// as the pods that are spun up by the Spark operator.
var _sparkLabelPrefix = "spark.drogon.k8s.label."

func addDrogonLabelForSparkJobAndPods(drogonJobConf map[string]string, key, value string) {
	drogonJobConf[fmt.Sprintf("%s%s", _sparkLabelPrefix, key)] = value
}

// Returns the UberRegionRouting config if it is for cloud provider.
// This is used to set the proper runtime parameters for drogon.
func getUberRegionRouting(cluster *v2beta1pb.Cluster) string {
	return cloud.GetRegionRouting(cluster.Spec.Dc, cluster.Spec.Region)
}

var _kubernetesClusterIdentifier = "kubernetes"

func isKubernetesCluster(name string) bool {
	return strings.Contains(name, _kubernetesClusterIdentifier)
}

// getDrogonClusterName gets the drogon cluster name from the peloton/k8s cluster name
func getDrogonClusterName(cluster *v2beta1pb.Cluster) (string, error) {
	if cluster.Spec.GetKubernetes() != nil {
		return "", fmt.Errorf("k8s cluster are not implemented yet")
	}
	clusterName := cluster.GetName()
	if isKubernetesCluster(clusterName) {
		return "", nil
	}
	// We have validated the cluster name in the apihook to ensure that we can
	// get a valid drogon cluster name from the peloton cluster name.
	drogonClusterName := sharedconst.PelotonClusterToDrogonCluster[clusterName]
	return drogonClusterName, nil
}

// IsJobKilledByUI checks if the drogon job is killed by UI
func IsJobKilledByUI(errString string) bool {
	return strings.Contains(errString, "bad status code: 500") && strings.Contains(errString, "object not found")
}
