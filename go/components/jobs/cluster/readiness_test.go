package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestReconciler_GetClusterReadiness(t *testing.T) {
	notReady := &v2pb.ClusterStatus{StatusConditions: []*apipb.Condition{
		{Type: constants.ClusterReady, Status: apipb.CONDITION_STATUS_FALSE},
	}}
	r := &Reconciler{clusterDataMap: &clusterMap{}}
	register := func(name string, status *v2pb.ClusterStatus) {
		r.clusterDataMap.add(name, &Data{
			cachedObj:     &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name}},
			clusterStatus: status,
		})
	}
	register("ready", newReadyStatus())
	register("not-ready", notReady)
	// Registered but never probed by this process: clusterStatus is nil.
	register("unprobed", nil)

	tests := []struct {
		name string
		want Readiness
	}{
		{name: "ready", want: ReadinessReady},
		{name: "not-ready", want: ReadinessNotReady},
		{name: "unprobed", want: ReadinessUnknown},
		{name: "unregistered", want: ReadinessNotRegistered},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, r.GetClusterReadiness(tt.name))
		})
	}

	// The readiness follows the latest probe.
	r.clusterDataMap.get("unprobed").SetClusterStatus(notReady)
	assert.Equal(t, ReadinessNotReady, r.GetClusterReadiness("unprobed"))
	r.clusterDataMap.get("unprobed").SetClusterStatus(newReadyStatus())
	assert.Equal(t, ReadinessReady, r.GetClusterReadiness("unprobed"))

	assert.Same(t, r, NewClusterReadiness(r))
}

func TestReadiness_String(t *testing.T) {
	assert.Equal(t, "unknown", ReadinessUnknown.String())
	assert.Equal(t, "ready", ReadinessReady.String())
	assert.Equal(t, "not ready", ReadinessNotReady.String())
	assert.Equal(t, "not registered", ReadinessNotRegistered.String())
	assert.Equal(t, "invalid", Readiness(99).String())
}
