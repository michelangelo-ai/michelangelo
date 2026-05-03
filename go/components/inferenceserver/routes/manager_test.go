package routes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testNamespace      = "default"
	testISName         = "is-1"
	testDeploymentName = "dep-1"
	testModelName      = "model-1"
	testClusterID      = "c-1"
)

var testIS = &v2pb.InferenceServer{
	ObjectMeta: metav1.ObjectMeta{
		Name:      testISName,
		Namespace: testNamespace,
		UID:       "is-uid",
	},
}

// newDynamicClient returns a fake dynamic client, optionally seeded with an
// existing HTTPRoute.
func newDynamicClient(route *unstructured.Unstructured) *fake.FakeDynamicClient {
	if route != nil {
		return fake.NewSimpleDynamicClient(scheme.Scheme, route)
	}
	return fake.NewSimpleDynamicClient(scheme.Scheme)
}

// newClusterClient is an alias for newDynamicClient kept for clarity in
// per-cluster traffic-route tests.
func newClusterClient(route *unstructured.Unstructured) *fake.FakeDynamicClient {
	return newDynamicClient(route)
}

// getRoute fetches the HTTPRoute from the dynamic client.
func getRoute(t *testing.T, c *fake.FakeDynamicClient, name string) *unstructured.Unstructured {
	t.Helper()
	out, err := c.Resource(httpRouteGVR).Namespace(testNamespace).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	return out
}

// rulesOf returns the rules slice from an unstructured HTTPRoute.
func rulesOf(t *testing.T, route *unstructured.Unstructured) []interface{} {
	t.Helper()
	rules, _, err := unstructured.NestedSlice(route.Object, "spec", "rules")
	require.NoError(t, err)
	return rules
}

// applyServerSideDefaults simulates the Gateway API server projecting CRD
// defaults onto every stored backendRef. The fake dynamic client does not run
// CRD defaulters, so without this the Upsert→Exists round-trip cases would
// pass on a buggy builder that omits a server-defaulted field, missing the
// class of bug where Retrieve loops because read != built.
func applyServerSideDefaults(t *testing.T, c *fake.FakeDynamicClient, routeName string) {
	t.Helper()
	ctx := context.Background()
	obj, err := c.Resource(httpRouteGVR).Namespace(testNamespace).Get(ctx, routeName, metav1.GetOptions{})
	require.NoError(t, err)
	rules := rulesOf(t, obj)
	for _, r := range rules {
		rm := r.(map[string]interface{})
		refs, _, _ := unstructured.NestedSlice(rm, "backendRefs")
		for _, ref := range refs {
			refm := ref.(map[string]interface{})
			if _, ok := refm["weight"]; !ok {
				refm["weight"] = int64(1)
			}
		}
		require.NoError(t, unstructured.SetNestedSlice(rm, refs, "backendRefs"))
	}
	require.NoError(t, unstructured.SetNestedSlice(obj.Object, rules, "spec", "rules"))
	_, err = c.Resource(httpRouteGVR).Namespace(testNamespace).Update(ctx, obj, metav1.UpdateOptions{})
	require.NoError(t, err)
}

// trafficRewriteOf returns the per-deployment rule's URLRewrite target so a
// table test can assert the model name embedded in the rewrite filter.
func trafficRewriteOf(t *testing.T, rule interface{}) string {
	t.Helper()
	filters, _, err := unstructured.NestedSlice(rule.(map[string]interface{}), "filters")
	require.NoError(t, err)
	rewrite, _, err := unstructured.NestedString(filters[0].(map[string]interface{}), "urlRewrite", "path", "replacePrefixMatch")
	require.NoError(t, err)
	return rewrite
}

