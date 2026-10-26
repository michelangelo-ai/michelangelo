package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var _versions = []string{"v1beta1", "v1beta2"}

// loadFixture reads a Kueue 0.19.9-shaped object from testdata/<version>/<name>.yaml.
func loadFixture(t *testing.T, version, name string) *unstructured.Unstructured {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", version, name+".yaml"))
	require.NoError(t, err)
	js, err := yaml.ToJSON(data)
	require.NoError(t, err)
	obj := &unstructured.Unstructured{}
	require.NoError(t, obj.UnmarshalJSON(js))
	return obj
}

// requireResources checks that got holds exactly the resources in want, each
// equal to the quantity want gives.
func requireResources(t *testing.T, want map[corev1.ResourceName]string, got corev1.ResourceList) {
	t.Helper()
	gotStrings := map[corev1.ResourceName]string{}
	for name, q := range got {
		gotStrings[name] = q.String()
	}
	require.Len(t, got, len(want), "got %v", gotStrings)
	for name, w := range want {
		q, ok := got[name]
		require.True(t, ok, "%s missing from %v", name, gotStrings)
		assert.Zero(t, q.Cmp(resource.MustParse(w)), "%s: got %s, want %s", name, q.String(), w)
	}
}

func TestParseClusterQueue(t *testing.T) {
	singleFlavorNominal := map[corev1.ResourceName]string{"cpu": "64", "memory": "256Gi", "nvidia.com/gpu": "8"}
	singleFlavorReserved := map[corev1.ResourceName]string{"cpu": "24", "memory": "96Gi", "nvidia.com/gpu": "2"}
	tests := []struct {
		fixture      string
		name         string
		cohort       string
		stopPolicy   string
		stopped      bool
		flavors      int
		covered      []corev1.ResourceName
		nominal      map[corev1.ResourceName]string
		reserved     map[corev1.ResourceName]string
		pending      int32
		wantScorable bool
	}{
		{
			fixture:      "clusterqueue-single-flavor",
			name:         "cluster-queue",
			stopPolicy:   "None",
			flavors:      1,
			covered:      []corev1.ResourceName{"cpu", "memory", "nvidia.com/gpu"},
			nominal:      singleFlavorNominal,
			reserved:     singleFlavorReserved,
			pending:      3,
			wantScorable: true,
		},
		{
			// quota and reservations summed over flavors and resource groups
			fixture:    "clusterqueue-multi-flavor",
			name:       "cluster-queue",
			stopPolicy: "None",
			flavors:    3,
			covered:    []corev1.ResourceName{"cpu", "memory", "nvidia.com/gpu"},
			nominal:    map[corev1.ResourceName]string{"cpu": "48", "memory": "192Gi", "nvidia.com/gpu": "4"},
			reserved:   map[corev1.ResourceName]string{"cpu": "12", "memory": "48Gi", "nvidia.com/gpu": "1"},
		},
		{
			// v1beta1 spec.cohort, v1beta2 spec.cohortName
			fixture:    "clusterqueue-cohort",
			name:       "research-queue",
			cohort:     "research",
			stopPolicy: "None",
			flavors:    1,
			covered:    []corev1.ResourceName{"cpu", "memory", "nvidia.com/gpu"},
			nominal:    singleFlavorNominal,
			reserved:   singleFlavorReserved,
			pending:    3,
		},
		{
			// stopping is not the view's call: still one flavor and no cohort
			fixture:      "clusterqueue-stopped",
			name:         "cluster-queue",
			stopPolicy:   "HoldAndDrain",
			stopped:      true,
			flavors:      1,
			covered:      []corev1.ResourceName{"cpu", "memory", "nvidia.com/gpu"},
			nominal:      singleFlavorNominal,
			reserved:     singleFlavorReserved,
			pending:      3,
			wantScorable: true,
		},
	}
	for _, version := range _versions {
		for _, tt := range tests {
			t.Run(version+"/"+tt.fixture, func(t *testing.T) {
				cq, err := parseClusterQueue(loadFixture(t, version, tt.fixture), version)
				require.NoError(t, err)
				assert.Equal(t, tt.name, cq.Name)
				assert.Equal(t, tt.cohort, cq.Cohort)
				assert.Equal(t, tt.stopPolicy, cq.StopPolicy)
				assert.Equal(t, tt.stopped, cq.Stopped())
				assert.Equal(t, tt.flavors, cq.FlavorCount)
				assert.Equal(t, tt.covered, cq.CoveredResources)
				requireResources(t, tt.nominal, cq.Nominal)
				requireResources(t, tt.reserved, cq.Reserved)
				assert.Equal(t, tt.pending, cq.PendingWorkloads)
				assert.Equal(t, tt.wantScorable, View{Synced: true, Exists: true, Active: true, ClusterQueue: cq}.Scorable())
			})
		}
	}
}

