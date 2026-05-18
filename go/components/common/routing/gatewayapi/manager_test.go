package gatewayapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
)

func newFakeClient() *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClient(runtime.NewScheme())
}

func newManager() *httpRouteManager {
	return New().(*httpRouteManager)
}

// setupRoute creates an empty route using the manager, for use in test setup.
func setupRoute(t *testing.T, client *fake.FakeDynamicClient, name, ns string) {
	t.Helper()
	require.NoError(t, newManager().Create(context.Background(), client, name, ns, routing.RouteConfig{
		GatewayName:      "gw",
		GatewayNamespace: ns,
	}))
}

// setupRouteWithRules creates a route with the given initial rules, for use in test setup.
func setupRouteWithRules(t *testing.T, client *fake.FakeDynamicClient, name, ns string, rules ...routing.Rule) {
	t.Helper()
	require.NoError(t, newManager().Create(context.Background(), client, name, ns, routing.RouteConfig{
		GatewayName:      "gw",
		GatewayNamespace: ns,
		Rules:            rules,
	}))
}

const (
	testNS   = "test-ns"
	testName = "my-route"
)

func TestCreate(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testing.T, *fake.FakeDynamicClient)
		cfg         routing.RouteConfig
		blockCreate bool
		check       func(*testing.T, *httpRouteManager, *fake.FakeDynamicClient)
	}{
		{
			name: "creates route when absent",
			cfg:  routing.RouteConfig{GatewayName: "gw", GatewayNamespace: testNS, Rules: []routing.Rule{{MatchPath: "/test", BackendName: "svc"}}},
			check: func(t *testing.T, m *httpRouteManager, c *fake.FakeDynamicClient) {
				ok, err := m.Exists(context.Background(), c, testName, testNS)
				require.NoError(t, err)
				assert.True(t, ok)
			},
		},
		{
			name:        "noops when already exists",
			setup:       func(t *testing.T, c *fake.FakeDynamicClient) { setupRoute(t, c, testName, testNS) },
			cfg:         routing.RouteConfig{GatewayName: "gw"},
			blockCreate: true,
		},
		{
			name: "sets owner reference",
			cfg: routing.RouteConfig{
				GatewayName:      "gw",
				GatewayNamespace: testNS,
				OwnerRef:         &routing.OwnerRef{APIVersion: "v1", Kind: "Foo", Name: "bar", UID: "uid-1"},
			},
			check: func(t *testing.T, m *httpRouteManager, c *fake.FakeDynamicClient) {
				got, err := c.Resource(httprouteGVR).Namespace(testNS).Get(context.Background(), testName, metav1.GetOptions{})
				require.NoError(t, err)
				refs := got.GetOwnerReferences()
				require.Len(t, refs, 1)
				assert.Equal(t, "bar", refs[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeClient()
			m := newManager()
			if tt.setup != nil {
				tt.setup(t, client)
			}
			if tt.blockCreate {
				client.PrependReactor("create", "httproutes", func(action k8stesting.Action) (bool, runtime.Object, error) {
					t.Fatal("Create must not be called when the route already exists")
					return true, nil, nil
				})
			}
			require.NoError(t, m.Create(context.Background(), client, testName, testNS, tt.cfg))
			if tt.check != nil {
				tt.check(t, m, client)
			}
		})
	}
}

func TestExists(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testing.T, *fake.FakeDynamicClient)
		wantExist bool
	}{
		{
			name:      "returns true when present",
			setup:     func(t *testing.T, c *fake.FakeDynamicClient) { setupRoute(t, c, testName, testNS) },
			wantExist: true,
		},
		{
			name:      "returns false when absent",
			wantExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeClient()
			m := newManager()
			if tt.setup != nil {
				tt.setup(t, client)
			}
			ok, err := m.Exists(context.Background(), client, testName, testNS)
			require.NoError(t, err)
			assert.Equal(t, tt.wantExist, ok)
		})
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testing.T, *fake.FakeDynamicClient)
		routeName string
		wantExist bool
	}{
		{
			name:      "removes existing route",
			setup:     func(t *testing.T, c *fake.FakeDynamicClient) { setupRoute(t, c, testName, testNS) },
			routeName: testName,
			wantExist: false,
		},
		{
			name:      "tolerates not found",
			routeName: "nonexistent",
			wantExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeClient()
			m := newManager()
			if tt.setup != nil {
				tt.setup(t, client)
			}
			require.NoError(t, m.Delete(context.Background(), client, tt.routeName, testNS))
			ok, err := m.Exists(context.Background(), client, tt.routeName, testNS)
			require.NoError(t, err)
			assert.Equal(t, tt.wantExist, ok)
		})
	}
}