func TestEnsureDiscoveryRoute(t *testing.T) {
	tests := []struct {
		name      string
		seed      func(t *testing.T) *unstructured.Unstructured
		wantPaths []string
	}{
		{
			name:      "creates route with default rule when absent",
			seed:      nil,
			wantPaths: []string{"/" + testISName},
		},
		{
			name: "preserves existing per-deployment rules",
			seed: func(t *testing.T) *unstructured.Unstructured {
				r := newDiscoveryRoute(testIS)
				rules := rulesOf(t, r)
				rules = append(rules, discoveryDeploymentRule(testISName, testDeploymentName))
				require.NoError(t, unstructured.SetNestedSlice(r.Object, rules, "spec", "rules"))
				return r
			},
			wantPaths: []string{"/" + testISName, "/" + testISName + "/" + testDeploymentName},
		},
		{
			// Default rule is appended, not prepended, so the deployment rule stays first.
			name: "restores missing default rule without reordering existing rules",
			seed: func(t *testing.T) *unstructured.Unstructured {
				r := newDiscoveryRoute(testIS)
				require.NoError(t, unstructured.SetNestedSlice(r.Object,
					[]interface{}{discoveryDeploymentRule(testISName, testDeploymentName)},
					"spec", "rules"))
				return r
			},
			wantPaths: []string{"/" + testISName + "/" + testDeploymentName, "/" + testISName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seeded *unstructured.Unstructured
			if tt.seed != nil {
				seeded = tt.seed(t)
			}
			c := newDynamicClient(seeded)
			p := NewDefaultRouteProvider()

			require.NoError(t, p.EnsureDiscoveryRoute(context.Background(), c, testIS))

			rules := rulesOf(t, getRoute(t, c, discoveryRouteName(testISName)))
			require.Len(t, rules, len(tt.wantPaths))
			for i, want := range tt.wantPaths {
				assert.Equal(t, want, rulePath(rules[i]), "rule path at index %d", i)
			}
		})
	}
}

func TestEnsureTrafficRoute(t *testing.T) {
	tests := []struct {
		name      string
		seed      func(t *testing.T) *unstructured.Unstructured
		wantPaths []string
	}{
		{
			name:      "creates route with default rule when absent",
			seed:      nil,
			wantPaths: []string{"/cluster/" + testISName},
		},
		{
			name: "preserves existing per-deployment rules",
			seed: func(t *testing.T) *unstructured.Unstructured {
				r := newTrafficRoute(testISName, testNamespace)
				rules := rulesOf(t, r)
				rules = append(rules, trafficDeploymentRule(testISName, testDeploymentName, testModelName))
				require.NoError(t, unstructured.SetNestedSlice(r.Object, rules, "spec", "rules"))
				return r
			},
			wantPaths: []string{"/cluster/" + testISName, "/cluster/" + testISName + "/" + testDeploymentName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seeded *unstructured.Unstructured
			if tt.seed != nil {
				seeded = tt.seed(t)
			}
			c := newClusterClient(seeded)
			p := NewDefaultRouteProvider()

			require.NoError(t, p.EnsureTrafficRoute(context.Background(), c, testClusterID, testISName, testNamespace))

			rules := rulesOf(t, getRoute(t, c, trafficRouteName(testISName)))
			require.Len(t, rules, len(tt.wantPaths))
			for i, want := range tt.wantPaths {
				assert.Equal(t, want, rulePath(rules[i]), "rule path at index %d", i)
			}
		})
	}
}

func TestDeleteDiscoveryRoute(t *testing.T) {
	tests := []struct {
		name string
		seed func(t *testing.T) *unstructured.Unstructured
	}{
		{name: "deletes an existing route", seed: func(t *testing.T) *unstructured.Unstructured { return newDiscoveryRoute(testIS) }},
		{name: "tolerates a missing route", seed: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seeded *unstructured.Unstructured
			if tt.seed != nil {
				seeded = tt.seed(t)
			}
			c := newDynamicClient(seeded)
			p := NewDefaultRouteProvider()

			require.NoError(t, p.DeleteDiscoveryRoute(context.Background(), c, testISName, testNamespace))

			_, err := c.Resource(httpRouteGVR).Namespace(testNamespace).Get(context.Background(), discoveryRouteName(testISName), metav1.GetOptions{})
			assert.Error(t, err, "route must be absent after delete")
		})
	}
}

