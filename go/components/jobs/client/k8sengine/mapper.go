package k8sengine

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"go.uber.org/config"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Mapper helps to map global to local crds and vice versa
type Mapper struct {
	LogPersistence LogPersistenceConfig
	logURLTemplate *template.Template
	// rayLocalNamespace is the configured compute-cluster namespace for Ray
	// workloads. Read it through localNamespace, which applies the default.
	rayLocalNamespace string
}

// MapperResult has Mapper result
type MapperResult struct {
	fx.Out

	Mapper types.Mapper `name:"k8sengineMapper"`
}

const _mapperName = "k8sengineMapper"

const logPersistenceConfigKey = "jobs.k8sengine.mapper.logPersistence"

const rayLocalNamespaceConfigKey = "jobs.k8sengine.mapper.rayLocalNamespace"

// RayLocalNamespaceConfig is the compute-cluster namespace that Ray workloads
// (RayClusters, RayJobs and the per-job Secrets and ConfigMaps created next to
// them) are created in. It is the same for every compute cluster. Empty means
// RayLocalNamespace.
type RayLocalNamespaceConfig string

// NewLogPersistenceConfig loads LogPersistenceConfig from YAML config provider.
func NewLogPersistenceConfig(provider config.Provider) (LogPersistenceConfig, error) {
	conf := LogPersistenceConfig{}
	err := provider.Get(logPersistenceConfigKey).Populate(&conf)
	if err != nil {
		// Config is optional — return zero-value (disabled) if not present
		return LogPersistenceConfig{}, nil
	}
	return conf, nil
}

// NewRayLocalNamespaceConfig loads RayLocalNamespaceConfig from YAML config
// provider. Returns RayLocalNamespace when the key is unset or empty.
func NewRayLocalNamespaceConfig(provider config.Provider) (RayLocalNamespaceConfig, error) {
	var namespace string
	if err := provider.Get(rayLocalNamespaceConfigKey).Populate(&namespace); err != nil {
		return "", fmt.Errorf("load %s: %w", rayLocalNamespaceConfigKey, err)
	}
	if namespace == "" {
		namespace = RayLocalNamespace
	}
	return RayLocalNamespaceConfig(namespace), nil
}

// MapperParams are the inputs to NewMapper.
type MapperParams struct {
	fx.In

	LogPersistence    LogPersistenceConfig
	RayLocalNamespace RayLocalNamespaceConfig
}

// NewMapper constructs the Mapper. Panics if LogURLFormat is set but does not
// parse as a valid Go text/template, or if RayLocalNamespace is set but is not
// a valid DNS-1123 label — config errors should fail at startup.
func NewMapper(p MapperParams) MapperResult {
	namespace := string(p.RayLocalNamespace)
	if namespace != "" {
		if errs := validation.IsDNS1123Label(namespace); len(errs) > 0 {
			panic(fmt.Sprintf("invalid Ray local namespace %q: %s", namespace, strings.Join(errs, "; ")))
		}
	}
	var tmpl *template.Template
	if p.LogPersistence.Enabled && p.LogPersistence.LogURLFormat != "" {
		tmpl = template.Must(template.New("logURL").Parse(p.LogPersistence.LogURLFormat))
	}
	return MapperResult{
		Mapper: Mapper{
			LogPersistence:    p.LogPersistence,
			logURLTemplate:    tmpl,
			rayLocalNamespace: namespace,
		},
	}
}

// localNamespace returns the compute-cluster namespace for Ray workloads: the
// configured one, or RayLocalNamespace when none is set (e.g. a zero Mapper).
func (m Mapper) localNamespace() string {
	if m.rayLocalNamespace == "" {
		return RayLocalNamespace
	}
	return m.rayLocalNamespace
}

