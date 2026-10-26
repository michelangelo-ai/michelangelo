//go:generate mamockgen FederatedClient
package client

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"go.uber.org/fx"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	k8sptr "k8s.io/utils/ptr"

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

// ErrSparkApplicationConflict is wrapped by the error CreateJob returns when the
// SparkApplication it has to create already exists in the member cluster but is
// not the job's own (sparkApplicationOwner): it belongs to a different job,
// since names there are not unique across projects, or it carries no
// ma/job-uid label at all, like one created by hand.
// It is deliberately not an AlreadyExists error, which callers read as "this
// job's own application is already there" -- the opposite of what happened.
// Test for it with errors.Is.
var ErrSparkApplicationConflict = errors.New("spark application name is already in use")

// errNoSparkClient is wrapped by the error every Spark arm returns when the
// compute cluster's client set carries no SparkApplication client.
var errNoSparkClient = errors.New("compute client set has no spark client")

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
	Mapper  matypes.Mapper `name:"k8sengineMapper"`
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

	// GetJobStatus gets the Ray or Spark job status from the specified cluster
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
	mapper          matypes.Mapper
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

// JobStatusNotFound is the JobStatus GetJobStatus reports when the compute
// cluster's API server says the KubeRay RayJob is gone. KubeRay never reports it.
const JobStatusNotFound = "Unknown"

