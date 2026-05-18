// Package gatewayapi implements routing.Manager for Gateway API HTTPRoutes.
package gatewayapi

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// httprouteGVR is the GroupVersionResource for Gateway API HTTPRoutes.
var httprouteGVR = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "httproutes",
}

// rulePath extracts the first path match value from an HTTPRouteRule.
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