// MapGlobalJobToLocal maps the global job object to local job object
func (m Mapper) MapGlobalJobToLocal(jobObject runtime.Object, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error) {
	if jobObject == nil {
		return nil, fmt.Errorf("jobObject cannot be nil")
	}

	switch obj := jobObject.(type) {
	case *v2pb.RayJob:
		localJob, err := m.mapRay(obj, jobClusterObject, cluster)
		if err != nil {
			return nil, fmt.Errorf("map ray job: %w", err)
		}
		return localJob, nil
	case *v2pb.SparkJob:
		return nil, fmt.Errorf("spark job mapping not implemented: %T", jobObject)
	default:
		return nil, fmt.Errorf("unsupported job object type: %T", jobObject)
	}
}

// MapGlobalJobClusterToLocal maps the global cluster object to local cluster object
func (m Mapper) MapGlobalJobClusterToLocal(jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error) {
	if jobClusterObject == nil {
		return nil, fmt.Errorf("jobClusterObject cannot be nil")
	}

	switch obj := jobClusterObject.(type) {
	case *v2pb.RayCluster:
		localCluster, err := m.mapRayCluster(obj)
		if err != nil {
			return nil, fmt.Errorf("map ray cluster: %w", err)
		}
		return localCluster, nil
	default:
		return nil, fmt.Errorf("unsupported cluster object type: %T", jobClusterObject)
	}
}

// GetLocalName gets the namespaced name of the local crd. This is used by methods that only require the
// namespaced name to perform operations like Delete or Get APIs.
func (m Mapper) GetLocalName(obj runtime.Object) (namespace, name string) {
	switch job := obj.(type) {
	case *v2pb.RayJob:
		namespace = m.localNamespace()
		name = job.Name
	case *v2pb.RayCluster:
		namespace = m.localNamespace()
		name = job.Name
	case *v2pb.SparkJob:
		// Not implemented yet; return empty
		return "", ""
	}
	return
}

// MapLocalClusterStatusToGlobal converts a local (Kubernetes) cluster status object
// to the global Michelangelo ClusterStatus representation.
func (m Mapper) MapLocalClusterStatusToGlobal(localClusterObject runtime.Object) (*types.JobClusterStatus, error) {
	if localClusterObject == nil {
		return nil, fmt.Errorf("localClusterObject cannot be nil")
	}

	switch obj := localClusterObject.(type) {
	case *rayv1.RayCluster:
		v2Status := convertRayV1ClusterStatusToV2(obj)
		v2Status.LogUrl = m.buildLogURL(obj.GetName())
		reason := deriveReasonFromConditions(obj)
		if reason == "" {
			reason = obj.Status.Reason
		}
		return &types.JobClusterStatus{
			Ray:    v2Status,
			Reason: reason,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported cluster object type: %T", localClusterObject)
	}
}

// buildLogURL renders LogPersistenceConfig.LogURLFormat against the per-cluster
// values (Bucket, PathPrefix, ClusterName, RayLocalNamespace). Returns "" when
// log persistence is disabled or no format is configured. RayLocalNamespace is
// the configured compute-cluster Ray namespace (where pods actually run), not
// the v2 RayCluster CR's control-plane namespace.
func (m Mapper) buildLogURL(clusterName string) string {
	if m.logURLTemplate == nil {
		return ""
	}
	var buf bytes.Buffer
	err := m.logURLTemplate.Execute(&buf, struct {
		Bucket            string
		PathPrefix        string
		ClusterName       string
		RayLocalNamespace string
	}{
		Bucket:            m.LogPersistence.Bucket,
		PathPrefix:        m.LogPersistence.PathPrefix,
		ClusterName:       clusterName,
		RayLocalNamespace: m.localNamespace(),
	})
	if err != nil {
		return ""
	}
	return buf.String()
}

// MapLocalJobStatusToGlobal converts a local (Kubernetes) job status object
// to the global Michelangelo JobStatus representation.
func (m Mapper) MapLocalJobStatusToGlobal(localJobObject runtime.Object) (*types.JobStatus, error) {
	if localJobObject == nil {
		return nil, fmt.Errorf("localJobObject cannot be nil")
	}

	switch obj := localJobObject.(type) {
	case *rayv1.RayJob:
		v2Status := convertRayV1JobStatusToGlobal(obj)
		return &types.JobStatus{
			Ray: v2Status,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported job object type: %T", localJobObject)
	}
}
