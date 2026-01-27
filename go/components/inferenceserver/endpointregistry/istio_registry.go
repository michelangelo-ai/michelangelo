package endpointregistry

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	// Istio ServiceEntry constants
	istioNetworkingGroup   = "networking.istio.io"
	istioNetworkingVersion = "v1"
	serviceEntryResource   = "serviceentries"

	// Label keys (control-plane + target-cluster discovery)
	labelInferenceServer = "michelangelo.ai/inference-server"
	labelClusterID       = "michelangelo.ai/cluster-id"
	labelComponent       = "app.kubernetes.io/component"
	labelManagedBy       = "app.kubernetes.io/managed-by"

	// Label on the target cluster's Service that represents the east-west gateway.
	// Sandbox setup should own this Service and apply these labels for discovery.
	labelEastWestGateway = "michelangelo.ai/east-west-gateway"

	// Control-plane object names
	globalServiceEntryName = "ma-inference-endpoints"

	// Default logical ports exposed by the upstream
	defaultHTTPPort = 80
	defaultGRPCPort = 8001

	// Default actual gateway port if we can't infer one from the Service.
	defaultEastWestGatewayPort = 15443
)

var serviceEntryGVR = schema.GroupVersionResource{
	Group:    istioNetworkingGroup,
	Version:  istioNetworkingVersion,
	Resource: serviceEntryResource,
}

var _ EndpointRegistry = &istioEndpointRegistry{}

// istioEndpointRegistry implements EndpointRegistry using:
// - a single control-plane ServiceEntry (hosts = all inference-server hosts; endpoints = per-cluster east-west gateways)
// - one control-plane ExternalName Service per inference server (bridge for HTTPRoute backendRefs)
type istioEndpointRegistry struct {
	dynamicClient dynamic.Interface
	kubeClient    client.Client
	clientFactory clientfactory.ClientFactory
	logger        *zap.Logger
}

func NewIstioEndpointRegistry(dynamicClient dynamic.Interface, kubeClient client.Client, clientFactory clientfactory.ClientFactory, logger *zap.Logger) EndpointRegistry {
	return &istioEndpointRegistry{
		dynamicClient: dynamicClient,
		kubeClient:    kubeClient,
		clientFactory: clientFactory,
		logger:        logger,
	}
}

// EnsureRegisteredEndpoint registers a remote cluster's inference server endpoint in the control plane.
// It creates/updates two resources:
// 1. A global ServiceEntry that maps the inference server hostname to the cluster's east-west gateway
// 2. An ExternalName Service that bridges HTTPRoute traffic to the ServiceEntry host
func (r *istioEndpointRegistry) EnsureRegisteredEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint, targetCluster *v2pb.ClusterTarget) error {
	resolved, err := r.resolveEndpointFromTargetCluster(ctx, endpoint, targetCluster)
	if err != nil {
		return err
	}

	// ServiceEntry must exist before ExternalName bridge so Istio knows how to route the hostname.
	if err := r.ensureGlobalServiceEntry(ctx, logger, resolved); err != nil {
		return err
	}

	return r.ensureBridgeService(ctx, logger, resolved.InferenceServerName, resolved.Namespace)
}

// DeleteRegisteredEndpoint removes a cluster's endpoint from the global ServiceEntry.
// It removes the endpoint entry for the given clusterID and cleans up the host if no
// other clusters are serving that inference server.
func (r *istioEndpointRegistry) DeleteRegisteredEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) error {
	se, err := r.getGlobalServiceEntry(ctx, namespace)
	if err != nil {
		return err
	}
	if se == nil {
		return nil
	}

	endpoints, _, _ := unstructured.NestedSlice(se.Object, "spec", "endpoints")
	newEndpoints := make([]interface{}, 0, len(endpoints))
	for _, e := range endpoints {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		labels, _, _ := unstructured.NestedStringMap(m, "labels")
		if labels[labelClusterID] == clusterID {
			continue
		}
		newEndpoints = append(newEndpoints, m)
	}
	if err := unstructured.SetNestedSlice(se.Object, newEndpoints, "spec", "endpoints"); err != nil {
		return fmt.Errorf("failed to set ServiceEntry endpoints: %w", err)
	}

	logger.Info("Updating global ServiceEntry to remove cluster endpoint",
		zap.String("serviceEntry", globalServiceEntryName),
		zap.String("namespace", namespace),
		zap.String("clusterID", clusterID),
	)
	if _, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(namespace).Update(ctx, se, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update global ServiceEntry: %w", err)
	}
	return nil
}

