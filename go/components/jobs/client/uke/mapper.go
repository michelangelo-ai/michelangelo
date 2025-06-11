package uke

import (
	"fmt"

	"code.uber.internal/go/envfx.git"
	"code.uber.internal/rt/flipr-client-go.git/flipr"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/skus"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/metrics"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
)

// TODO Decide where to keep cluster specific mappers

// Mapper helps to map global to local crds and vice versa
type Mapper struct {
	env                     envfx.Context
	skuCache                skus.SkuConfigCache
	fliprClient             flipr.FliprClient
	fliprConstraintsBuilder types.FliprConstraintsBuilder
	metrics                 *metrics.ControllerMetrics
	mTLSHandler             types.MTLSHandler
	spiffeProvider          utils.SpiffeIDProvider
}

// MapperParams has Mapper params
type MapperParams struct {
	fx.In
	Env                     envfx.Context
	SkuCache                skus.SkuConfigCache
	FliprClient             flipr.FliprClient
	FliprConstraintsBuilder types.FliprConstraintsBuilder
	Scope                   tally.Scope
	MTLSHandler             types.MTLSHandler
	SpiffeProvider          utils.SpiffeIDProvider `optional:"true"`
}

// MapperResult has Mapper result
type MapperResult struct {
	fx.Out

	Mapper Mapper `name:"ukeMapper"`
}

const _mapperName = "ukeMapper"

// NewUkeMapper constructs the Mapper
func NewUkeMapper(p MapperParams) MapperResult {
	if p.SpiffeProvider == nil {
		p.SpiffeProvider = utils.NewDefaultSpiffeIDProvider()
	}
	return MapperResult{
		Mapper: Mapper{
			env:                     p.Env,
			skuCache:                p.SkuCache,
			fliprClient:             p.FliprClient,
			fliprConstraintsBuilder: p.FliprConstraintsBuilder,
			metrics:                 metrics.NewControllerMetrics(p.Scope, _mapperName),
			mTLSHandler:             p.MTLSHandler,
			spiffeProvider:          p.SpiffeProvider,
		},
	}
}

// MapGlobalToLocal maps the global crd to local crd
func (m Mapper) MapGlobalToLocal(obj runtime.Object, cluster *v2beta1pb.Cluster) (runtime.Object, error) {
	switch job := obj.(type) {
	case *v2beta1pb.RayJob:
		return m.mapRay(job, cluster)
	case *v2beta1pb.SparkJob:
		return m.mapSpark(job, cluster)
	}
	return nil, fmt.Errorf("the object must be a RayJob or a SparkJob, got:%T", obj)
}

// GetLocalName gets the namespaced name of the local crd. This is used by methods that only require the
// namespaced name to perform operations like Delete or Get APIs.
func (m Mapper) GetLocalName(obj runtime.Object) (namespace, name string) {
	switch job := obj.(type) {
	case *v2beta1pb.RayJob:
		namespace = RayLocalNamespace
		name = job.Name
	case *v2beta1pb.SparkJob:
		namespace = SparkLocalNamespace
		name = job.Name
	}
	return
}
