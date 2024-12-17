package api

import (
	"context"
	"go.uber.org/yarpc"

	"k8s.io/apimachinery/pkg/runtime"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Handler defines an interface to handle the unified API objects with the underlying systems.
type Handler interface {
	// Create saves the API object obj into the unified API system.
	// Create API handler derives the name, namespace and object type from obj.
	// Returns nil if successful, otherwise a gRPC status error is returned.
	Create(ctx context.Context, obj client.Object, opts *metav1.CreateOptions) error

	// Get retrieves an API object by the given name and namespace from unified API system, and stores
	// that object into where the obj struct pointer pointed.
	// Get API handler derives the object type from obj.
	// Returns nil if successful, otherwise a gRPC status error is returned.
	Get(ctx context.Context, namespace string, name string, opts *metav1.GetOptions, obj client.Object) error

	// Update updates the given API object to the unified API system.
	// Update API handler derives the name, namespace and object type from obj.
	// Returns nil if successful, otherwise a gRPC status error is returned.
	Update(ctx context.Context, obj client.Object, opts *metav1.UpdateOptions) error

	// UpdateStatus updates Status of the given API object to the unified API system.
	// UpdateStatus API handler derives the name, namespace and object type from obj.
	// Returns nil if successful, otherwise a gRPC status error is returned.
	UpdateStatus(ctx context.Context, obj client.Object, opts *metav1.UpdateOptions) error

	// Delete deletes an API object specified by obj and opts.
	// Delete API handler derives name, namespace and object type from obj.
	// Returns nil if successful, otherwise a gRPC status error is returned.
	Delete(ctx context.Context, obj client.Object, opts *metav1.DeleteOptions) error

	// List retrieves a list of API objects in the given namespace specified by opts and listOptionsExt,
	// and stores the result into the list object pointed by list.
	// List API handler derives the object type from list.
	// Returns nil if successful, otherwise a gRPC status error is returned.
	List(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt,
		list client.ObjectList) error

	// DeleteCollection deletes a collection of API objects in the namespace specified by deleteOpts
	// and listOpts.
	// DeleteCollection API handler derives the object type from objType.
	// Returns nil if successful, otherwise a gRPC status error is returned.
	DeleteCollection(ctx context.Context, objType client.Object, namespace string,
		deleteOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error
}

// TypedAPIHandler defines an interface to handle a specific typed objects with the underlying systems.
type TypedAPIHandler[T runtime.Object, L client.ObjectList] interface {
	// Create saves the API object obj into the unified API system.
	Create(context.Context, T, *metav1.CreateOptions, ...yarpc.CallOption) error

	// Get retrieves an API object by the given name and namespace from unified API system
	Get(context.Context, string, string, *metav1.GetOptions, ...yarpc.CallOption) (T, error)

	// Update updates the given API object to the unified API system.
	Update(context.Context, T, *metav1.UpdateOptions, ...yarpc.CallOption) error

	// List retrieves a list of API objects in the given namespace specified by opts and listOptionsExt
	List(context.Context, string, *metav1.ListOptions, *apipb.ListOptionsExt, ...yarpc.CallOption) (L, error)

	// Delete deletes an API object specified by obj and opts.
	Delete(context.Context, T, *metav1.DeleteOptions, ...yarpc.CallOption) error

	// DeleteCollection deletes a collection of API objects in the namespace specified by deleteOpts and listOpts.
	DeleteCollection(context.Context, T, *metav1.DeleteOptions, *metav1.ListOptions, ...yarpc.CallOption) error
}