// GetJobStatus retrieves the status of a Ray or Spark job from a remote cluster
func (c *Client) GetJobStatus(
	ctx context.Context,
	jobObject runtime.Object,
	kubeCluster *v2pb.Cluster,
) (*matypes.JobStatus, error) {
	switch job := jobObject.(type) {
	case *v2pb.RayJob:
		cs, err := c.factory.GetClientSetForCluster(kubeCluster)
		if err != nil {
			return nil, fmt.Errorf("get client for cluster err:%w", err)
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
			// Only the API server's own NotFound means the RayJob is gone. A proxy
			// or ingress in front of the compute cluster can answer 404 itself,
			// which client-go also reports as NotFound but flags as an unexpected
			// server response; reporting that as missing would fail a job that
			// may still be running.
			if apierrors.IsNotFound(err) && !apierrors.IsUnexpectedServerError(err) {
				return &matypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: JobStatusNotFound,
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
		cs, err := c.factory.GetClientSetForCluster(kubeCluster)
		if err != nil {
			return nil, fmt.Errorf("get client for cluster err:%w", err)
		}
		sparkClient, err := requireSparkClient(cs, kubeCluster)
		if err != nil {
			return nil, err
		}

		localNamespace, localName := c.mapper.GetLocalName(jobObject)

		// Get the SparkApplication resource
		sparkApp := &sparkv1beta2.SparkApplication{}
		err = c.helper.GetResource(
			ctx,
			sparkClient,
			constants.KubeSparkResource,
			localNamespace,
			localName,
			sparkApp,
		)
		if err != nil {
			// Deliberately unlike the Ray arm above: a missing
			// SparkApplication is surfaced as an error, not turned into a
			// synthetic terminal status. What absence means is the caller's to
			// decide: for a launched job the SparkJob controller records
			// Succeeded=FALSE with reason SparkApplicationDeleted. Wrapped with
			// %w so the caller can still recover the API status: NotFound, which
			// is also how the ownership rule below reports an application that
			// is not this job's, and whether the 404 came with a Kubernetes
			// Status at all (sparkApplicationAbsent).
			return nil, fmt.Errorf("get spark application %q status: %w", localName, err)
		}

		// The application under this name may not be this job's: names in the
		// member cluster are shared across projects, so a job whose own
		// application is gone can find a same-named one left by another job, or
		// one created outside Michelangelo. Its status is not this job's, and the
		// caller reads NotFound as "this job's application is gone", so it has
		// to look absent. The rule is sparkApplicationOwner's, as in CreateJob
		// and DeleteJob: an application without the job's UID in its ma/job-uid
		// label is not the job's own.
		if owner, own := sparkApplicationOwner(sparkApp, string(job.GetUID())); !own {
			c.logger.Info("Reporting spark application as not found: it is not this job's",
				zap.String("namespace", localNamespace),
				zap.String("name", localName),
				zap.String("cluster", kubeCluster.GetName()),
				zap.String("ownerJobUID", owner))
			return nil, fmt.Errorf("get spark application %q status: %w", localName,
				apierrors.NewNotFound(sparkv1beta2.Resource(constants.KubeSparkResource), localName))
		}

		jobStatus, err := c.mapper.MapLocalJobStatusToGlobal(sparkApp)
		if err != nil {
			return nil, fmt.Errorf("map job status: %w", err)
		}
		return jobStatus, nil
	}
	return nil, fmt.Errorf("unsupported job type")
}

// DeleteJob deletes a Ray or Spark job from the specified cluster
func (c *Client) DeleteJob(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2pb.Cluster,
) error {
	switch job := jobObject.(type) {
	case *v2pb.RayJob:
		cs, err := c.factory.GetClientSetForCluster(cluster)
		if err != nil {
			return err
		}
		localNamespace, localName := c.mapper.GetLocalName(jobObject)
		return c.helper.DeleteResource(ctx, cs.Ray, constants.KubeRayJobResource, localNamespace, localName, metav1.DeleteOptions{})
	case *v2pb.SparkJob:
		cs, err := c.factory.GetClientSetForCluster(cluster)
		if err != nil {
			return err
		}
		sparkClient, err := requireSparkClient(cs, cluster)
		if err != nil {
			return err
		}
		// This is the only thing that stops a running projected SparkApplication:
		// unlike a RayJob, it has no owning CR in the member cluster to
		// cascade-GC it, so a no-op here would leak a running driver. A
		// finished application is reaped by its TTL instead.
		return c.deleteSparkApplication(ctx, sparkClient, job, cluster)
	}
	return fmt.Errorf("unrecognized job type")
}

// CreateJob sends request to Compute to create a Ray or Spark job on the cluster.
// A Ray job is created as a RayJob owned by its (already created) RayCluster; a
// Spark job is projected as a self-contained SparkApplication.
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
			return fmt.Errorf("get client for cluster err:%w", err)
		}

		kubeRayJob, err := c.mapper.MapGlobalJobToLocal(jobObject, jobClusterObject, kubeCluster)
		if err != nil {
			return fmt.Errorf("map global to local err:%w", err)
		}

		// Fetch the remote RayCluster to get its UID for ownerReference.
		// When the RayCluster is later deleted, K8s GC cascade-deletes all owned RayJobs.
		clusterNamespace, clusterName := c.mapper.GetLocalName(jobClusterObject)
		remoteCluster := &rayv1.RayCluster{}
		err = c.helper.GetResource(ctx, cs.Ray, constants.KubeRayResource, clusterNamespace, clusterName, remoteCluster)
		if err != nil {
			return fmt.Errorf("get remote ray cluster for owner ref: %w", err)
		}
		kubeRayJobTyped := kubeRayJob.(*rayv1.RayJob)
		kubeRayJobTyped.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "ray.io/v1",
			Kind:       "RayCluster",
			Name:       remoteCluster.Name,
			UID:        remoteCluster.UID,
			Controller: k8sptr.To(true),
		}}

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
		cs, err := c.factory.GetClientSetForCluster(kubeCluster)
		if err != nil {
			return fmt.Errorf("get client for cluster err:%w", err)
		}
		sparkClient, err := requireSparkClient(cs, kubeCluster)
		if err != nil {
			return err
		}

		sparkApp, err := c.mapper.MapGlobalJobToLocal(jobObject, jobClusterObject, kubeCluster)
		if err != nil {
			return fmt.Errorf("map global to local err:%w", err)
		}

		// No ownerReference block here, unlike the Ray arm above. A
		// SparkApplication is self-contained: there is no parent CR in the
		// member cluster to own it, and jobClusterObject is nil for Spark, so
		// the Ray arm's parent lookup would have nothing to look up
		// (GetLocalName of nil is empty, and the GET fails with "resource name
		// may not be empty"). Nothing cascades from the global side either, so
		// DeleteJob is what stops a running projected SparkApplication; a
		// finished one is reaped by its TTL.
		if err := c.createSparkApplication(ctx, sparkClient, job, sparkApp, kubeCluster); err != nil {
			return err
		}
		localNamespace, _ := c.mapper.GetLocalName(jobObject)
		c.logger.Info("Successfully created spark application in compute cluster", zap.String(constants.Job, job.GetName()), zap.String("namespace", localNamespace), zap.String("cluster", kubeCluster.GetName()))

		return nil
	}
	return fmt.Errorf("unsupported job type: %T", jobObject)
}

// requireSparkClient returns the SparkApplication client of a compute cluster's
// client set, or an error when the set has none. The production factory always
// fills it in, but a nil interface would otherwise panic at the first request
// -- or, for a watcher, in a reflector goroutine nothing recovers.
func requireSparkClient(cs *compute.ClientSet, cluster *v2pb.Cluster) (rest.Interface, error) {
	if cs == nil || cs.Spark == nil {
		return nil, fmt.Errorf("cluster %q: %w", cluster.GetName(), errNoSparkClient)
	}
	return cs.Spark, nil
}

