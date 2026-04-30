package endpoints

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testNamespace = "default"
	testServer    = "test-is"
)

func newFixture(t *testing.T, existing ...client.Object) (EndpointPublisher, client.Client, *v2pb.InferenceServer) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, discoveryv1.AddToScheme(scheme))
	require.NoError(t, v2pb.AddToScheme(scheme))

	server := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testServer,
			Namespace: testNamespace,
			UID:       "test-uid",
		},
	}
	objects := append([]client.Object{server}, existing...)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	return NewDefaultPublisher(c, scheme), c, server
}

// TestSync_CreatesServiceAndSlices covers the cold-start path: no existing
// objects, Sync should create the parent Service plus one EndpointSlice per
// cluster ID with the port-name join wired up correctly.
func TestSync_CreatesServiceAndSlices(t *testing.T) {
	pub, c, server := newFixture(t)
	endpoints := map[string]Endpoint{
		"clusterA": {Host: "10.0.0.1", Port: 31001, Scheme: "http"},
		"clusterB": {Host: "10.0.0.2", Port: 31002, Scheme: "http"},
	}

	require.NoError(t, pub.Sync(context.Background(), server, endpoints))

	svc := &corev1.Service{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-is-endpoints", Namespace: testNamespace}, svc))
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, portName, svc.Spec.Ports[0].Name, "service port name must match endpointslice port name (the gateway-controller join key)")
	assert.Equal(t, servicePort, svc.Spec.Ports[0].Port)
	assert.Empty(t, svc.Spec.Selector, "service must have no selector — its EndpointSlices are managed explicitly")

	slices := &discoveryv1.EndpointSliceList{}
	require.NoError(t, c.List(context.Background(), slices, client.MatchingLabels{kubeServiceNameLabel: "test-is-endpoints"}))
	require.Len(t, slices.Items, 2)

	for _, slice := range slices.Items {
		require.Len(t, slice.Ports, 1)
		assert.Equal(t, portName, *slice.Ports[0].Name, "endpointslice port name must match service port name")
		require.Len(t, slice.Endpoints, 1)
		require.Len(t, slice.Endpoints[0].Addresses, 1)
		clusterID := slice.Labels[clusterIDLabel]
		expected, ok := endpoints[clusterID]
		require.True(t, ok, "slice has unexpected cluster ID label %q", clusterID)
		assert.Equal(t, expected.Host, slice.Endpoints[0].Addresses[0])
		assert.Equal(t, expected.Port, *slice.Ports[0].Port)
	}
}

// TestSync_Idempotent covers the warm-state path: calling Sync twice with the
// same input should not create duplicate objects or error.
func TestSync_Idempotent(t *testing.T) {
	pub, c, server := newFixture(t)
	endpoints := map[string]Endpoint{
		"clusterA": {Host: "10.0.0.1", Port: 31001, Scheme: "http"},
	}
	require.NoError(t, pub.Sync(context.Background(), server, endpoints))
	require.NoError(t, pub.Sync(context.Background(), server, endpoints))

	svcs := &corev1.ServiceList{}
	require.NoError(t, c.List(context.Background(), svcs, client.InNamespace(testNamespace)))
	assert.Len(t, svcs.Items, 1)

	slices := &discoveryv1.EndpointSliceList{}
	require.NoError(t, c.List(context.Background(), slices, client.MatchingLabels{kubeServiceNameLabel: "test-is-endpoints"}))
	assert.Len(t, slices.Items, 1)
}

// TestSync_OrphanDeletion covers the convergence path: when a cluster ID is
// removed from the desired map, Sync must delete its stale EndpointSlice.
func TestSync_OrphanDeletion(t *testing.T) {
	pub, c, server := newFixture(t)
	require.NoError(t, pub.Sync(context.Background(), server, map[string]Endpoint{
		"clusterA": {Host: "10.0.0.1", Port: 31001},
		"clusterB": {Host: "10.0.0.2", Port: 31002},
	}))

	require.NoError(t, pub.Sync(context.Background(), server, map[string]Endpoint{
		"clusterA": {Host: "10.0.0.1", Port: 31001},
	}))

	slices := &discoveryv1.EndpointSliceList{}
	require.NoError(t, c.List(context.Background(), slices, client.MatchingLabels{kubeServiceNameLabel: "test-is-endpoints"}))
	require.Len(t, slices.Items, 1)
	assert.Equal(t, "clusterA", slices.Items[0].Labels[clusterIDLabel])
}

