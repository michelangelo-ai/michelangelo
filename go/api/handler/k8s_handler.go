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

// CreateInK8s implements K8sHandler.CreateInK8s by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) CreateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.CreateOptions) error {
	return k.client.Create(ctx, obj, &ctrlRTClient.CreateOptions{
		DryRun:       opts.DryRun,
		FieldManager: opts.FieldManager,
		Raw:          opts,
	})
}

// GetFromK8s implements K8sHandler.GetFromK8s by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) GetFromK8s(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	return k.client.Get(ctx, ctrlRTClient.ObjectKey{Namespace: namespace, Name: name}, obj)
}

// UpdateInK8s implements K8sHandler.UpdateInK8s by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) UpdateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	return k.client.Update(ctx, obj, &ctrlRTClient.UpdateOptions{
		DryRun:       opts.DryRun,
		FieldManager: opts.FieldManager,
		Raw:          opts,
	})
}

// UpdateStatusInK8s implements K8sHandler.UpdateStatusInK8s by delegating to the status writer.
func (k *K8sHandlerImpl) UpdateStatusInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	return k.client.Status().Update(ctx, obj, &ctrlRTClient.UpdateOptions{
		DryRun:       opts.DryRun,
		FieldManager: opts.FieldManager,
		Raw:          opts,
	})
}

// DeleteFromK8s implements K8sHandler.DeleteFromK8s by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) DeleteFromK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.DeleteOptions) error {
	return k.client.Delete(ctx, obj, &ctrlRTClient.DeleteOptions{
		DryRun:             opts.DryRun,
		Preconditions:      opts.Preconditions,
		PropagationPolicy:  opts.PropagationPolicy,
		GracePeriodSeconds: opts.GracePeriodSeconds,
		Raw:                opts,
	})
}

// ListFromK8s implements K8sHandler.ListFromK8s by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) ListFromK8s(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error {
	parsedListOptions, err := getCRTListOptions(namespace, opts)
	if err != nil {
		return err
	}
	return k.client.List(ctx, list, parsedListOptions)
}

// DeleteCollectionFromK8s implements K8sHandler.DeleteCollectionFromK8s by delegating to the controller-runtime client.
func (k *K8sHandlerImpl) DeleteCollectionFromK8s(ctx context.Context, objType ctrlRTClient.Object, namespace string, deleteOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error {
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