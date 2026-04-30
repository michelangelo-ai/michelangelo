package endpoints

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Label keys on the published Service and EndpointSlices.
const (
	// kubeServiceNameLabel is the well-known kubernetes label that links an
	// EndpointSlice to its parent Service. Gateway controllers (Istio, Envoy
	// Gateway, GKE Gateway) read EndpointSlices keyed on this label when
	// resolving the Service's backends.
	kubeServiceNameLabel = "kubernetes.io/service-name"

	// clusterIDLabel records which ClusterTarget this EndpointSlice represents.
	// Used by the publisher to find slices for orphan deletion in Sync.
	clusterIDLabel = "michelangelo.ai/cluster-id"

	// portName is the named port shared between the parent Service and each
	// EndpointSlice. The Gateway controller resolves the Service's port number
	// to its name, then finds the matching name on the EndpointSlice and uses
	// the EndpointSlice's port (the actual cluster Gateway NodePort or
	// LoadBalancer port) as the upstream destination. The shared name is the
	// join key.
	portName = "http"

	// servicePort is the logical port published on the parent Service. It is
	// not the upstream destination, which comes from the EndpointSlice port.
	servicePort int32 = 80

	// endpointsServiceSuffix is appended to the InferenceServer name to form
	// the per-server Service name in the control plane.
	endpointsServiceSuffix = "-endpoints"
)

var _ EndpointPublisher = &defaultPublisher{}

// defaultPublisher implements EndpointPublisher by writing into the local
// Kubernetes API. The published surface for a server is one ClusterIP Service
// named "{is-name}-endpoints" (no selector) plus one EndpointSlice per
// cluster, named "{is-name}-endpoints-{cluster-id}" and labeled with the
// parent service name.
//
// Gateway API implementations (Istio, Envoy Gateway, GKE Gateway) resolve a
// Service backend by reading its EndpointSlices, so consumers can reference
// "{is-name}-endpoints" as a single Service even though traffic actually fans
// out across cluster gateways. The Service must be ClusterIP rather than
// headless: headless Services do not work as backends in Gateway API
// implementations that take the ClusterIP resolution path.
type defaultPublisher struct {
	kubeClient client.Client
	scheme     *runtime.Scheme
}

// NewDefaultPublisher returns an EndpointPublisher that targets the local
// (control-plane) cluster via the supplied client. The scheme is needed to
// stamp each published object with a Kubernetes owner reference back to the
// InferenceServer, so the kube garbage collector wipes the Service and
// EndpointSlices automatically when the InferenceServer is deleted.
func NewDefaultPublisher(kubeClient client.Client, scheme *runtime.Scheme) EndpointPublisher {
	return &defaultPublisher{kubeClient: kubeClient, scheme: scheme}
}

// Sync makes the control-plane Service and per-cluster EndpointSlices match
// `endpoints`. Idempotent.
func (p *defaultPublisher) Sync(ctx context.Context, server *v2pb.InferenceServer, endpoints map[string]Endpoint) error {
	if err := p.ensureService(ctx, server); err != nil {
		return fmt.Errorf("ensure service: %w", err)
	}
	for clusterID, ep := range endpoints {
		if err := p.upsertSlice(ctx, server, clusterID, ep); err != nil {
			return fmt.Errorf("upsert endpoint slice %q: %w", clusterID, err)
		}
	}
	// Delete orphan EndpointSlices: ones whose cluster_id label is no longer in
	// the desired endpoints map (cluster removed from spec).
	existing, err := p.listSlices(ctx, server)
	if err != nil {
		return fmt.Errorf("list existing slices: %w", err)
	}
	for _, slice := range existing.Items {
		clusterID := slice.Labels[clusterIDLabel]
		if _, keep := endpoints[clusterID]; keep {
			continue
		}
		if err := p.kubeClient.Delete(ctx, &slice); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete orphan slice %q: %w", slice.Name, err)
		}
	}
	return nil
}

// Get returns the currently published cluster ID to Endpoint map for `server`.
// Empty when nothing is published yet.
func (p *defaultPublisher) Get(ctx context.Context, server *v2pb.InferenceServer) (map[string]Endpoint, error) {
	slices, err := p.listSlices(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("list slices: %w", err)
	}
	out := make(map[string]Endpoint, len(slices.Items))
	for _, slice := range slices.Items {
		clusterID := slice.Labels[clusterIDLabel]
		if clusterID == "" || len(slice.Endpoints) == 0 || len(slice.Ports) == 0 {
			continue
		}
		ep := slice.Endpoints[0]
		port := slice.Ports[0]
		if len(ep.Addresses) == 0 || port.Port == nil {
			continue
		}
		out[clusterID] = Endpoint{
			Host:   ep.Addresses[0],
			Port:   *port.Port,
			Scheme: "http",
		}
	}
	return out, nil
}