// sparkApplicationOwner returns app's ma/job-uid label, the UID of the job that
// owns it, and whether that is the job with UID jobUID. An application is the
// job's own only when the label equals the job's UID and that UID is not empty.
// Anything else is not the job's own: an application labelled for another job,
// and one without the label, or with an empty one, which Michelangelo did not
// create for a job -- the mapper labels every application it projects from a
// job that has a UID. Such an application was created by hand or by another
// tool. The standalone Spark controller is one: it left unlabelled applications
// under their job's name in the job's own namespace on the control plane's
// cluster, so one of them holds a name here only for a job in the `default`
// namespace, when that cluster is also registered as a member. A job without a
// UID owns nothing.
func sparkApplicationOwner(app *sparkv1beta2.SparkApplication, jobUID string) (owner string, own bool) {
	owner = app.GetLabels()[constants.JobUIDLabelKey]
	return owner, jobUID != "" && owner == jobUID
}

// sparkApplicationAbsent reports whether err, from a read of a SparkApplication,
// shows that the application does not exist: a 404 the member cluster's API
// server answered with a Kubernetes Status. A 404 whose body is not a Status
// also satisfies apierrors.IsNotFound, and apierrors.IsUnexpectedServerError
// as well. It does not come from the SparkApplication API: a proxy or an
// ingress in front of the API server sends one when it misroutes the request,
// and the API server itself sends one when it does not serve that API at all,
// because the Spark Operator's CRD is gone and every application with it. The
// two cannot be told apart, so a read does not take such a 404 as proof that
// the application is gone: it is returned as the error it is (the SparkJob
// controller retries it as a transient failure).
func sparkApplicationAbsent(err error) bool {
	return apierrors.IsNotFound(err) && !apierrors.IsUnexpectedServerError(err)
}

// sparkApplicationAPINotServed is the error CreateJob returns when the POST that
// creates a SparkApplication is answered with a 404, err.
//
// The POST goes to the collection in the member cluster's `default` namespace,
// which cannot be deleted, so a 404 there means the cluster does not serve the
// SparkApplication API: the Spark Operator, whose CRD defines it, is not
// installed. That holds whether or not the 404 carries a Kubernetes Status. A
// kube-apiserver answers a path no API it serves covers with a plain-text "404
// page not found", which client-go reports as a NotFound that is also
// apierrors.IsUnexpectedServerError. A read does not take that kind as proof
// that an application is gone, because a proxy's 404 looks the same
// (sparkApplicationAbsent), but on this POST it is the usual answer. A proxy
// that misroutes the POST fails it the same way, and the user can resubmit.
//
// err is formatted with %v, not wrapped, on purpose: with no API status left
// in the chain, the SparkJob controller records this message on the job rather
// than the API server's "the server could not find the requested resource",
// and fails the job rather than retry a creation that cannot succeed until
// someone installs the operator.
func sparkApplicationAPINotServed(namespace, clusterName string, err error) error {
	return fmt.Errorf("cluster %q does not serve %s in namespace %q; is the Spark Operator installed? (%v)",
		clusterName, sparkv1beta2.Resource(constants.KubeSparkResource), namespace, err)
}

// sparkApplicationConflict is the error CreateJob returns when the application
// holding the job's name is not the job's own; owner is that application's
// ma/job-uid label.
func sparkApplicationConflict(namespace, name, clusterName, jobUID, owner string) error {
	var whose string
	switch {
	case owner != "":
		whose = fmt.Sprintf("belongs to job UID %q", owner)
	case jobUID != "":
		whose = fmt.Sprintf("carries no %s label, so Michelangelo did not create it for this job", constants.JobUIDLabelKey)
	default:
		whose = "cannot be this job's: the job has no UID"
	}
	return fmt.Errorf("%w: %s/%s on cluster %q %s", ErrSparkApplicationConflict, namespace, name, clusterName, whose)
}

