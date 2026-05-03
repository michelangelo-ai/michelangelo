package routes

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/keyedmutex"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	gatewayAPIGroup         = "gateway.networking.k8s.io"
	gatewayAPIVersion       = "v1"
	httpRouteKind           = "HTTPRoute"
	httpRouteResource       = "httproutes"
	gatewayKind             = "Gateway"
	gatewayName             = "ma-gateway"
	servicePort       int64 = 80
	backendRefWeight  int64 = 1

	inferenceServerKind = "InferenceServer"

	discoveryRouteSuffix       = "-discovery"
	trafficRouteSuffix         = "-traffic"
	endpointsServiceSuffix     = "-endpoints"
	inferenceServiceSuffix     = "-inference-service"
	tritonAPIPrefix            = "/v2"
	tritonModelPathPrefix      = "/v2/models"
	clusterPathPrefix          = "/cluster"
	pathTypeExact              = "Exact"
	pathTypePathPrefix         = "PathPrefix"
	rewritePathReplaceFullPath = "ReplaceFullPath"
	rewritePathReplacePrefix   = "ReplacePrefixMatch"
	filterTypeURLRewrite       = "URLRewrite"
)

var (
	httpRouteGVR = schema.GroupVersionResource{
		Group:    gatewayAPIGroup,
		Version:  gatewayAPIVersion,
		Resource: httpRouteResource,
	}
	httpRouteAPIVersion = gatewayAPIGroup + "/" + gatewayAPIVersion
)

var _ RouteProvider = &defaultRouteProvider{}

// defaultRouteProvider serializes mutating writes on each route via a per-route
// in-memory mutex and absorbs stale-cache conflicts via RetryOnConflict.
type defaultRouteProvider struct {
	locks *keyedmutex.Map
}

// NewDefaultRouteProvider returns the default RouteProvider implementation.
func NewDefaultRouteProvider() RouteProvider {
	return &defaultRouteProvider{locks: keyedmutex.New()}
}

// EnsureDiscoveryRoute creates the discovery HTTPRoute for the InferenceServer
// if it does not exist, and ensures its default rule is present. Existing
// per-Deployment rules are preserved.
func (p *defaultRouteProvider) EnsureDiscoveryRoute(ctx context.Context, dynamicClient dynamic.Interface, server *v2pb.InferenceServer) error {
	unlock := p.locks.Lock(discoveryLockKey(server.Namespace, server.Name))
	defer unlock()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		name := discoveryRouteName(server.Name)
		existing, err := dynamicClient.Resource(httpRouteGVR).Namespace(server.Namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			route := newDiscoveryRoute(server)
			setInferenceServerOwner(route, server)
			if _, createErr := dynamicClient.Resource(httpRouteGVR).Namespace(server.Namespace).Create(ctx, route, metav1.CreateOptions{}); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
				return fmt.Errorf("create discovery route: %w", createErr)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("get discovery route: %w", err)
		}

		rules, _, err := unstructured.NestedSlice(existing.Object, "spec", "rules")
		if err != nil {
			return fmt.Errorf("read existing rules: %w", err)
		}
		desired := upsertRule(rules, discoveryDefaultRule(server.Name))
		if equalRules(rules, desired) {
			return nil
		}
		if err := unstructured.SetNestedSlice(existing.Object, desired, "spec", "rules"); err != nil {
			return fmt.Errorf("set rules on discovery route: %w", err)
		}
		if _, err := dynamicClient.Resource(httpRouteGVR).Namespace(server.Namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update discovery route: %w", err)
		}
		return nil
	})
}

// DeleteDiscoveryRoute removes the discovery HTTPRoute. Tolerates not-found.
func (p *defaultRouteProvider) DeleteDiscoveryRoute(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string) error {
	if err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Delete(ctx, discoveryRouteName(inferenceServerName), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete discovery route: %w", err)
	}
	return nil
}

// DiscoveryRouteExists reports whether the discovery HTTPRoute object has
// been provisioned for the InferenceServer.
func (p *defaultRouteProvider) DiscoveryRouteExists(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string) (bool, error) {
	_, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, discoveryRouteName(inferenceServerName), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get discovery route: %w", err)
	}
	return true, nil
}

