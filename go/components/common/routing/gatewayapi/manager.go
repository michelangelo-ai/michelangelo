package gatewayapi

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"

	"github.com/michelangelo-ai/michelangelo/go/components/common/keyedmutex"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
)

var _ routing.Manager = &httpRouteManager{}

type httpRouteManager struct {
	mu *keyedmutex.Map
}

// New returns the Gateway API HTTPRoute implementation of routing.Manager.
func New() routing.Manager {
	return &httpRouteManager{mu: keyedmutex.New()}
}

func (m *httpRouteManager) Create(ctx context.Context, client dynamic.Interface, name, namespace string, config routing.RouteConfig) error {
	obj, err := client.Resource(httprouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if obj != nil {
		return nil
	}
	route := buildHTTPRoute(name, namespace, config)
	_, createErr := client.Resource(httprouteGVR).Namespace(namespace).Create(ctx, route, metav1.CreateOptions{})
	if createErr != nil && !apierrors.IsAlreadyExists(createErr) {
		return createErr
	}
	return nil
}

func (m *httpRouteManager) Exists(ctx context.Context, client dynamic.Interface, name, namespace string) (bool, error) {
	obj, err := client.Resource(httprouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return obj != nil, nil
}

func (m *httpRouteManager) Delete(ctx context.Context, client dynamic.Interface, name, namespace string) error {
	err := client.Resource(httprouteGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (m *httpRouteManager) AddRules(ctx context.Context, client dynamic.Interface, name, namespace string, rules ...routing.Rule) error {
	unlock := m.mu.Lock(namespace + "/" + name)
	defer unlock()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj, err := client.Resource(httprouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("route %s/%s not found", namespace, name)
		}
		if err != nil {
			return err
		}
		existing, _, _ := unstructured.NestedSlice(obj.Object, "spec", "rules")
		desired := existing
		for _, rule := range rules {
			desired = upsertHTTPRule(desired, buildHTTPRule(rule))
		}
		if httpRulesEqual(existing, desired) {
			return nil
		}
		if setErr := unstructured.SetNestedSlice(obj.Object, desired, "spec", "rules"); setErr != nil {
			return setErr
		}
		_, err = client.Resource(httprouteGVR).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
		return err
	})
}

func (m *httpRouteManager) RemoveRules(ctx context.Context, client dynamic.Interface, name, namespace string, matchPaths ...string) error {
	unlock := m.mu.Lock(namespace + "/" + name)
	defer unlock()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj, err := client.Resource(httprouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		existing, _, _ := unstructured.NestedSlice(obj.Object, "spec", "rules")
		desired := existing
		for _, path := range matchPaths {
			desired = removeHTTPRuleByPath(desired, path)
		}
		if httpRulesEqual(existing, desired) {
			return nil
		}
		if setErr := unstructured.SetNestedSlice(obj.Object, desired, "spec", "rules"); setErr != nil {
			return setErr
		}
		_, err = client.Resource(httprouteGVR).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
		return err
	})
}

func (m *httpRouteManager) RuleExists(ctx context.Context, client dynamic.Interface, name, namespace string, rule routing.Rule) (bool, error) {
	obj, err := client.Resource(httprouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rules, _, _ := unstructured.NestedSlice(obj.Object, "spec", "rules")
	i := findHTTPRuleByPath(rules, rule.MatchPath)
	if i < 0 {
		return false, nil
	}
	if rule.RewritePath == "" {
		return true, nil
	}
	return httpRuleRewritePath(rules[i]) == rule.RewritePath, nil
}

func buildHTTPRoute(name, namespace string, config routing.RouteConfig) *unstructured.Unstructured {
	route := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
	}}
	route.SetName(name)
	route.SetNamespace(namespace)

	spec := map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{
				"group":     "gateway.networking.k8s.io",
				"kind":      "Gateway",
				"name":      config.GatewayName,
				"namespace": config.GatewayNamespace,
			},
		},
	}
	if len(config.Rules) > 0 {
		httpRules := make([]interface{}, len(config.Rules))
		for i, r := range config.Rules {
			httpRules[i] = buildHTTPRule(r)
		}
		spec["rules"] = httpRules
	}
	_ = unstructured.SetNestedMap(route.Object, spec, "spec")

	if ref := config.OwnerRef; ref != nil {
		t := true
		route.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion:         ref.APIVersion,
			Kind:               ref.Kind,
			Name:               ref.Name,
			UID:                ref.UID,
			Controller:         &t,
			BlockOwnerDeletion: &t,
		}})
	}
	return route
}

