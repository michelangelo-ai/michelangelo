package v2

import "k8s.io/apimachinery/pkg/runtime"

// CrdObjects contains the list of CRD objects that the ingester should watch.
// Keep this list in sync with the CRDs installed in the cluster.
var CrdObjects = []runtime.Object{
	&Model{},
	&ModelFamily{},
	&Pipeline{},
	&PipelineRun{},
	&Deployment{},
	&InferenceServer{},
	&Project{},
	&Revision{},
	&Cluster{},
	&RayCluster{},
	&RayJob{},
	&SparkJob{},
	&TriggerRun{},
}