// EnsureTrafficRoute creates the traffic HTTPRoute in the target cluster if it
// does not exist, and ensures its default rule is present.
func (p *defaultRouteProvider) EnsureTrafficRoute(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string) error {
	unlock := p.locks.Lock(trafficLockKey(namespace, inferenceServerName, clusterID))
	defer unlock()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		name := trafficRouteName(inferenceServerName)
		existing, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			route := newTrafficRoute(inferenceServerName, namespace)
			if _, createErr := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Create(ctx, route, metav1.CreateOptions{}); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
				return fmt.Errorf("create traffic route in cluster %s: %w", clusterID, createErr)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("get traffic route in cluster %s: %w", clusterID, err)
		}

		rules, _, err := unstructured.NestedSlice(existing.Object, "spec", "rules")
		if err != nil {
			return fmt.Errorf("read existing rules: %w", err)
		}
		desired := upsertRule(rules, trafficDefaultRule(inferenceServerName))
		if equalRules(rules, desired) {
			return nil
		}
		if err := unstructured.SetNestedSlice(existing.Object, desired, "spec", "rules"); err != nil {
			return fmt.Errorf("set rules on traffic route: %w", err)
		}
		if _, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update traffic route in cluster %s: %w", clusterID, err)
		}
		return nil
	})
}

// DeleteTrafficRoute removes the traffic HTTPRoute from the target cluster.
// Tolerates not-found.
func (p *defaultRouteProvider) DeleteTrafficRoute(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string) error {
	if err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Delete(ctx, trafficRouteName(inferenceServerName), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete traffic route in cluster %s: %w", clusterID, err)
	}
	return nil
}

// TrafficRouteExists reports whether the traffic HTTPRoute object has been
// provisioned for the InferenceServer in the target cluster.
func (p *defaultRouteProvider) TrafficRouteExists(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string) (bool, error) {
	_, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, trafficRouteName(inferenceServerName), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get traffic route in cluster %s: %w", clusterID, err)
	}
	return true, nil
}

// UpsertDiscoveryRule adds or updates the rule that exposes the deployment's
// model on the discovery HTTPRoute.
func (p *defaultRouteProvider) UpsertDiscoveryRule(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string, deploymentName string, modelName string) error {
	unlock := p.locks.Lock(discoveryLockKey(namespace, inferenceServerName))
	defer unlock()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		name := discoveryRouteName(inferenceServerName)
		existing, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get discovery route: %w", err)
		}
		rules, _, err := unstructured.NestedSlice(existing.Object, "spec", "rules")
		if err != nil {
			return fmt.Errorf("read existing rules: %w", err)
		}
		desired := upsertRule(rules, discoveryDeploymentRule(inferenceServerName, deploymentName))
		if equalRules(rules, desired) {
			return nil
		}
		if err := unstructured.SetNestedSlice(existing.Object, desired, "spec", "rules"); err != nil {
			return fmt.Errorf("set rules on discovery route: %w", err)
		}
		if _, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update discovery route: %w", err)
		}
		return nil
	})
}

// RemoveDiscoveryRule removes the rule for the deployment from the discovery
// HTTPRoute. Tolerates a missing rule.
func (p *defaultRouteProvider) RemoveDiscoveryRule(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string, deploymentName string) error {
	unlock := p.locks.Lock(discoveryLockKey(namespace, inferenceServerName))
	defer unlock()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		name := discoveryRouteName(inferenceServerName)
		existing, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get discovery route: %w", err)
		}
		rules, _, err := unstructured.NestedSlice(existing.Object, "spec", "rules")
		if err != nil {
			return fmt.Errorf("read existing rules: %w", err)
		}
		desired := removeRuleByPath(rules, discoveryRulePath(inferenceServerName, deploymentName))
		if equalRules(rules, desired) {
			return nil
		}
		if err := unstructured.SetNestedSlice(existing.Object, desired, "spec", "rules"); err != nil {
			return fmt.Errorf("set rules on discovery route: %w", err)
		}
		if _, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update discovery route: %w", err)
		}
		return nil
	})
}

// DeploymentDiscoveryRouteExists reports whether the discovery routing for the
// deployment is configured.
func (p *defaultRouteProvider) DeploymentDiscoveryRouteExists(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string, deploymentName string) (bool, error) {
	existing, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, discoveryRouteName(inferenceServerName), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get discovery route: %w", err)
	}
	rules, _, err := unstructured.NestedSlice(existing.Object, "spec", "rules")
	if err != nil {
		return false, fmt.Errorf("read existing rules: %w", err)
	}
	return findRuleByPath(rules, discoveryRulePath(inferenceServerName, deploymentName)) >= 0, nil
}