// createSparkApplication creates the SparkApplication app and, when the name is
// already taken, works out whose application holds it.
//
// Every application is named after its job in one shared namespace, so a 409
// can mean two things. It may be this job's own application, left by an
// earlier attempt whose result was lost or whose Launched write failed, and the
// AlreadyExists error is then returned unchanged, because the caller treats it
// as launched. Or it may be an application that is not this job's
// (sparkApplicationOwner) and happens to hold the name: another job's, or one
// without the ma/job-uid label. The error then wraps
// ErrSparkApplicationConflict, which is not AlreadyExists: the newer job fails
// rather than adopt that application and, with it, an outcome that is not its
// own.
//
// A 404 on the create itself means the cluster does not serve the
// SparkApplication API (sparkApplicationAPINotServed).
func (c *Client) createSparkApplication(
	ctx context.Context,
	sparkClient rest.Interface,
	job *v2pb.SparkJob,
	app runtime.Object,
	cluster *v2pb.Cluster,
) error {
	// always use mapper's local namespace/name to avoid inconsistencies
	namespace, name := c.mapper.GetLocalName(job)
	jobUID := string(job.GetUID())

	// The second attempt happens only when the application that blocked the
	// first was deleted before it could be read, which only a 404 the API
	// server answered with a Status shows (sparkApplicationAbsent). Any other
	// failed read is returned.
	for attempt := 0; ; attempt++ {
		err := c.helper.CreateResource(ctx, sparkClient, app, constants.KubeSparkResource, namespace)
		if err == nil {
			return nil
		}
		if apierrors.IsNotFound(err) {
			return sparkApplicationAPINotServed(namespace, cluster.GetName(), err)
		}
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create spark application err:%w", err)
		}

		existing := &sparkv1beta2.SparkApplication{}
		getErr := c.helper.GetResource(ctx, sparkClient, constants.KubeSparkResource, namespace, name, existing)
		switch {
		case getErr == nil:
			if owner, own := sparkApplicationOwner(existing, jobUID); !own {
				return sparkApplicationConflict(namespace, name, cluster.GetName(), jobUID, owner)
			}
			return fmt.Errorf("create spark application err:%w", err)
		case sparkApplicationAbsent(getErr) && attempt == 0:
			continue
		default:
			return fmt.Errorf("get spark application %s/%s after create conflict: %w", namespace, name, getErr)
		}
	}
}

// deleteSparkApplication deletes the job's own SparkApplication and nothing
// else.
//
// Names in the member cluster are shared across projects, so the delete is
// guarded twice. The application is read first, and one that is not the job's
// own (sparkApplicationOwner) is left alone, whether it belongs to another job
// or carries no ma/job-uid label. The delete itself then carries the UID of the
// object that was read as a precondition, so an application that was replaced
// between the read and the delete is refused by the API server instead of
// removed. Both outcomes return nil: this job's own application is gone, which
// is all the caller wanted. An application that does not exist is returned as
// NotFound, which callers treat as success. The read shows that only through
// a 404 the API server answered with a Status (sparkApplicationAbsent); any
// other failed read is returned wrapped, and nothing is deleted. The delete's
// own error is returned as it is, apart from the API server's refusal of the
// precondition.
func (c *Client) deleteSparkApplication(
	ctx context.Context,
	sparkClient rest.Interface,
	job *v2pb.SparkJob,
	cluster *v2pb.Cluster,
) error {
	namespace, name := c.mapper.GetLocalName(job)

	app := &sparkv1beta2.SparkApplication{}
	if err := c.helper.GetResource(ctx, sparkClient, constants.KubeSparkResource, namespace, name, app); err != nil {
		if sparkApplicationAbsent(err) {
			return err
		}
		return fmt.Errorf("get spark application %s/%s before delete: %w", namespace, name, err)
	}

	if owner, own := sparkApplicationOwner(app, string(job.GetUID())); !own {
		c.logger.Info("Not deleting spark application: it is not this job's",
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.String("cluster", cluster.GetName()),
			zap.String("ownerJobUID", owner))
		return nil
	}

	err := c.helper.DeleteResource(ctx, sparkClient, constants.KubeSparkResource, namespace, name,
		metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &app.UID}})
	if apierrors.IsConflict(err) && !apierrors.IsUnexpectedServerError(err) {
		// The API server refuses a delete whose UID precondition does not match
		// with a Conflict: the application read above was deleted and another
		// one created under the same name since, so what is there now is not
		// this job's. A 409 whose body is not a Kubernetes Status reads as a
		// Conflict too, but the API server did not send it: a proxy in front of
		// it did, and the application may well still be there, so that one is
		// returned like any other failed delete.
		c.logger.Info("Not deleting spark application: it was replaced after it was read",
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.String("cluster", cluster.GetName()),
			zap.String("readUID", string(app.UID)))
		return nil
	}
	return err
}

func (c *Client) CreateJobCluster(ctx context.Context, jobClusterObject runtime.Object, kubeCluster *v2pb.Cluster) error {
	cs, err := c.factory.GetClientSetForCluster(kubeCluster)
	if err != nil {
		return fmt.Errorf("get client for cluster err:%w", err)
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
			return fmt.Errorf("get client for cluster err:%w", err)
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
			return nil, fmt.Errorf("get client for cluster err:%w", err)
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

// Watcher creates the requested resource watchers (pods, configmaps, Ray and
// Spark objects) on the compute cluster's API server
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
		case constants.KubeRayResource, constants.KubeRayJobResource:
			wp.Client = clientSet.Ray
		case corev1.ResourcePods.String(), corev1.ResourceConfigMaps.String():
			wp.Client = clientSet.CoreV1
		case constants.KubeSparkResource:
			// An informer built on a nil client panics in the reflector's own
			// goroutine, which nothing recovers, so refuse it here.
			sparkClient, err := requireSparkClient(clientSet, cluster)
			if err != nil {
				return nil, err
			}
			wp.Client = sparkClient
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
