package utils

import (
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// KnownResources are the compute resources which jobs can use.
var KnownResources = []v1.ResourceName{
	v1.ResourceCPU,
	constants.ResourceNvidiaGPU,
	v1.ResourceMemory,
	v1.ResourceEphemeralStorage,
}

// ScaleKnownResources multiplies the resource requirement with the
// scaleFactor.
//
// It's used to calculate the total resource
// requirements for all the workers combined. This can
// potentially be a util method if other components have a
// similar requirement.
func ScaleKnownResources(
	list v1.ResourceList,
	scaleFactor int64) (v1.ResourceList, error) {
	rList := make(v1.ResourceList)

	for _, res := range KnownResources {
		qt := list[res]
		rInt, ok := qt.AsInt64()
		if !ok {
			return v1.ResourceList{}, fmt.Errorf(
				"could not parse %s as int64", res)
		}

		rList[res] = *resource.NewQuantity(
			rInt*scaleFactor, resource.DecimalSI)
	}
	return rList, nil
}