func TestDeleteTrafficRoute(t *testing.T) {
	tests := []struct {
		name string
		seed func(t *testing.T) *unstructured.Unstructured
	}{
		{name: "deletes an existing route", seed: func(t *testing.T) *unstructured.Unstructured { return newTrafficRoute(testISName, testNamespace) }},
		{name: "tolerates a missing route", seed: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seeded *unstructured.Unstructured
			if tt.seed != nil {
				seeded = tt.seed(t)
			}
			c := newClusterClient(seeded)
			p := NewDefaultRouteProvider()

			require.NoError(t, p.DeleteTrafficRoute(context.Background(), c, testClusterID, testISName, testNamespace))

			_, err := c.Resource(httpRouteGVR).Namespace(testNamespace).Get(context.Background(), trafficRouteName(testISName), metav1.GetOptions{})
			assert.Error(t, err, "route must be absent after delete")
		})
	}
}

// preUpsert lets a table case seed the route with one or more existing
// per-deployment rules before the case's primary Upsert call.
type preUpsert struct {
	deployment string
	model      string
}

func TestUpsertDiscoveryRule(t *testing.T) {
	tests := []struct {
		name string
		// pre is applied via UpsertDiscoveryRule before the primary call so the
		// route reaches a realistic seeded state (rules carry server defaults
		// after the projection step below).
		pre []preUpsert
		// projectAfterPre simulates the K8s server defaulting fields on the
		// stored route between the seed step and the primary Upsert. Catches
		// builders whose output diverges from the server-projected form.
		projectAfterPre bool
		// Inputs to the primary Upsert.
		deployment string
		model      string
		// Expected rule paths in order after the primary Upsert completes.
		wantPaths []string
	}{
		{
			name:       "appends rule for a new deployment on a fresh route",
			deployment: testDeploymentName,
			model:      testModelName,
			wantPaths:  []string{"/" + testISName, "/" + testISName + "/" + testDeploymentName},
		},
		{
			name:            "is a no-op when the same deployment is upserted again",
			pre:             []preUpsert{{testDeploymentName, testModelName}},
			projectAfterPre: true,
			deployment:      testDeploymentName,
			model:           testModelName,
			wantPaths:       []string{"/" + testISName, "/" + testISName + "/" + testDeploymentName},
		},
		{
			// Discovery rule body is model-agnostic (the model rewrite happens at
			// the traffic route), so changing the model name must not change the
			// rule and must not trigger an Update loop.
			name:            "is a no-op when only the model changes",
			pre:             []preUpsert{{testDeploymentName, "old-model"}},
			projectAfterPre: true,
			deployment:      testDeploymentName,
			model:           "new-model",
			wantPaths:       []string{"/" + testISName, "/" + testISName + "/" + testDeploymentName},
		},
		{
			name:       "appends a distinct rule for a second deployment",
			pre:        []preUpsert{{"dep-A", testModelName}},
			deployment: "dep-B",
			model:      testModelName,
			wantPaths:  []string{"/" + testISName, "/" + testISName + "/dep-A", "/" + testISName + "/dep-B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newDynamicClient(newDiscoveryRoute(testIS))
			p := NewDefaultRouteProvider()
			ctx := context.Background()

			for _, seed := range tt.pre {
				require.NoError(t, p.UpsertDiscoveryRule(ctx, c, testISName, testNamespace, seed.deployment, seed.model))
			}
			if tt.projectAfterPre {
				applyServerSideDefaults(t, c, discoveryRouteName(testISName))
			}

			require.NoError(t, p.UpsertDiscoveryRule(ctx, c, testISName, testNamespace, tt.deployment, tt.model))

			rules := rulesOf(t, getRoute(t, c, discoveryRouteName(testISName)))
			require.Len(t, rules, len(tt.wantPaths))
			for i, want := range tt.wantPaths {
				assert.Equal(t, want, rulePath(rules[i]), "rule path at index %d", i)
			}
		})
	}
}

