package testutils

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FakeClientWrapper wraps a controller-runtime fake client to simulate Status().Update() behavior
type FakeClientWrapper struct {
	client.Client
}

// Status overrides the Status() method to return a custom FakeStatusWriter
func (f *FakeClientWrapper) Status() client.StatusWriter {
	return &FakeStatusWriter{Client: f.Client}
}

// FakeStatusWriter simulates the behavior of Status().Update() by directly modifying the status field
type FakeStatusWriter struct {
	client.Client
}

func (f *FakeStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return f.Client.Create(ctx, obj)
}

// Update overrides the Update method to simulate status updates
func (f *FakeStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	// Perform a regular update, since the fake client doesn't differentiate status subresources
	original := obj.DeepCopyObject()

	// Update the object in the fake client
	if err := f.Client.Update(ctx, obj); err != nil {
		return err
	}

	// Restore metadata like resource version to simulate status subresource update
	if existingObj, ok := original.(metav1.Object); ok {
		obj.SetResourceVersion(existingObj.GetResourceVersion())
	}

	return nil
}

// Patch simulates the Patch method for the status subresource
func (f *FakeStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return f.Client.Patch(ctx, obj, patch)
}

// NewFakeClientWrapper creates a new FakeClientWrapper with a given fake client
func NewFakeClientWrapper(fakeClient client.Client) *FakeClientWrapper {
	return &FakeClientWrapper{Client: fakeClient}
}
