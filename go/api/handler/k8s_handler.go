package handler

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sHandlerImpl implements K8sHandler interface.
// Focuses only on Kubernetes operations, following Kubernetes controller patterns.
type K8sHandlerImpl struct {
	client ctrlRTClient.Client
	logger logr.Logger
}

// NewK8sHandler creates a new K8sHandler implementation.
func NewK8sHandler(client ctrlRTClient.Client, logger logr.Logger) K8sHandler {
	return &K8sHandlerImpl{
		client: client,
		logger: logger.WithName("k8s-handler"),
	}
}

// CreateInK8s creates an object in Kubernetes cluster only.
func (k *K8sHandlerImpl) CreateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.CreateOptions) error {
	k.logger.V(1).Info("Creating object in K8s",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"kind", obj.GetObjectKind().GroupVersionKind().Kind,
	)

	createOpts := &ctrlRTClient.CreateOptions{}
	if opts != nil {
		createOpts.DryRun = opts.DryRun
		createOpts.FieldManager = opts.FieldManager
		createOpts.Raw = opts
	}

	err := k.client.Create(ctx, obj, createOpts)
	if err != nil {
		k.logger.Error(err, "Failed to create object in K8s",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	k.logger.V(1).Info("Successfully created object in K8s",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// GetFromK8s retrieves an object from Kubernetes cluster only.
func (k *K8sHandlerImpl) GetFromK8s(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	k.logger.V(1).Info("Getting object from K8s",
		"namespace", namespace,
		"name", name,
	)

	key := ctrlRTClient.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}

	err := k.client.Get(ctx, key, obj)
	if err != nil {
		k.logger.V(1).Info("Failed to get object from K8s",
			"namespace", namespace,
			"name", name,
			"error", err,
		)
		return err
	}

	k.logger.V(1).Info("Successfully retrieved object from K8s",
		"namespace", namespace,
		"name", name,
	)
	return nil
}

// UpdateInK8s updates an object in Kubernetes cluster only.
func (k *K8sHandlerImpl) UpdateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	k.logger.V(1).Info("Updating object in K8s",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	updateOpts := &ctrlRTClient.UpdateOptions{}
	if opts != nil {
		updateOpts.DryRun = opts.DryRun
		updateOpts.FieldManager = opts.FieldManager
		updateOpts.Raw = opts
	}

	err := k.client.Update(ctx, obj, updateOpts)
	if err != nil {
		k.logger.Error(err, "Failed to update object in K8s",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	k.logger.V(1).Info("Successfully updated object in K8s",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// UpdateStatusInK8s updates only the status of an object in Kubernetes.
func (k *K8sHandlerImpl) UpdateStatusInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	k.logger.V(1).Info("Updating object status in K8s",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	updateOpts := &ctrlRTClient.UpdateOptions{}
	if opts != nil {
		updateOpts.DryRun = opts.DryRun
		updateOpts.FieldManager = opts.FieldManager
		updateOpts.Raw = opts
	}

	err := k.client.Status().Update(ctx, obj, updateOpts)
	if err != nil {
		k.logger.Error(err, "Failed to update object status in K8s",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	k.logger.V(1).Info("Successfully updated object status in K8s",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// DeleteFromK8s deletes an object from Kubernetes cluster only.
func (k *K8sHandlerImpl) DeleteFromK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.DeleteOptions) error {
	k.logger.V(1).Info("Deleting object from K8s",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	deleteOpts := &ctrlRTClient.DeleteOptions{}
	if opts != nil {
		deleteOpts.GracePeriodSeconds = opts.GracePeriodSeconds
		deleteOpts.Preconditions = opts.Preconditions
		deleteOpts.PropagationPolicy = opts.PropagationPolicy
		deleteOpts.DryRun = opts.DryRun
		deleteOpts.Raw = opts
	}

	err := k.client.Delete(ctx, obj, deleteOpts)
	if err != nil {
		k.logger.Error(err, "Failed to delete object from K8s",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	k.logger.V(1).Info("Successfully deleted object from K8s",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// ListFromK8s lists objects from Kubernetes cluster only.
func (k *K8sHandlerImpl) ListFromK8s(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error {
	k.logger.V(1).Info("Listing objects from K8s",
		"namespace", namespace,
	)

	listOpts := &ctrlRTClient.ListOptions{}
	if namespace != "" {
		listOpts.Namespace = namespace
	}

	if opts != nil {
		if opts.LabelSelector != "" {
			selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
				MatchLabels: map[string]string{},
			})
			if err == nil {
				listOpts.LabelSelector = selector
			}
		}
		listOpts.Raw = opts
	}

	err := k.client.List(ctx, list, listOpts)
	if err != nil {
		k.logger.Error(err, "Failed to list objects from K8s",
			"namespace", namespace,
		)
		return err
	}

	k.logger.V(1).Info("Successfully listed objects from K8s",
		"namespace", namespace,
	)
	return nil
}

// DeleteCollectionFromK8s deletes a collection of objects from Kubernetes only.
func (k *K8sHandlerImpl) DeleteCollectionFromK8s(ctx context.Context, objType ctrlRTClient.Object, namespace string,
	deleteOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error {
	k.logger.V(1).Info("Deleting collection from K8s",
		"namespace", namespace,
	)

	listOptions := &ctrlRTClient.ListOptions{}
	if namespace != "" {
		listOptions.Namespace = namespace
	}
	if listOpts != nil {
		listOptions.Raw = listOpts
	}

	clientDeleteOpts := &ctrlRTClient.DeleteAllOfOptions{
		ListOptions: *listOptions,
	}
	
	if deleteOpts != nil {
		clientDeleteOpts.DeleteOptions = ctrlRTClient.DeleteOptions{
			GracePeriodSeconds: deleteOpts.GracePeriodSeconds,
			Preconditions:      deleteOpts.Preconditions,
			PropagationPolicy:  deleteOpts.PropagationPolicy,
			DryRun:             deleteOpts.DryRun,
			Raw:                deleteOpts,
		}
	}

	err := k.client.DeleteAllOf(ctx, objType, clientDeleteOpts)
	if err != nil {
		k.logger.Error(err, "Failed to delete collection from K8s",
			"namespace", namespace,
		)
		return err
	}

	k.logger.V(1).Info("Successfully deleted collection from K8s",
		"namespace", namespace,
	)
	return nil
}