// TestSync_UpdatesExistingSlice covers spec drift: when an existing slice's
// host/port changes, Sync must update the slice rather than create a duplicate.
func TestSync_UpdatesExistingSlice(t *testing.T) {
	pub, c, server := newFixture(t)
	require.NoError(t, pub.Sync(context.Background(), server, map[string]Endpoint{
		"clusterA": {Host: "10.0.0.1", Port: 31001},
	}))
	require.NoError(t, pub.Sync(context.Background(), server, map[string]Endpoint{
		"clusterA": {Host: "10.0.0.99", Port: 31999},
	}))

	slice := &discoveryv1.EndpointSlice{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{
		Name:      "test-is-endpoints-clusterA",
		Namespace: testNamespace,
	}, slice))
	assert.Equal(t, "10.0.0.99", slice.Endpoints[0].Addresses[0])
	assert.Equal(t, int32(31999), *slice.Ports[0].Port)
}

// TestGet_ReturnsPublishedEndpoints covers the read path used by the actor's
// Retrieve to detect drift: Get must round-trip whatever Sync wrote.
func TestGet_ReturnsPublishedEndpoints(t *testing.T) {
	pub, _, server := newFixture(t)
	desired := map[string]Endpoint{
		"clusterA": {Host: "10.0.0.1", Port: 31001, Scheme: "http"},
		"clusterB": {Host: "10.0.0.2", Port: 31002, Scheme: "http"},
	}
	require.NoError(t, pub.Sync(context.Background(), server, desired))

	got, err := pub.Get(context.Background(), server)
	require.NoError(t, err)
	assert.Equal(t, desired, got)
}

// TestGet_EmptyWhenNoSlices covers the cold-start branch of Retrieve.
func TestGet_EmptyWhenNoSlices(t *testing.T) {
	pub, _, server := newFixture(t)
	got, err := pub.Get(context.Background(), server)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestGet_FiltersByServiceLabel ensures Get does not return EndpointSlices
// belonging to a different InferenceServer in the same namespace.
func TestGet_FiltersByServiceLabel(t *testing.T) {
	otherSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-is-endpoints-clusterX",
			Namespace: testNamespace,
			Labels: map[string]string{
				kubeServiceNameLabel: "other-is-endpoints",
				clusterIDLabel:       "clusterX",
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints:   []discoveryv1.Endpoint{{Addresses: []string{"10.99.0.1"}}},
		Ports:       []discoveryv1.EndpointPort{{Name: ptr(portName), Port: ptr(int32(99999))}},
	}
	pub, _, server := newFixture(t, otherSlice)
	require.NoError(t, pub.Sync(context.Background(), server, map[string]Endpoint{
		"clusterA": {Host: "10.0.0.1", Port: 31001, Scheme: "http"},
	}))

	got, err := pub.Get(context.Background(), server)
	require.NoError(t, err)
	assert.Equal(t, map[string]Endpoint{
		"clusterA": {Host: "10.0.0.1", Port: 31001, Scheme: "http"},
	}, got)
}

// TestDelete_RemovesServiceAndSlices covers the explicit-teardown path used
// when something other than IS deletion needs to wipe the published surface.
func TestDelete_RemovesServiceAndSlices(t *testing.T) {
	pub, c, server := newFixture(t)
	require.NoError(t, pub.Sync(context.Background(), server, map[string]Endpoint{
		"clusterA": {Host: "10.0.0.1", Port: 31001},
	}))

	require.NoError(t, pub.Delete(context.Background(), server))

	err := c.Get(context.Background(), types.NamespacedName{Name: "test-is-endpoints", Namespace: testNamespace}, &corev1.Service{})
	assert.True(t, apierrors.IsNotFound(err), "service should be gone after Delete; got %v", err)

	slices := &discoveryv1.EndpointSliceList{}
	require.NoError(t, c.List(context.Background(), slices, client.MatchingLabels{kubeServiceNameLabel: "test-is-endpoints"}))
	assert.Empty(t, slices.Items)
}