func (r *istioEndpointRegistry) ListRegisteredEndpoints(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) ([]ClusterEndpoint, error) {
	se, err := r.getGlobalServiceEntry(ctx, namespace)
	if err != nil {
		return nil, err
	}
	if se == nil {
		return []ClusterEndpoint{}, nil
	}

	wantHost := generateInferenceServerHost(inferenceServerName, namespace)
	hosts, _, _ := unstructured.NestedStringSlice(se.Object, "spec", "hosts")
	hostExists := false
	for _, h := range hosts {
		if h == wantHost {
			hostExists = true
			break
		}
	}
	if !hostExists {
		return []ClusterEndpoint{}, nil
	}

	endpoints, _, _ := unstructured.NestedSlice(se.Object, "spec", "endpoints")
	out := make([]ClusterEndpoint, 0, len(endpoints))
	for _, e := range endpoints {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}

		addr, _, _ := unstructured.NestedString(m, "address")
		lbls, _, _ := unstructured.NestedStringMap(m, "labels")
		portsAny, _, _ := unstructured.NestedMap(m, "ports")

		ports := map[string]uint32{}
		for k, v := range portsAny {
			switch vv := v.(type) {
			case int64:
				ports[k] = uint32(vv)
			case float64:
				ports[k] = uint32(vv)
			}
		}

		out = append(out, ClusterEndpoint{
			ClusterID:           lbls[labelClusterID],
			InferenceServerName: inferenceServerName,
			Namespace:           namespace,
			Address:             addr,
			Ports:               ports,
		})
	}
	return out, nil
}

func (r *istioEndpointRegistry) GetControlPlaneServiceName(inferenceServerName string) string {
	return generateControlPlaneServiceName(inferenceServerName)
}

// ensureBridgeService creates an ExternalName Service that acts as a bridge between
// Gateway API HTTPRoutes and Istio's ServiceEntry. HTTPRoutes require a Kubernetes Service
// as a backend target, but ServiceEntry hosts are not directly referenceable. This bridge
// Service resolves to the inference server's ServiceEntry hostname, allowing Istio to
// route traffic to the appropriate remote cluster via the east-west gateway.
func (r *istioEndpointRegistry) ensureBridgeService(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) error {
	serviceName := generateControlPlaneServiceName(inferenceServerName)
	externalName := generateInferenceServerHost(inferenceServerName, namespace)

	existing := &corev1.Service{}
	err := r.kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, existing)
	if err == nil {
		needsUpdate := existing.Spec.Type != corev1.ServiceTypeExternalName || existing.Spec.ExternalName != externalName
		if !needsUpdate {
			return nil
		}
		existing.Spec.Type = corev1.ServiceTypeExternalName
		existing.Spec.ExternalName = externalName
		return r.kubeClient.Update(ctx, existing)
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get bridge Service %s/%s: %w", namespace, serviceName, err)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels: map[string]string{
				labelInferenceServer: inferenceServerName,
				labelComponent:       "endpoint-registry",
				labelManagedBy:       "michelangelo",
			},
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: externalName,
		},
	}
	logger.Info("Creating bridge ExternalName Service",
		zap.String("service", serviceName),
		zap.String("namespace", namespace),
		zap.String("externalName", externalName),
	)
	if err := r.kubeClient.Create(ctx, svc); err != nil {
		return fmt.Errorf("failed to create bridge Service %s/%s: %w", namespace, serviceName, err)
	}
	return nil
}