// UpsertTrafficRule adds or updates the rule on the traffic HTTPRoute that
// forwards the deployment's requests to its model on the local inference
// Service.
func (p *defaultRouteProvider) UpsertTrafficRule(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string, deploymentName string, modelName string) error {
	unlock := p.locks.Lock(trafficLockKey(namespace, inferenceServerName, clusterID))
	defer unlock()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		name := trafficRouteName(inferenceServerName)
		existing, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get traffic route in cluster %s: %w", clusterID, err)
		}
		rules, _, err := unstructured.NestedSlice(existing.Object, "spec", "rules")
		if err != nil {
			return fmt.Errorf("read existing rules: %w", err)
		}
		desired := upsertRule(rules, trafficDeploymentRule(inferenceServerName, deploymentName, modelName))
		if equalRules(rules, desired) {
			return nil
		}
		if err := unstructured.SetNestedSlice(existing.Object, desired, "spec", "rules"); err != nil {
			return fmt.Errorf("set rules on traffic route: %w", err)
		}
		if _, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update traffic route in cluster %s: %w", clusterID, err)
		}
		return nil
	})
}

// RemoveTrafficRule removes the rule for the deployment from the traffic
// HTTPRoute. Tolerates a missing route or a missing rule.
func (p *defaultRouteProvider) RemoveTrafficRule(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string, deploymentName string) error {
	unlock := p.locks.Lock(trafficLockKey(namespace, inferenceServerName, clusterID))
	defer unlock()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		name := trafficRouteName(inferenceServerName)
		existing, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get traffic route in cluster %s: %w", clusterID, err)
		}
		rules, _, err := unstructured.NestedSlice(existing.Object, "spec", "rules")
		if err != nil {
			return fmt.Errorf("read existing rules: %w", err)
		}
		desired := removeRuleByPath(rules, trafficRulePath(inferenceServerName, deploymentName))
		if equalRules(rules, desired) {
			return nil
		}
		if err := unstructured.SetNestedSlice(existing.Object, desired, "spec", "rules"); err != nil {
			return fmt.Errorf("set rules on traffic route: %w", err)
		}
		if _, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update traffic route in cluster %s: %w", clusterID, err)
		}
		return nil
	})
}

// DeploymentTrafficRouteExists reports whether the deployment's traffic rule
// is present and equal to the rule for the given model. False indicates the
// rule is missing or its filter targets a different model, so the rollout
// actor must reapply.
func (p *defaultRouteProvider) DeploymentTrafficRouteExists(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string, deploymentName string, modelName string) (bool, error) {
	existing, err := dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, trafficRouteName(inferenceServerName), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get traffic route in cluster %s: %w", clusterID, err)
	}
	rules, _, err := unstructured.NestedSlice(existing.Object, "spec", "rules")
	if err != nil {
		return false, fmt.Errorf("read existing rules: %w", err)
	}
	i := findRuleByPath(rules, trafficRulePath(inferenceServerName, deploymentName))
	if i < 0 {
		return false, nil
	}
	current, ok := rules[i].(map[string]interface{})
	if !ok {
		return false, nil
	}
	return mapsEqual(current, trafficDeploymentRule(inferenceServerName, deploymentName, modelName)), nil
}

// newHTTPRoute returns an empty unstructured HTTPRoute with apiVersion and kind
// set, ready for Get/Create/Update via dynamic.Interface.
func newHTTPRoute() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": httpRouteAPIVersion,
			"kind":       httpRouteKind,
		},
	}
}

// newDiscoveryRoute builds a fresh discovery HTTPRoute with the default rule
// installed. The InferenceServer's owner reference is set by the caller after
// construction.
func newDiscoveryRoute(server *v2pb.InferenceServer) *unstructured.Unstructured {
	route := newHTTPRoute()
	route.SetName(discoveryRouteName(server.Name))
	route.SetNamespace(server.Namespace)
	_ = unstructured.SetNestedMap(route.Object, map[string]interface{}{
		"parentRefs": []interface{}{parentRef(server.Namespace)},
		"rules":      []interface{}{discoveryDefaultRule(server.Name)},
	}, "spec")
	return route
}

