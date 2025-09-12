package handler

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sHandlerImpl implements the K8sHandler interface by delegating to a controller-runtime client.
// This provides a clean abstraction layer for Kubernetes operations while maintaining
// full compatibility with the controller-runtime ecosystem.
type K8sHandlerImpl struct {
	client ctrlRTClient.Client
}

// NewK8sHandler creates a new K8sHandler implementation.
// The provided client should be properly configured with the appropriate scheme
// and authentication for the target Kubernetes cluster.
func NewK8sHandler(client ctrlRTClient.Client) K8sHandler {
	return &K8sHandlerImpl{client: client}
}

// Create implements K8sHandler.Create by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) Create(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.CreateOptions) error {
	return k.client.Create(ctx, obj, &ctrlRTClient.CreateOptions{
		DryRun:       opts.DryRun,
		FieldManager: opts.FieldManager,
		Raw:          opts,
	})
}

// Get implements K8sHandler.Get by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) Get(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	return k.client.Get(ctx, ctrlRTClient.ObjectKey{Namespace: namespace, Name: name}, obj)
}

// Update implements K8sHandler.Update by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) Update(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	return k.client.Update(ctx, obj, &ctrlRTClient.UpdateOptions{
		DryRun:       opts.DryRun,
		FieldManager: opts.FieldManager,
		Raw:          opts,
	})
}

// UpdateStatus implements K8sHandler.UpdateStatus by delegating to the status writer.
func (k *K8sHandlerImpl) UpdateStatus(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	return k.client.Status().Update(ctx, obj, &ctrlRTClient.UpdateOptions{
		DryRun:       opts.DryRun,
		FieldManager: opts.FieldManager,
		Raw:          opts,
	})
}

// Delete implements K8sHandler.Delete by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) Delete(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.DeleteOptions) error {
	return k.client.Delete(ctx, obj, &ctrlRTClient.DeleteOptions{
		DryRun:             opts.DryRun,
		Preconditions:      opts.Preconditions,
		PropagationPolicy:  opts.PropagationPolicy,
		GracePeriodSeconds: opts.GracePeriodSeconds,
		Raw:                opts,
	})
}

// List implements K8sHandler.List by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) List(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error {
	parsedListOptions, err := getCRTListOptions(namespace, opts)
	if err != nil {
		return err
	}
	return k.client.List(ctx, list, parsedListOptions)
}

// DeleteCollection implements K8sHandler.DeleteCollection by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) DeleteCollection(ctx context.Context, objType ctrlRTClient.Object, namespace string, deleteOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error {
	parsedListOptions, err := getCRTListOptions(namespace, listOpts)
	if err != nil {
		return err
	}

	return k.client.DeleteAllOf(ctx, objType, &ctrlRTClient.DeleteAllOfOptions{
		ListOptions: *parsedListOptions,
		DeleteOptions: ctrlRTClient.DeleteOptions{
			GracePeriodSeconds: deleteOpts.GracePeriodSeconds,
			Preconditions:      deleteOpts.Preconditions,
			PropagationPolicy:  deleteOpts.PropagationPolicy,
			Raw:                deleteOpts,
			DryRun:             deleteOpts.DryRun,
		},
	})
}