func TestUpsertTrafficRule(t *testing.T) {
	tests := []struct {
		name            string
		pre             []preUpsert
		projectAfterPre bool
		deployment      string
		model           string
		wantPaths       []string
		// wantRewriteAt asserts the URLRewrite target on a specific rule index.
		// Use only for cases where the rewrite is the property under test.
		wantRewriteAt int
		wantRewrite   string
	}{
		{
			name:          "appends rule for a new deployment on a fresh route",
			deployment:    testDeploymentName,
			model:         testModelName,
			wantPaths:     []string{"/cluster/" + testISName, "/cluster/" + testISName + "/" + testDeploymentName},
			wantRewriteAt: 1,
			wantRewrite:   "/v2/models/" + testModelName,
		},
		{
			name:            "is a no-op when the same deployment + same model is upserted again",
			pre:             []preUpsert{{testDeploymentName, testModelName}},
			projectAfterPre: true,
			deployment:      testDeploymentName,
			model:           testModelName,
			wantPaths:       []string{"/cluster/" + testISName, "/cluster/" + testISName + "/" + testDeploymentName},
			wantRewriteAt:   1,
			wantRewrite:     "/v2/models/" + testModelName,
		},
		{
			// The bug this regression catches: previously the builder omitted
			// the server-defaulted backendRef.weight, so after a desiredRevision
			// change the comparison kept reporting drift even after the rewrite
			// was correctly updated to the new model.
			name:            "updates the rewrite when the model changes for the same deployment",
			pre:             []preUpsert{{testDeploymentName, "old-model"}},
			projectAfterPre: true,
			deployment:      testDeploymentName,
			model:           "new-model",
			wantPaths:       []string{"/cluster/" + testISName, "/cluster/" + testISName + "/" + testDeploymentName},
			wantRewriteAt:   1,
			wantRewrite:     "/v2/models/new-model",
		},
		{
			// Two deployments referencing the same model produce distinct rules
			// because the match path is keyed by deployment name. Removing one
			// must not affect the other (covered separately by the Remove test).
			name:          "appends a distinct rule when a second deployment shares the model",
			pre:           []preUpsert{{"dep-A", "shared-model"}},
			deployment:    "dep-B",
			model:         "shared-model",
			wantPaths:     []string{"/cluster/" + testISName, "/cluster/" + testISName + "/dep-A", "/cluster/" + testISName + "/dep-B"},
			wantRewriteAt: 2,
			wantRewrite:   "/v2/models/shared-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newClusterClient(newTrafficRoute(testISName, testNamespace))
			p := NewDefaultRouteProvider()
			ctx := context.Background()

			for _, seed := range tt.pre {
				require.NoError(t, p.UpsertTrafficRule(ctx, c, testClusterID, testISName, testNamespace, seed.deployment, seed.model))
			}
			if tt.projectAfterPre {
				applyServerSideDefaults(t, c, trafficRouteName(testISName))
			}

			require.NoError(t, p.UpsertTrafficRule(ctx, c, testClusterID, testISName, testNamespace, tt.deployment, tt.model))

			rules := rulesOf(t, getRoute(t, c, trafficRouteName(testISName)))
			require.Len(t, rules, len(tt.wantPaths))
			for i, want := range tt.wantPaths {
				assert.Equal(t, want, rulePath(rules[i]), "rule path at index %d", i)
			}
			if tt.wantRewrite != "" {
				assert.Equal(t, tt.wantRewrite, trafficRewriteOf(t, rules[tt.wantRewriteAt]))
			}
		})
	}
}