func TestAddRules(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(*testing.T, *fake.FakeDynamicClient)
		routeName       string
		rule            routing.Rule
		wantErrContains string
		wantRuleCount   int
		wantRewrite     string
	}{
		{
			name:          "appends new rule",
			setup:         func(t *testing.T, c *fake.FakeDynamicClient) { setupRoute(t, c, testName, testNS) },
			routeName:     testName,
			rule:          routing.Rule{MatchPath: "/new", BackendName: "svc", BackendPort: 8080},
			wantRuleCount: 1,
		},
		{
			name: "upserts existing rule",
			setup: func(t *testing.T, c *fake.FakeDynamicClient) {
				setupRouteWithRules(t, c, testName, testNS,
					routing.Rule{MatchPath: "/existing", RewritePath: "/old", RewriteType: routing.RewriteFullPath, BackendName: "svc"},
				)
			},
			routeName:     testName,
			rule:          routing.Rule{MatchPath: "/existing", RewritePath: "/new", RewriteType: routing.RewriteFullPath, BackendName: "svc"},
			wantRuleCount: 1,
			wantRewrite:   "/new",
		},
		{
			name:            "errors when route not found",
			routeName:       "missing",
			rule:            routing.Rule{MatchPath: "/x"},
			wantErrContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeClient()
			m := newManager()
			if tt.setup != nil {
				tt.setup(t, client)
			}
			err := m.AddRules(context.Background(), client, tt.routeName, testNS, tt.rule)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}
			require.NoError(t, err)
			got, err := client.Resource(httprouteGVR).Namespace(testNS).Get(context.Background(), tt.routeName, metav1.GetOptions{})
			require.NoError(t, err)
			rules, _, _ := unstructured.NestedSlice(got.Object, "spec", "rules")
			assert.Len(t, rules, tt.wantRuleCount)
			if tt.wantRewrite != "" {
				assert.Equal(t, tt.wantRewrite, httpRuleRewritePath(rules[0]))
			}
		})
	}
}

func TestRemoveRules(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*testing.T, *fake.FakeDynamicClient)
		routeName     string
		removePaths   []string
		wantRuleCount int
		wantPath      string
	}{
		{
			name: "removes matching rule",
			setup: func(t *testing.T, c *fake.FakeDynamicClient) {
				setupRouteWithRules(t, c, testName, testNS,
					routing.Rule{MatchPath: "/keep", BackendName: "svc"},
					routing.Rule{MatchPath: "/remove", BackendName: "svc"},
				)
			},
			routeName:     testName,
			removePaths:   []string{"/remove"},
			wantRuleCount: 1,
			wantPath:      "/keep",
		},
		{
			name:          "noops when route absent",
			routeName:     "absent",
			removePaths:   []string{"/x"},
			wantRuleCount: -1, // skip rule count check — route doesn't exist
		},
		{
			name: "noops when rule absent",
			setup: func(t *testing.T, c *fake.FakeDynamicClient) {
				setupRouteWithRules(t, c, testName, testNS,
					routing.Rule{MatchPath: "/keep", BackendName: "svc"},
				)
			},
			routeName:     testName,
			removePaths:   []string{"/nonexistent"},
			wantRuleCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeClient()
			m := newManager()
			if tt.setup != nil {
				tt.setup(t, client)
			}
			require.NoError(t, m.RemoveRules(context.Background(), client, tt.routeName, testNS, tt.removePaths...))
			if tt.wantRuleCount < 0 {
				return
			}
			got, err := client.Resource(httprouteGVR).Namespace(testNS).Get(context.Background(), tt.routeName, metav1.GetOptions{})
			require.NoError(t, err)
			rules, _, _ := unstructured.NestedSlice(got.Object, "spec", "rules")
			assert.Len(t, rules, tt.wantRuleCount)
			if tt.wantPath != "" {
				assert.Equal(t, tt.wantPath, rulePath(rules[0]))
			}
		})
	}
}

func TestRuleExists(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testing.T, *fake.FakeDynamicClient)
		routeName string
		rule      routing.Rule
		want      bool
	}{
		{
			name: "returns true when path present",
			setup: func(t *testing.T, c *fake.FakeDynamicClient) {
				setupRouteWithRules(t, c, testName, testNS, routing.Rule{MatchPath: "/found", BackendName: "svc"})
			},
			routeName: testName,
			rule:      routing.Rule{MatchPath: "/found"},
			want:      true,
		},
		{
			name:      "returns false when path absent",
			setup:     func(t *testing.T, c *fake.FakeDynamicClient) { setupRoute(t, c, testName, testNS) },
			routeName: testName,
			rule:      routing.Rule{MatchPath: "/missing"},
			want:      false,
		},
		{
			name:      "returns false when route absent",
			routeName: "absent",
			rule:      routing.Rule{MatchPath: "/x"},
			want:      false,
		},
		{
			name: "returns true when path and rewrite match",
			setup: func(t *testing.T, c *fake.FakeDynamicClient) {
				setupRouteWithRules(t, c, testName, testNS,
					routing.Rule{MatchPath: "/p", RewritePath: "/v2", RewriteType: routing.RewriteFullPath, BackendName: "svc"},
				)
			},
			routeName: testName,
			rule:      routing.Rule{MatchPath: "/p", RewritePath: "/v2"},
			want:      true,
		},
		{
			name: "returns false when rewrite differs",
			setup: func(t *testing.T, c *fake.FakeDynamicClient) {
				setupRouteWithRules(t, c, testName, testNS,
					routing.Rule{MatchPath: "/p", RewritePath: "/v2", RewriteType: routing.RewriteFullPath, BackendName: "svc"},
				)
			},
			routeName: testName,
			rule:      routing.Rule{MatchPath: "/p", RewritePath: "/other"},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeClient()
			m := newManager()
			if tt.setup != nil {
				tt.setup(t, client)
			}
			ok, err := m.RuleExists(context.Background(), client, tt.routeName, testNS, tt.rule)
			require.NoError(t, err)
			assert.Equal(t, tt.want, ok)
		})
	}
}