// newTrafficRoute builds a fresh traffic HTTPRoute with the default catch-all
// rule installed.
func newTrafficRoute(inferenceServerName, namespace string) *unstructured.Unstructured {
	route := newHTTPRoute()
	route.SetName(trafficRouteName(inferenceServerName))
	route.SetNamespace(namespace)
	_ = unstructured.SetNestedMap(route.Object, map[string]interface{}{
		"parentRefs": []interface{}{parentRef(namespace)},
		"rules":      []interface{}{trafficDefaultRule(inferenceServerName)},
	}, "spec")
	return route
}

// setInferenceServerOwner stamps the InferenceServer as the controller owner
// on the route so the kube garbage collector deletes the route when the
// InferenceServer is removed.
func setInferenceServerOwner(route *unstructured.Unstructured, server *v2pb.InferenceServer) {
	t := true
	route.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion:         v2pb.GroupVersion.String(),
		Kind:               inferenceServerKind,
		Name:               server.Name,
		UID:                server.UID,
		Controller:         &t,
		BlockOwnerDeletion: &t,
	}})
}

func parentRef(namespace string) map[string]interface{} {
	return map[string]interface{}{
		"group":     gatewayAPIGroup,
		"kind":      gatewayKind,
		"name":      gatewayName,
		"namespace": namespace,
	}
}

// discoveryDefaultRule matches /{inferenceServerName} exactly and rewrites it
// to /cluster/{inferenceServerName} so a bare `curl /{inferenceServerName}`
// reaches the discovery Service. The traffic route in the receiving cluster
// then rewrites /cluster/{inferenceServerName} to Triton's /v2 metadata path.
func discoveryDefaultRule(inferenceServerName string) map[string]interface{} {
	return map[string]interface{}{
		"matches": []interface{}{
			pathMatch(pathTypeExact, discoveryDefaultPath(inferenceServerName)),
		},
		"filters": []interface{}{
			urlRewriteFilter(rewritePathReplaceFullPath, discoveryDefaultRewrite(inferenceServerName)),
		},
		"backendRefs": []interface{}{
			serviceBackendRef(endpointsServiceName(inferenceServerName)),
		},
	}
}

// discoveryDeploymentRule matches /{inferenceServerName}/{deploymentName} and
// rewrites it to /cluster/{inferenceServerName}/{deploymentName} before
// forwarding to the discovery Service. The model-name rewrite happens at the
// traffic route in the receiving cluster, where each Deployment owns its own
// rule keyed by deployment name.
func discoveryDeploymentRule(inferenceServerName, deploymentName string) map[string]interface{} {
	return map[string]interface{}{
		"matches": []interface{}{
			pathMatch(pathTypePathPrefix, discoveryRulePath(inferenceServerName, deploymentName)),
		},
		"filters": []interface{}{
			urlRewriteFilter(rewritePathReplacePrefix, discoveryRuleRewrite(inferenceServerName, deploymentName)),
		},
		"backendRefs": []interface{}{
			serviceBackendRef(endpointsServiceName(inferenceServerName)),
		},
	}
}

// trafficDefaultRule matches /cluster/{inferenceServerName} exactly and rewrites
// it to /v2 so the bare-IS request returns Triton's server metadata.
func trafficDefaultRule(inferenceServerName string) map[string]interface{} {
	return map[string]interface{}{
		"matches": []interface{}{
			pathMatch(pathTypeExact, trafficDefaultPath(inferenceServerName)),
		},
		"filters": []interface{}{
			urlRewriteFilter(rewritePathReplaceFullPath, tritonAPIPrefix),
		},
		"backendRefs": []interface{}{
			serviceBackendRef(inferenceServiceName(inferenceServerName)),
		},
	}
}

// trafficDeploymentRule matches /cluster/{inferenceServerName}/{deploymentName}
// and rewrites it to /v2/models/{modelName} so requests reach the deployment's
// model on the local Triton.
func trafficDeploymentRule(inferenceServerName, deploymentName, modelName string) map[string]interface{} {
	return map[string]interface{}{
		"matches": []interface{}{
			pathMatch(pathTypePathPrefix, trafficRulePath(inferenceServerName, deploymentName)),
		},
		"filters": []interface{}{
			urlRewriteFilter(rewritePathReplacePrefix, trafficRuleRewrite(modelName)),
		},
		"backendRefs": []interface{}{
			serviceBackendRef(inferenceServiceName(inferenceServerName)),
		},
	}
}

func pathMatch(pathType, value string) map[string]interface{} {
	return map[string]interface{}{
		"path": map[string]interface{}{
			"type":  pathType,
			"value": value,
		},
	}
}