// ensureGlobalServiceEntry creates or updates the shared ServiceEntry that aggregates all
// inference server endpoints across clusters. The ServiceEntry maps logical hostnames
// (e.g., inference-server.default.mesh.internal) to physical endpoints (east-west gateway
// addresses). Each cluster's endpoint is identified by its clusterID label, enabling
// Istio to route mesh traffic to the correct remote cluster.
func (r *istioEndpointRegistry) ensureGlobalServiceEntry(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	se, err := r.getGlobalServiceEntry(ctx, endpoint.Namespace)
	if err != nil {
		return err
	}

	if se == nil {
		se = r.newGlobalServiceEntry(endpoint.Namespace)
	}

	// Hosts = union of all inference-server hosts.
	hosts, _, _ := unstructured.NestedStringSlice(se.Object, "spec", "hosts")
	wantHost := generateInferenceServerHost(endpoint.InferenceServerName, endpoint.Namespace)
	if !containsString(hosts, wantHost) {
		hosts = append(hosts, wantHost)
	}
	if err := unstructured.SetNestedStringSlice(se.Object, hosts, "spec", "hosts"); err != nil {
		return fmt.Errorf("failed to set ServiceEntry hosts: %w", err)
	}

	// Endpoints = one per cluster (east-west gateways), shared across all hosts.
	endpoints, _, _ := unstructured.NestedSlice(se.Object, "spec", "endpoints")
	newEndpoints := make([]interface{}, 0, len(endpoints)+1)
	replaced := false
	for _, e := range endpoints {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		lbls, _, _ := unstructured.NestedStringMap(m, "labels")
		if lbls[labelClusterID] == endpoint.ClusterID {
			newEndpoints = append(newEndpoints, r.serviceEntryEndpoint(endpoint))
			replaced = true
		} else {
			newEndpoints = append(newEndpoints, m)
		}
	}
	if !replaced {
		newEndpoints = append(newEndpoints, r.serviceEntryEndpoint(endpoint))
	}
	if err := unstructured.SetNestedSlice(se.Object, newEndpoints, "spec", "endpoints"); err != nil {
		return fmt.Errorf("failed to set ServiceEntry endpoints: %w", err)
	}

	// Upsert.
	creationTimestamp := se.GetCreationTimestamp()
	if creationTimestamp.IsZero() {
		logger.Info("Creating global ServiceEntry",
			zap.String("serviceEntry", globalServiceEntryName),
			zap.String("namespace", endpoint.Namespace),
		)
		if _, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(endpoint.Namespace).Create(ctx, se, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create global ServiceEntry: %w", err)
		}
		return nil
	}

	if _, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(endpoint.Namespace).Update(ctx, se, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update global ServiceEntry: %w", err)
	}
	return nil
}

func (r *istioEndpointRegistry) getGlobalServiceEntry(ctx context.Context, namespace string) (*unstructured.Unstructured, error) {
	se, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(namespace).Get(ctx, globalServiceEntryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get global ServiceEntry %s/%s: %w", namespace, globalServiceEntryName, err)
	}
	return se, nil
}

func (r *istioEndpointRegistry) newGlobalServiceEntry(namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", istioNetworkingGroup, istioNetworkingVersion),
			"kind":       "ServiceEntry",
			"metadata": map[string]interface{}{
				"name":      globalServiceEntryName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					labelComponent: "endpoint-registry",
					labelManagedBy: "michelangelo",
				},
			},
			"spec": map[string]interface{}{
				"hosts":      []interface{}{},
				"location":   "MESH_EXTERNAL",
				"resolution": "STATIC",
				"ports": []interface{}{
					map[string]interface{}{"number": int64(defaultHTTPPort), "name": "http", "protocol": "HTTP"},
					map[string]interface{}{"number": int64(defaultGRPCPort), "name": "grpc", "protocol": "GRPC"},
				},
				"endpoints": []interface{}{},
			},
		},
	}
}

func (r *istioEndpointRegistry) serviceEntryEndpoint(endpoint ClusterEndpoint) map[string]interface{} {
	httpPort := endpoint.Ports["http"]
	if httpPort == 0 {
		httpPort = defaultEastWestGatewayPort
	}
	grpcPort := endpoint.Ports["grpc"]
	if grpcPort == 0 {
		grpcPort = defaultEastWestGatewayPort
	}
	return map[string]interface{}{
		"address": endpoint.Address,
		"labels": map[string]interface{}{
			labelClusterID: endpoint.ClusterID,
		},
		"ports": map[string]interface{}{
			"http": int64(httpPort),
			"grpc": int64(grpcPort),
		},
	}
}