// Delete removes the per-server Service and every EndpointSlice for `server`.
func (p *defaultPublisher) Delete(ctx context.Context, server *v2pb.InferenceServer) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName(server),
			Namespace: server.Namespace,
		},
	}
	if err := p.kubeClient.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete service: %w", err)
	}
	slices, err := p.listSlices(ctx, server)
	if err != nil {
		return fmt.Errorf("list slices: %w", err)
	}
	for _, slice := range slices.Items {
		if err := p.kubeClient.Delete(ctx, &slice); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete slice %q: %w", slice.Name, err)
		}
	}
	return nil
}

// ensureService creates the per-server discovery Service if it does not already exist in the control plane.
// The Service has no selector because its EndpointSlices are populated
// explicitly by upsertSlice. ClusterIP (rather than headless) is required so
// the Service works as an HTTPRoute backend in Gateway implementations that
// resolve via the Service ClusterIP.
func (p *defaultPublisher) ensureService(ctx context.Context, server *v2pb.InferenceServer) error {
	key := types.NamespacedName{Name: serviceName(server), Namespace: server.Namespace}
	existing := &corev1.Service{}
	err := p.kubeClient.Get(ctx, key, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get service: %w", err)
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name:       portName,
				Port:       servicePort,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromString(portName),
			}},
		},
	}
	if refErr := controllerutil.SetControllerReference(server, svc, p.scheme); refErr != nil {
		return fmt.Errorf("set owner reference on service: %w", refErr)
	}
	if createErr := p.kubeClient.Create(ctx, svc); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
		return fmt.Errorf("create service: %w", createErr)
	}
	return nil
}

// upsertSlice creates or updates the EndpointSlice for one cluster. The slice
// is named "{is-name}-endpoints-{cluster-id}" and labeled with the parent
// service name (consumed by Gateway controllers via EDS) and the cluster ID
// (consumed by the publisher itself for orphan detection).
func (p *defaultPublisher) upsertSlice(ctx context.Context, server *v2pb.InferenceServer, clusterID string, ep Endpoint) error {
	name := sliceName(server, clusterID)
	key := types.NamespacedName{Name: name, Namespace: server.Namespace}
	port := ep.Port
	desired := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
			Labels: map[string]string{
				kubeServiceNameLabel: serviceName(server),
				clusterIDLabel:       clusterID,
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{{
			Addresses: []string{ep.Host},
		}},
		Ports: []discoveryv1.EndpointPort{{
			Name:     ptr(portName),
			Port:     &port,
			Protocol: ptr(corev1.ProtocolTCP),
		}},
	}
	if err := controllerutil.SetControllerReference(server, desired, p.scheme); err != nil {
		return fmt.Errorf("set owner reference on slice: %w", err)
	}
	existing := &discoveryv1.EndpointSlice{}
	err := p.kubeClient.Get(ctx, key, existing)
	if apierrors.IsNotFound(err) {
		if createErr := p.kubeClient.Create(ctx, desired); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			return fmt.Errorf("create slice: %w", createErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get slice: %w", err)
	}
	existing.Labels = desired.Labels
	existing.AddressType = desired.AddressType
	existing.Endpoints = desired.Endpoints
	existing.Ports = desired.Ports
	if err := p.kubeClient.Update(ctx, existing); err != nil {
		return fmt.Errorf("update slice: %w", err)
	}
	return nil
}

// listSlices returns every EndpointSlice in the server's namespace whose
// kubernetes.io/service-name label matches the published Service. Used by
// Sync (orphan detection) and Get (drift check).
func (p *defaultPublisher) listSlices(ctx context.Context, server *v2pb.InferenceServer) (*discoveryv1.EndpointSliceList, error) {
	out := &discoveryv1.EndpointSliceList{}
	err := p.kubeClient.List(ctx, out,
		client.InNamespace(server.Namespace),
		client.MatchingLabels{kubeServiceNameLabel: serviceName(server)},
	)
	return out, err
}

func serviceName(server *v2pb.InferenceServer) string {
	return server.Name + endpointsServiceSuffix
}

func sliceName(server *v2pb.InferenceServer, clusterID string) string {
	return server.Name + endpointsServiceSuffix + "-" + clusterID
}

func ptr[T any](v T) *T { return &v }