func urlRewriteFilter(rewriteType, value string) map[string]interface{} {
	rewrite := map[string]interface{}{"type": rewriteType}
	switch rewriteType {
	case rewritePathReplaceFullPath:
		rewrite["replaceFullPath"] = value
	case rewritePathReplacePrefix:
		rewrite["replacePrefixMatch"] = value
	}
	return map[string]interface{}{
		"type": filterTypeURLRewrite,
		"urlRewrite": map[string]interface{}{
			"path": rewrite,
		},
	}
}

func serviceBackendRef(serviceName string) map[string]interface{} {
	return map[string]interface{}{
		"group":  "",
		"kind":   "Service",
		"name":   serviceName,
		"port":   servicePort,
		"weight": backendRefWeight,
	}
}

// upsertRule returns a copy of `rules` with `rule` inserted or updated in
// place. Identity is determined by the rule's first path match value.
func upsertRule(rules []interface{}, rule map[string]interface{}) []interface{} {
	target := rulePath(rule)
	out := make([]interface{}, len(rules))
	copy(out, rules)
	if i := findRuleByPath(out, target); i >= 0 {
		out[i] = rule
		return out
	}
	return append(out, rule)
}

// removeRuleByPath returns a copy of `rules` with any rule matching `path`
// removed.
func removeRuleByPath(rules []interface{}, path string) []interface{} {
	out := make([]interface{}, 0, len(rules))
	for _, r := range rules {
		if rulePath(r) == path {
			continue
		}
		out = append(out, r)
	}
	return out
}

// findRuleByPath returns the index of the first rule whose first path match
// equals `path`, or -1 if none found.
func findRuleByPath(rules []interface{}, path string) int {
	for i, r := range rules {
		if rulePath(r) == path {
			return i
		}
	}
	return -1
}

// rulePath extracts the first path match value from an HTTPRouteRule, returning
// "" if absent or malformed.
func rulePath(rule interface{}) string {
	m, ok := rule.(map[string]interface{})
	if !ok {
		return ""
	}
	matches, _, err := unstructured.NestedSlice(m, "matches")
	if err != nil || len(matches) == 0 {
		return ""
	}
	first, ok := matches[0].(map[string]interface{})
	if !ok {
		return ""
	}
	value, _, _ := unstructured.NestedString(first, "path", "value")
	return value
}

// equalRules returns true if a and b are deeply equal slices of rules.
func equalRules(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !mapsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func mapsEqual(a, b interface{}) bool {
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
				if !mapsEqual(asl[i], bsl[i]) {
					return false
				}
			}
			continue
		}
		if !mapsEqual(av, bv) {
			return false
		}
	}
	return true
}

func discoveryRouteName(inferenceServerName string) string {
	return inferenceServerName + discoveryRouteSuffix
}

func trafficRouteName(inferenceServerName string) string {
	return inferenceServerName + trafficRouteSuffix
}

func endpointsServiceName(inferenceServerName string) string {
	return inferenceServerName + endpointsServiceSuffix
}

func inferenceServiceName(inferenceServerName string) string {
	return inferenceServerName + inferenceServiceSuffix
}

func discoveryLockKey(namespace, inferenceServerName string) string {
	return namespace + "/" + inferenceServerName + discoveryRouteSuffix
}

func trafficLockKey(namespace, inferenceServerName, clusterID string) string {
	return namespace + "/" + inferenceServerName + trafficRouteSuffix + "/" + clusterID
}

func discoveryRulePath(inferenceServerName, deploymentName string) string {
	return "/" + inferenceServerName + "/" + deploymentName
}

func discoveryRuleRewrite(inferenceServerName, deploymentName string) string {
	return clusterPathPrefix + "/" + inferenceServerName + "/" + deploymentName
}

func discoveryDefaultPath(inferenceServerName string) string {
	return "/" + inferenceServerName
}

func discoveryDefaultRewrite(inferenceServerName string) string {
	return clusterPathPrefix + "/" + inferenceServerName
}

func trafficRulePath(inferenceServerName, deploymentName string) string {
	return clusterPathPrefix + "/" + inferenceServerName + "/" + deploymentName
}

func trafficRuleRewrite(modelName string) string {
	return tritonModelPathPrefix + "/" + modelName
}

func trafficDefaultPath(inferenceServerName string) string {
	return clusterPathPrefix + "/" + inferenceServerName
}