func (r *istioEndpointRegistry) resolveEndpointFromTargetCluster(ctx context.Context, endpoint ClusterEndpoint, targetCluster *v2pb.ClusterTarget) (ClusterEndpoint, error) {
	if targetCluster == nil {
		return ClusterEndpoint{}, fmt.Errorf("targetCluster is required for cluster %s", endpoint.ClusterID)
	}
	switch targetCluster.GetConfig().(type) {
	case *v2pb.ClusterTarget_Kubernetes:
		// ok
	default:
		return ClusterEndpoint{}, fmt.Errorf("unsupported cluster type: %T", targetCluster.GetConfig())
	}

	clusterClient, err := r.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return ClusterEndpoint{}, fmt.Errorf("failed to get client for cluster %s: %w", endpoint.ClusterID, err)
	}

	// Discover the east-west gateway Service by label.
	svcs := &corev1.ServiceList{}
	if err := clusterClient.List(ctx, svcs, client.MatchingLabels{
		labelEastWestGateway: "true",
		labelClusterID:       endpoint.ClusterID,
	}); err != nil {
		return ClusterEndpoint{}, fmt.Errorf("failed to list east-west gateway services in cluster %s: %w", endpoint.ClusterID, err)
	}
	if len(svcs.Items) == 0 {
		return ClusterEndpoint{}, fmt.Errorf("no east-west gateway Service found in cluster %s (expected labels: %s=true, %s=%s)",
			endpoint.ClusterID, labelEastWestGateway, labelClusterID, endpoint.ClusterID)
	}

	svc := svcs.Items[0]

	var addr string
	var port uint32

	// For NodePort services, we need the node address + NodePort.
	// For LoadBalancer/ClusterIP, we use the service address + service port.
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		// Get a node address from the target cluster
		nodes := &corev1.NodeList{}
		if err := clusterClient.List(ctx, nodes); err != nil {
			return ClusterEndpoint{}, fmt.Errorf("failed to list nodes in cluster %s: %w", endpoint.ClusterID, err)
		}
		if len(nodes.Items) == 0 {
			return ClusterEndpoint{}, fmt.Errorf("no nodes found in cluster %s", endpoint.ClusterID)
		}
		addr = nodeAddress(&nodes.Items[0])
		port = serviceNodePort(&svc)
	} else {
		addr = serviceAddress(&svc)
		port = servicePort(&svc)
	}

	if addr == "" {
		return ClusterEndpoint{}, fmt.Errorf("east-west gateway Service %s/%s has no reachable address", svc.Namespace, svc.Name)
	}
	if port == 0 {
		port = defaultEastWestGatewayPort
	}

	endpoint.Address = addr
	if endpoint.Ports == nil {
		endpoint.Ports = map[string]uint32{}
	}
	// Both logical ports route via the gateway; the gateway routes to the right upstream.
	endpoint.Ports["http"] = port
	endpoint.Ports["grpc"] = port
	return endpoint, nil
}

func serviceAddress(svc *corev1.Service) string {
	if len(svc.Status.LoadBalancer.Ingress) > 0 {
		if svc.Status.LoadBalancer.Ingress[0].IP != "" {
			return svc.Status.LoadBalancer.Ingress[0].IP
		}
		if svc.Status.LoadBalancer.Ingress[0].Hostname != "" {
			return svc.Status.LoadBalancer.Ingress[0].Hostname
		}
	}
	if svc.Spec.ExternalName != "" {
		return svc.Spec.ExternalName
	}
	if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
		return svc.Spec.ClusterIP
	}
	return ""
}

func servicePort(svc *corev1.Service) uint32 {
	for _, p := range svc.Spec.Ports {
		switch p.Name {
		case "tls", "https", "istio":
			if p.Port > 0 {
				return uint32(p.Port)
			}
		}
	}
	if len(svc.Spec.Ports) > 0 && svc.Spec.Ports[0].Port > 0 {
		return uint32(svc.Spec.Ports[0].Port)
	}
	return 0
}

// serviceNodePort returns the NodePort for the east-west gateway service.
func serviceNodePort(svc *corev1.Service) uint32 {
	for _, p := range svc.Spec.Ports {
		switch p.Name {
		case "tls", "https", "istio":
			if p.NodePort > 0 {
				return uint32(p.NodePort)
			}
		}
	}
	if len(svc.Spec.Ports) > 0 && svc.Spec.Ports[0].NodePort > 0 {
		return uint32(svc.Spec.Ports[0].NodePort)
	}
	return 0
}

// nodeAddress returns a routable address for the node.
// Prefers InternalIP, then ExternalIP, then Hostname.
func nodeAddress(node *corev1.Node) string {
	var internalIP, externalIP, hostname string
	for _, addr := range node.Status.Addresses {
		switch addr.Type {
		case corev1.NodeInternalIP:
			internalIP = addr.Address
		case corev1.NodeExternalIP:
			externalIP = addr.Address
		case corev1.NodeHostName:
			hostname = addr.Address
		}
	}
	// For k3d, InternalIP is the Docker container IP, which is routable within the Docker network.
	if internalIP != "" {
		return internalIP
	}
	if externalIP != "" {
		return externalIP
	}
	return hostname
}

func generateInferenceServerHost(inferenceServerName, namespace string) string {
	return fmt.Sprintf("%s-inference-service.%s.svc.cluster.local", inferenceServerName, namespace)
}

func containsString(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func generateControlPlaneServiceName(inferenceServerName string) string {
	return fmt.Sprintf("%s-inference-bridge", inferenceServerName)
}