func TestRemoveDiscoveryRule(t *testing.T) {
	tests := []struct {
		name        string
		pre         []preUpsert
		seedRouteFn func(t *testing.T) *unstructured.Unstructured // overrides default seed when set
		removeDep   string
		wantPaths   []string
	}{
		{
			name:      "removes the deployment's rule and preserves others",
			pre:       []preUpsert{{"dep-A", testModelName}, {"dep-B", testModelName}},
			removeDep: "dep-A",
			wantPaths: []string{"/" + testISName, "/" + testISName + "/dep-B"},
		},
		{
			name:      "is a no-op when the deployment's rule is absent",
			removeDep: "missing",
			wantPaths: []string{"/" + testISName},
		},
		{
			name:        "tolerates a missing route",
			seedRouteFn: func(t *testing.T) *unstructured.Unstructured { return nil },
			removeDep:   testDeploymentName,
			// No assertion on rules; the route is absent. Test passes if Remove returns no error.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seeded *unstructured.Unstructured
			if tt.seedRouteFn != nil {
				seeded = tt.seedRouteFn(t)
			} else {
				seeded = newDiscoveryRoute(testIS)
			}
			c := newDynamicClient(seeded)
			p := NewDefaultRouteProvider()
			ctx := context.Background()

			for _, seed := range tt.pre {
				require.NoError(t, p.UpsertDiscoveryRule(ctx, c, testISName, testNamespace, seed.deployment, seed.model))
			}

			require.NoError(t, p.RemoveDiscoveryRule(ctx, c, testISName, testNamespace, tt.removeDep))

			if tt.wantPaths == nil {
				return
			}
			rules := rulesOf(t, getRoute(t, c, discoveryRouteName(testISName)))
			require.Len(t, rules, len(tt.wantPaths))
			for i, want := range tt.wantPaths {
				assert.Equal(t, want, rulePath(rules[i]), "rule path at index %d", i)
			}
		})
	}
}

func TestRemoveTrafficRule(t *testing.T) {
	tests := []struct {
		name        string
		pre         []preUpsert
		seedRouteFn func(t *testing.T) *unstructured.Unstructured
		removeDep   string
		wantPaths   []string
	}{
		{
			name:      "removes the deployment's rule and preserves others",
			pre:       []preUpsert{{"dep-A", "shared-model"}, {"dep-B", "shared-model"}},
			removeDep: "dep-A",
			wantPaths: []string{"/cluster/" + testISName, "/cluster/" + testISName + "/dep-B"},
		},
		{
			name:      "is a no-op when the deployment's rule is absent",
			removeDep: "missing",
			wantPaths: []string{"/cluster/" + testISName},
		},
		{
			name:        "tolerates a missing route",
			seedRouteFn: func(t *testing.T) *unstructured.Unstructured { return nil },
			removeDep:   testDeploymentName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seeded *unstructured.Unstructured
			if tt.seedRouteFn != nil {
				seeded = tt.seedRouteFn(t)
			} else {
				seeded = newTrafficRoute(testISName, testNamespace)
			}
			c := newClusterClient(seeded)
			p := NewDefaultRouteProvider()
			ctx := context.Background()

			for _, seed := range tt.pre {
				require.NoError(t, p.UpsertTrafficRule(ctx, c, testClusterID, testISName, testNamespace, seed.deployment, seed.model))
			}

			require.NoError(t, p.RemoveTrafficRule(ctx, c, testClusterID, testISName, testNamespace, tt.removeDep))

			if tt.wantPaths == nil {
				return
			}
			rules := rulesOf(t, getRoute(t, c, trafficRouteName(testISName)))
			require.Len(t, rules, len(tt.wantPaths))
			for i, want := range tt.wantPaths {
				assert.Equal(t, want, rulePath(rules[i]), "rule path at index %d", i)
			}
		})
	}
}

