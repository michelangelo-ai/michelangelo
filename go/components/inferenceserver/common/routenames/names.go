// Package routenames provides the shared naming contract between the
// InferenceServer and Deployment controllers for HTTPRoute objects.
// The InferenceServer controller creates routes with these names; the
// Deployment controller references them by the same names.
package routenames

const (
	discoveryRouteSuffix = "-discovery"
	trafficRouteSuffix   = "-traffic"
)

// DiscoveryRouteName returns the name of the control-plane discovery HTTPRoute
// for the given InferenceServer.
func DiscoveryRouteName(inferenceServerName string) string {
	return inferenceServerName + discoveryRouteSuffix
}

// TrafficRouteName returns the name of the per-cluster traffic HTTPRoute for
// the given InferenceServer.
func TrafficRouteName(inferenceServerName string) string {
	return inferenceServerName + trafficRouteSuffix
}

// DiscoveryMatchPath returns the per-deployment match path on the control-plane
// discovery HTTPRoute. It matches /{isName}/{deploymentName} and prefixes for
// all sub-paths of that deployment.
func DiscoveryMatchPath(isName, deploymentName string) string {
	return "/" + isName + "/" + deploymentName
}

// DiscoveryRewritePath returns the rewrite target on the discovery HTTPRoute,
// pointing to the per-cluster traffic path for the same deployment.
func DiscoveryRewritePath(isName, deploymentName string) string {
	return "/cluster/" + isName + "/" + deploymentName
}

// TrafficMatchPath returns the per-deployment match path on the per-cluster
// traffic HTTPRoute. It matches /cluster/{isName}/{deploymentName}.
func TrafficMatchPath(isName, deploymentName string) string {
	return "/cluster/" + isName + "/" + deploymentName
}

// TrafficRewritePath returns the Triton model path that a traffic rule rewrites
// to: /v2/models/{modelName}.
func TrafficRewritePath(modelName string) string {
	return "/v2/models/" + modelName
}