func TestParseClusterQueueReadsTheVersionsCohortField(t *testing.T) {
	// a v1beta1 object read at v1beta2 has no spec.cohortName, and the reverse
	cq, err := parseClusterQueue(loadFixture(t, "v1beta1", "clusterqueue-cohort"), "v1beta2")
	require.NoError(t, err)
	assert.Empty(t, cq.Cohort)
	cq, err = parseClusterQueue(loadFixture(t, "v1beta2", "clusterqueue-cohort"), "v1beta1")
	require.NoError(t, err)
	assert.Empty(t, cq.Cohort)
}

func TestParseClusterQueueErrors(t *testing.T) {
	tests := []struct {
		name    string
		version string
		mutate  func(obj map[string]interface{})
		wantErr string
	}{
		{
			name:    "unsupported API version",
			version: "v1alpha1",
			mutate:  func(map[string]interface{}) {},
			wantErr: `unsupported Kueue API version "v1alpha1"`,
		},
		{
			name:    "resource groups not a list",
			version: "v1beta2",
			mutate: func(obj map[string]interface{}) {
				obj["spec"].(map[string]interface{})["resourceGroups"] = "cpu"
			},
			wantErr: "spec.resourceGroups is string, not a list",
		},
		{
			name:    "malformed quota",
			version: "v1beta2",
			mutate: func(obj map[string]interface{}) {
				resources := firstFlavorResources(obj)
				resources[0].(map[string]interface{})["nominalQuota"] = "lots"
			},
			wantErr: "resource cpu",
		},
		{
			name:    "cohort not a string",
			version: "v1beta2",
			mutate: func(obj map[string]interface{}) {
				obj["spec"].(map[string]interface{})["cohortName"] = int64(1)
			},
			wantErr: "spec.cohortName is int64, not a string",
		},
		{
			name:    "pending workloads not an integer",
			version: "v1beta2",
			mutate: func(obj map[string]interface{}) {
				obj["status"].(map[string]interface{})["pendingWorkloads"] = "3"
			},
			wantErr: "status.pendingWorkloads is string, not an integer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := loadFixture(t, "v1beta2", "clusterqueue-single-flavor")
			tt.mutate(obj.Object)
			_, err := parseClusterQueue(obj, tt.version)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func firstFlavorResources(obj map[string]interface{}) []interface{} {
	groups := obj["spec"].(map[string]interface{})["resourceGroups"].([]interface{})
	flavors := groups[0].(map[string]interface{})["flavors"].([]interface{})
	return flavors[0].(map[string]interface{})["resources"].([]interface{})
}

func TestParseClusterQueueWithoutStatus(t *testing.T) {
	// a ClusterQueue Kueue has not reconciled yet
	obj := loadFixture(t, "v1beta2", "clusterqueue-single-flavor")
	delete(obj.Object, "status")
	cq, err := parseClusterQueue(obj, "v1beta2")
	require.NoError(t, err)
	assert.Empty(t, cq.Reserved)
	assert.Zero(t, cq.PendingWorkloads)
	assert.Equal(t, 1, cq.FlavorCount)
}

func TestParseLocalQueue(t *testing.T) {
	tests := []struct {
		fixture    string
		wantActive bool
	}{
		{fixture: "localqueue-active", wantActive: true},
		{fixture: "localqueue-inactive", wantActive: false},
	}
	for _, version := range _versions {
		for _, tt := range tests {
			t.Run(version+"/"+tt.fixture, func(t *testing.T) {
				clusterQueue, active, err := parseLocalQueue(loadFixture(t, version, tt.fixture))
				require.NoError(t, err)
				assert.Equal(t, "cluster-queue", clusterQueue)
				assert.Equal(t, tt.wantActive, active)
			})
		}
	}

	t.Run("no conditions yet", func(t *testing.T) {
		obj := loadFixture(t, "v1beta2", "localqueue-active")
		delete(obj.Object, "status")
		clusterQueue, active, err := parseLocalQueue(obj)
		require.NoError(t, err)
		assert.Equal(t, "cluster-queue", clusterQueue)
		assert.False(t, active)
	})

	t.Run("cluster queue not a string", func(t *testing.T) {
		obj := loadFixture(t, "v1beta2", "localqueue-active")
		obj.Object["spec"].(map[string]interface{})["clusterQueue"] = []interface{}{}
		_, _, err := parseLocalQueue(obj)
		require.ErrorContains(t, err, "LocalQueue default/ma-team-a: spec.clusterQueue is []interface {}, not a string")
	})
}

func TestScorable(t *testing.T) {
	oneFlavor := &ClusterQueueView{FlavorCount: 1}
	tests := []struct {
		name string
		view View
		want bool
	}{
		{name: "synced, one flavor, no cohort", view: View{Synced: true, Exists: true, ClusterQueue: oneFlavor}, want: true},
		{name: "unsynced", view: View{ClusterQueue: oneFlavor}},
		{name: "no ClusterQueue", view: View{Synced: true, Exists: true}},
		{name: "two flavors", view: View{Synced: true, ClusterQueue: &ClusterQueueView{FlavorCount: 2}}},
		{name: "no flavor", view: View{Synced: true, ClusterQueue: &ClusterQueueView{}}},
		{name: "cohort", view: View{Synced: true, ClusterQueue: &ClusterQueueView{FlavorCount: 1, Cohort: "c"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.view.Scorable())
		})
	}
}