func TestDeploymentDiscoveryRouteExists(t *testing.T) {
	tests := []struct {
		name            string
		pre             []preUpsert
		projectAfterPre bool
		seedRouteFn     func(t *testing.T) *unstructured.Unstructured
		queryDep        string
		want            bool
	}{
		{
			name:     "true when the rule for the deployment is present",
			pre:      []preUpsert{{testDeploymentName, testModelName}},
			queryDep: testDeploymentName,
			want:     true,
		},
		{
			name:     "false when the queried deployment has no rule",
			pre:      []preUpsert{{testDeploymentName, testModelName}},
			queryDep: "missing",
			want:     false,
		},
		{
			name:        "false when the route is absent",
			seedRouteFn: func(t *testing.T) *unstructured.Unstructured { return nil },
			queryDep:    testDeploymentName,
			want:        false,
		},
		{
			// Round-trip regression: catches builders whose output diverges
			// from the server-projected stored form.
			name:            "true after Upsert and server-side defaulting",
			pre:             []preUpsert{{testDeploymentName, testModelName}},
			projectAfterPre: true,
			queryDep:        testDeploymentName,
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seeded *unstructured.Unstructured
			if tt.seedRouteFn != nil {
				seeded = tt.seedRouteFn(t)
			} else {
				seeded = newDiscoveryRoute(testIS)
			}
			c := newDynamicClient(seeded)
			p := NewDefaultRouteProvider()
			ctx := context.Background()

			for _, seed := range tt.pre {
				require.NoError(t, p.UpsertDiscoveryRule(ctx, c, testISName, testNamespace, seed.deployment, seed.model))
			}
			if tt.projectAfterPre {
				applyServerSideDefaults(t, c, discoveryRouteName(testISName))
			}

			got, err := p.DeploymentDiscoveryRouteExists(ctx, c, testISName, testNamespace, tt.queryDep)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDeploymentTrafficRouteExists(t *testing.T) {
	tests := []struct {
		name            string
		pre             []preUpsert
		projectAfterPre bool
		seedRouteFn     func(t *testing.T) *unstructured.Unstructured
		queryDep        string
		queryModel      string
		want            bool
	}{
		{
			name:       "true when the rule is present and the model matches",
			pre:        []preUpsert{{testDeploymentName, testModelName}},
			queryDep:   testDeploymentName,
			queryModel: testModelName,
			want:       true,
		},
		{
			name:       "false when the rule's model differs from the query",
			pre:        []preUpsert{{testDeploymentName, "model-A"}},
			queryDep:   testDeploymentName,
			queryModel: "model-B",
			want:       false,
		},
		{
			name:       "false when the queried deployment has no rule",
			pre:        []preUpsert{{testDeploymentName, testModelName}},
			queryDep:   "missing",
			queryModel: testModelName,
			want:       false,
		},
		{
			name:        "false when the route is absent",
			seedRouteFn: func(t *testing.T) *unstructured.Unstructured { return nil },
			queryDep:    testDeploymentName,
			queryModel:  testModelName,
			want:        false,
		},
		{
			// Round-trip regression: catches the bug where a server-defaulted
			// field on the read rule made the equality check report drift even
			// though the controller had just written the matching rule.
			name:            "true after Upsert and server-side defaulting",
			pre:             []preUpsert{{testDeploymentName, testModelName}},
			projectAfterPre: true,
			queryDep:        testDeploymentName,
			queryModel:      testModelName,
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seeded *unstructured.Unstructured
			if tt.seedRouteFn != nil {
				seeded = tt.seedRouteFn(t)
			} else {
				seeded = newTrafficRoute(testISName, testNamespace)
			}
			c := newClusterClient(seeded)
			p := NewDefaultRouteProvider()
			ctx := context.Background()

			for _, seed := range tt.pre {
				require.NoError(t, p.UpsertTrafficRule(ctx, c, testClusterID, testISName, testNamespace, seed.deployment, seed.model))
			}
			if tt.projectAfterPre {
				applyServerSideDefaults(t, c, trafficRouteName(testISName))
			}

			got, err := p.DeploymentTrafficRouteExists(ctx, c, testClusterID, testISName, testNamespace, tt.queryDep, tt.queryModel)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