func buildHTTPRule(rule routing.Rule) map[string]interface{} {
	matchType := "Exact"
	if rule.MatchType == routing.PathMatchPrefix {
		matchType = "PathPrefix"
	}

	result := map[string]interface{}{
		"matches": []interface{}{
			map[string]interface{}{
				"path": map[string]interface{}{
					"type":  matchType,
					"value": rule.MatchPath,
				},
			},
		},
	}

	if rule.RewritePath != "" {
		var rewriteTypeName, rewriteKey string
		if rule.RewriteType == routing.RewriteFullPath {
			rewriteTypeName = "ReplaceFullPath"
			rewriteKey = "replaceFullPath"
		} else {
			rewriteTypeName = "ReplacePrefixMatch"
			rewriteKey = "replacePrefixMatch"
		}
		result["filters"] = []interface{}{
			map[string]interface{}{
				"type": "URLRewrite",
				"urlRewrite": map[string]interface{}{
					"path": map[string]interface{}{
						"type":     rewriteTypeName,
						rewriteKey: rule.RewritePath,
					},
				},
			},
		}
	}

	port := rule.BackendPort
	if port == 0 {
		port = 80
	}
	if rule.BackendName != "" {
		result["backendRefs"] = []interface{}{
			map[string]interface{}{
				"group":  "",
				"kind":   "Service",
				"name":   rule.BackendName,
				"port":   int64(port),
				"weight": int64(1),
			},
		}
	}
	return result
}

func upsertHTTPRule(rules []interface{}, rule map[string]interface{}) []interface{} {
	target := rulePath(rule)
	out := make([]interface{}, len(rules))
	copy(out, rules)
	if i := findHTTPRuleByPath(out, target); i >= 0 {
		out[i] = rule
		return out
	}
	return append(out, rule)
}

func removeHTTPRuleByPath(rules []interface{}, path string) []interface{} {
	out := make([]interface{}, 0, len(rules))
	for _, r := range rules {
		if rulePath(r) == path {
			continue
		}
		out = append(out, r)
	}
	return out
}

func findHTTPRuleByPath(rules []interface{}, path string) int {
	for i, r := range rules {
		if rulePath(r) == path {
			return i
		}
	}
	return -1
}

func httpRuleRewritePath(rule interface{}) string {
	rm, ok := rule.(map[string]interface{})
	if !ok {
		return ""
	}
	filters, _, _ := unstructured.NestedSlice(rm, "filters")
	if len(filters) == 0 {
		return ""
	}
	fm, ok := filters[0].(map[string]interface{})
	if !ok {
		return ""
	}
	if v, _, _ := unstructured.NestedString(fm, "urlRewrite", "path", "replaceFullPath"); v != "" {
		return v
	}
	v, _, _ := unstructured.NestedString(fm, "urlRewrite", "path", "replacePrefixMatch")
	return v
}

func httpRulesEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !mapsDeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func mapsDeepEqual(a, b interface{}) bool {
	am, aok := a.(map[string]interface{})
	bm, bok := b.(map[string]interface{})
	if aok != bok {
		return false
	}
	if !aok {
		return a == b
	}
	if len(am) != len(bm) {
		return false
	}
	for k, av := range am {
		bv, ok := bm[k]
		if !ok {
			return false
		}
		if asl, ok := av.([]interface{}); ok {
			bsl, ok := bv.([]interface{})
			if !ok || len(asl) != len(bsl) {
				return false
			}
			for i := range asl {
				if !mapsDeepEqual(asl[i], bsl[i]) {
					return false
				}
			}
			continue
		}
		if !mapsDeepEqual(av, bv) {
			return false
		}
	}
	return true
}
