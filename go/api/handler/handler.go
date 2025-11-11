package handler

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlRTApiutil "sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	ctrlRTUtil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	apiActionLatencyMetric = "apiActionLatency"
)

// NewFakeAPIHandler creates an API handler with the provided k8s client.  This is used for unit test only.
func NewFakeAPIHandler(k8sClient ctrlRTClient.Client) api.Handler {
	return &apiHandler{
		conf: storage.MetadataStorageConfig{
			EnableMetadataStorage: false,
		},
		logger:            zapr.NewLogger(zap.NewNop()),
		metrics:           tally.NoopScope,
		k8sHandler:        NewK8sHandler(k8sClient),
		metadataHandler:   NewMetadataHandler(nil, nil, zapr.NewLogger(zap.NewNop())),
		blobHandler:       NewBlobHandler(nil),
		validationHandler: NewValidationHandler(),
	}
}

// apiHandler is an api.Handler that abstracts the API operations from the underlying systems (i.e. k8s/ETCD + MetadataStorage).
type apiHandler struct {
	// storage library configuration
	conf storage.MetadataStorageConfig

	logger logr.Logger

	metrics tally.Scope

	// Focused handlers for better separation of concerns
	k8sHandler        K8sHandler        `optional:"true"`
	metadataHandler   MetadataHandler   `optional:"true"`
	blobHandler       BlobHandler       `optional:"true"`
	validationHandler ValidationHandler `optional:"true"`
}

// Create implements api.Handler.Create
// Returns nil if successful, otherwise a gRPC status error is returned.
func (handler *apiHandler) Create(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.CreateOptions) error {
	start := time.Now()
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if typeMeta, err := utils.GetObjectTypeMetafromObject(obj, scheme.Scheme); err == nil {
		kind = typeMeta.Kind
	}
	log, headers := initLogger(ctx, handler.logger, "Create", obj.GetNamespace(), obj.GetName(), kind)
	defer emitAPIMetrics("Create", handler.metrics, log, start, kind, headers)
	if handler.validationHandler != nil {
		if err := handler.validationHandler.ValidateCreate(obj); err != nil {
			return err
		}
	} else if err := api.Validate(obj); err != nil {
		return err
	}

	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}

	if metadataErr := handler.handleMetadataStorageForCreate(ctx, obj, objMeta); metadataErr != nil {
		return metadataErr
	}

	setUpdateTimestamp(obj, true)

	// If the object does not exist in MetadataStorage, create it in K8s/ETCD.
	err = handler.k8sHandler.Create(ctx, obj, opts)

	return surfaceGrpcError(err, "create", objMeta.GetNamespace(), objMeta.GetName())
}

// Get implements api.Handler.Get
// Returns nil if successful, otherwise a gRPC status error is returned.
func (handler *apiHandler) Get(
	ctx context.Context, namespace string, name string, _ *metav1.GetOptions, obj ctrlRTClient.Object) error {
	start := time.Now()
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if typeMeta, err := utils.GetObjectTypeMetafromObject(obj, scheme.Scheme); err == nil {
		kind = typeMeta.Kind
	}
	log, headers := initLogger(ctx, handler.logger, "Get", namespace, name, kind)
	defer emitAPIMetrics("Get", handler.metrics, log, start, kind, headers)

	// Get from K8s/ETCD
	err := handler.k8sHandler.Get(ctx, namespace, name, obj)
	if err == nil {
		// If an object is immutable and is being deleted in ETCD, it means the object is within the grace period and is
		// pending deleted, so we should proceed to the section below to get from the metadata storage.
		if !isDeletedImmutableObject(obj) {
			log.Info("Find object in ETCD")
			return nil
		}
	}
	// if the k8s client error is not "not found", return the error
	if !apiErrors.IsNotFound(err) {
		return surfaceGrpcError(err, "get", namespace, name)
	}

	// If the object does not exist in K8s/ETCD, get from metadata storage.
	if storage.EnableMetadataStorage(&handler.conf) {
		if err = handler.metadataHandler.Get(ctx, namespace, name, obj); err != nil {
			return surfaceGrpcError(err, "get", namespace, name)
		}

		handler.handleBlobStorageForGet(ctx, obj, log)
		log.Info("Find object in metadata storage")
	}

	return surfaceGrpcError(err, "get", namespace, name)
}

// Update implements api.Handler.Update
// Returns nil if successful, otherwise a gRPC status error is returned.
func (handler *apiHandler) Update(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	start := time.Now()
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	log, headers := initLogger(ctx, handler.logger, "Update", obj.GetNamespace(), obj.GetName(), kind)
	defer emitAPIMetrics("Update", handler.metrics, log, start, kind, headers)
	if handler.validationHandler != nil {
		if err := handler.validationHandler.ValidateUpdate(obj); err != nil {
			return err
		}
	} else if err := api.Validate(obj); err != nil {
		return err
	}

	hasSpecChange, err := handler.hasSpecChange(ctx, obj)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}
	setUpdateTimestamp(obj, hasSpecChange)

	tmpObj, ok := obj.DeepCopyObject().(ctrlRTClient.Object)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "object does not implement the controller-runtime client.Object interface")
	}

	err = handler.k8sHandler.Update(ctx, obj, opts)

	// If the object does not exist in K8s/ETCD, update it in MetadataStorage directly.
	if apiErrors.IsNotFound(err) && storage.EnableMetadataStorage(&handler.conf) {
		err = handler.metadataHandler.Update(ctx, tmpObj)
		if err != nil {
			return surfaceGrpcError(err, "update", tmpObj.GetNamespace(), tmpObj.GetName())
		}
	}

	return surfaceGrpcError(err, "update", tmpObj.GetNamespace(), tmpObj.GetName())
}

// UpdateStatus implements api.Handler.UpdateStatus
// Returns nil if successful, otherwise a gRPC status error is returned.
func (handler *apiHandler) UpdateStatus(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	start := time.Now()
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	log, headers := initLogger(ctx, handler.logger, "UpdateStatus", obj.GetNamespace(), obj.GetName(), kind)
	defer emitAPIMetrics("UpdateStatus", handler.metrics, log, start, kind, headers)

	if handler.validationHandler != nil {
		if err := handler.validationHandler.ValidateUpdate(obj); err != nil {
			return err
		}
	} else if err := api.Validate(obj); err != nil {
		return err
	}

	tmpObj, ok := obj.DeepCopyObject().(ctrlRTClient.Object)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "object does not implement the controller-runtime client.Object interface")
	}

	setUpdateTimestamp(obj, false)

	err := handler.k8sHandler.UpdateStatus(ctx, obj, opts)

	return surfaceGrpcError(err, "updateStatus", tmpObj.GetNamespace(), tmpObj.GetName())
}

// Delete implements api.Handler.Delete
// Returns nil if successful, otherwise a gRPC status error is returned.
func (handler *apiHandler) Delete(ctx context.Context, obj ctrlRTClient.Object,
	opts *metav1.DeleteOptions) error {
	start := time.Now()
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if typeMeta, err := utils.GetObjectTypeMetafromObject(obj, scheme.Scheme); err == nil {
		kind = typeMeta.Kind
	}
	log, headers := initLogger(ctx, handler.logger, "Delete", obj.GetNamespace(), obj.GetName(), kind)
	defer emitAPIMetrics("Delete", handler.metrics, log, start, kind, headers)

	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}

	// Delete the object in K8s/ETCD
	if storage.EnableMetadataStorage(&handler.conf) == false {
		err = handler.k8sHandler.Delete(ctx, obj, opts)
		return surfaceGrpcError(err, "delete", objMeta.GetNamespace(), objMeta.GetName())
	}

	// When metadata storage is enabled, we only mark DeletingAnnotation here. The ingester will handle the deletion.
	tmpObj, ok := obj.DeepCopyObject().(ctrlRTClient.Object)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "object does not implement the controller-runtime client.Object interface")
	}
	err = handler.k8sHandler.Get(ctx, objMeta.GetNamespace(), objMeta.GetName(), tmpObj)
	if err == nil {
		// Object exists in K8s/ETCD
		if utils.IsImmutable(tmpObj) {
			// If the obj is immutable, it means we're deleting an immutable object within its grace period
			// There are two cases:
			// 1. The object has deletion time stamp: It means the object is already synced to metadata storage by the
			// 	  ingester controller, and is pending to be deleted after the grace period passes. We should delete the
			//    object from metadata storage.
			// 2. The object does not have deletion time stamp: It means the object has not yet be processed by the
			//    ingester controller. This should be a very rare case as typically objects are processed by the
			//    ingester within seconds. In this case, we return Unavailable error to ask the user to retry the action
			//    later.
			if tmpObj.GetDeletionTimestamp() != nil {
				log.Info("Deleting an immutable object with deletion timestamp != nil - deleting from metadata storage")
				return deleteObjectFromMetadataStorage(ctx, tmpObj, handler)
			}

			log.Info("Deleting an immutable object w/o deletion timestamp")
			return status.Error(codes.Unavailable, "The system is not caught up yet. Please try again.")
		}

		log.Info("Deleting a mutable object")
		// Update the UserDeletionAnnotation to "true" so that ingester will delete it
		annotation := tmpObj.GetAnnotations()
		if annotation == nil {
			annotation = make(map[string]string)
			tmpObj.SetAnnotations(annotation)
		}
		annotation[api.DeletingAnnotation] = "true"

		err = handler.k8sHandler.Update(ctx, tmpObj, &metav1.UpdateOptions{})
		return surfaceGrpcError(err, "delete", objMeta.GetNamespace(), objMeta.GetName())
	}

	// If the object does not exist in K8s/ETCD, delete it in metadata storage.
	if apiErrors.IsNotFound(err) && storage.EnableMetadataStorage(&handler.conf) {
		log.Info("Object does not exist in ETCD - deleting from metadata storage")
		return deleteObjectFromMetadataStorage(ctx, obj, handler)
	}
	return err
}

// List implements api.Handler.List
// When metadata storage is enabled, this function only returns objects that are already ingested into metadata storage.
// ListOptionsExt is only supported when metadata storage is enabled.
// Returns nil if successful, otherwise a gRPC status error is returned.
func (handler *apiHandler) List(ctx context.Context, namespace string, opts *metav1.ListOptions,
	listOptionsExt *apipb.ListOptionsExt, list ctrlRTClient.ObjectList) error {
	start := time.Now()
	kind := list.GetObjectKind().GroupVersionKind().Kind
	if typeMeta, err := utils.GetObjectTypeMetaFromList(list, scheme.Scheme); err == nil {
		kind = typeMeta.Kind
	}
	log, headers := initLogger(ctx, handler.logger, "List", namespace, "", kind)
	defer emitAPIMetrics("List", handler.metrics, log, start, kind, headers)

	if storage.EnableMetadataStorage(&handler.conf) {
		return handler.metadataHandler.List(ctx, namespace, opts, listOptionsExt, list)
	} else if listOptionsExt != nil && !listOptionsExt.Equal(&apipb.ListOptionsExt{}) {
		log.Info("ListOptionsExt is ignored when metadata storage is not enabled", "listOptionsExt", listOptionsExt)
	}

	err := handler.k8sHandler.List(ctx, namespace, opts, list)
	return surfaceGrpcError(err, "list", namespace, "")
}

// DeleteCollection implements api.Handler.DeleteCollection
// When metadata storage is enabled, this function only list and delete the objects that are already ingested into
// metadata storage.
// Returns nil if successful, otherwise a gRPC status error is returned.
func (handler *apiHandler) DeleteCollection(
	ctx context.Context, objType ctrlRTClient.Object, namespace string, deleteOpts *metav1.DeleteOptions,
	listOpts *metav1.ListOptions) error {
	start := time.Now()
	kind := objType.GetObjectKind().GroupVersionKind().Kind
	if typeMeta, err := utils.GetObjectTypeMetafromObject(objType, scheme.Scheme); err == nil {
		kind = typeMeta.Kind
	}
	log, headers := initLogger(ctx, handler.logger, "DeleteCollection", namespace, "", kind)
	defer emitAPIMetrics("DeleteCollection", handler.metrics, log, start, kind, headers)

	if storage.EnableMetadataStorage(&handler.conf) == false {
		err := handler.k8sHandler.DeleteCollection(ctx, objType, namespace, deleteOpts, listOpts)
		return surfaceGrpcError(err, "delete collection", namespace, "")
	}

	if namespace == "" {
		return status.Errorf(codes.InvalidArgument, "namespace is not specified")
	}

	// List objects from metadata storage
	gvk, err := ctrlRTApiutil.GVKForObject(objType, scheme.Scheme)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}

	listGVK := gvk.GroupVersion().WithKind(gvk.Kind + "List")
	newObj, err := scheme.Scheme.New(listGVK)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}
	listObj, ok := newObj.(ctrlRTClient.ObjectList)
	if !ok {
		return status.Errorf(codes.Internal, "new object does not implement the controller-runtime client.ObjectList interface")
	}
	err = handler.metadataHandler.List(ctx, namespace, listOpts, nil, listObj)
	if err != nil {
		return err
	}

	// Delete the list of items one by one
	items, err := meta.ExtractList(listObj)
	if err != nil || len(items) == 0 {
		return err
	}

	for _, item := range items {
		err = handler.Delete(ctx, item.(ctrlRTClient.Object), deleteOpts)
		if err != nil {
			return err
		}
	}

	return nil
}

func isDeletedImmutableObject(obj ctrlRTClient.Object) bool {
	return utils.IsImmutable(obj) && obj.GetDeletionTimestamp() != nil
}

func deleteObjectFromMetadataStorage(ctx context.Context, obj ctrlRTClient.Object, handler *apiHandler) error {
	err := handler.metadataHandler.Delete(ctx, obj)
	if err != nil {
		return surfaceGrpcError(err, "delete", obj.GetNamespace(), obj.GetName())
	}
	return nil
}

func getCRTListOptions(namespace string, opts *metav1.ListOptions) (*ctrlRTClient.ListOptions, error) {
	var labelSelector labels.Selector
	var fieldSelector fields.Selector
	var err error
	if len(opts.LabelSelector) > 0 {
		labelSelector, err = labels.Parse(opts.LabelSelector)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "failed to parse label selector: %v", err)
		}
	}
	if len(opts.FieldSelector) > 0 {
		fieldSelector, err = fields.ParseSelector(opts.FieldSelector)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "failed to parse field selector: %v", err)
		}
	}

	return &ctrlRTClient.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
		Namespace:     namespace,
		Limit:         opts.Limit,
		Continue:      opts.Continue,
		Raw:           opts}, nil
}

func surfaceGrpcError(err error, apiAction string, namespace string, name string) error {
	if err == nil {
		return nil
	}
	errMsg := fmt.Sprintf("failed to %v API object. namespace: %v, name: %v", apiAction, namespace, name)

	// k8s errors
	if _, ok := err.(apiErrors.APIStatus); ok {
		return api.K8sError2GrpcError(err, errMsg)
	}

	// other errors
	// if err is a grpc status error, status.Convert() keeps the original error code
	// otherwise, the error code is set to Unknown
	s := status.Convert(err)
	return status.Errorf(s.Code(), "%v: %v", errMsg, err)
}

func initLogger(ctx context.Context, log logr.Logger, action string, namespace string, name string,
	kind string) (logr.Logger, map[string]string) {
	initLog := log.WithValues("action", action, "namespace", namespace, "name", name, "kind", kind)
	headers := utils.GetHeaders(ctx)
	return initLog, headers
}

func (handler *apiHandler) hasSpecChange(ctx context.Context, objForUpdate ctrlRTClient.Object) (bool, error) {
	// Since the spec cannot be updated for objects only in metadata storage (immutable objects), we only need to
	// look for the object in k8s/ETCD. If the object is not found in ETCD, the update must be on either labels or
	// annotations, so there's no change in spec.
	apiServerObj := reflect.New(reflect.TypeOf(objForUpdate).Elem()).Interface().(ctrlRTClient.Object)
	err := handler.k8sHandler.Get(ctx, objForUpdate.GetNamespace(), objForUpdate.GetName(), apiServerObj)
	if err == nil {
		isEqual, err := isSpecEqual(apiServerObj, objForUpdate)
		if err != nil {
			return false, nil
		}
		if !isEqual {
			return true, nil
		}
	}

	return false, nil
}

func setUpdateTimestamp(obj ctrlRTClient.Object, hasSpecChange bool) {
	t := strconv.FormatInt(time.Now().UnixMicro(), 10)
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	labels[api.UpdateTimestampLabel] = t
	if hasSpecChange {
		labels[api.SpecUpdateTimestampLabel] = t
	}
	obj.SetLabels(labels)
}

func emitAPIMetrics(action string, scope tally.Scope, logger logr.Logger, t time.Time, kind string, headers map[string]string) {
	took := time.Now().Sub(t)
	tag := map[string]string{"action": action, "kind": kind}
	// 10 bucket ~= 1 sec
	// 16 bucket ~= 1 min
	scope.Tagged(tag).Histogram(apiActionLatencyMetric, tally.MustMakeExponentialDurationBuckets(time.Millisecond, 2.0,
		16)).RecordDuration(took)
	scope.Tagged(tag).Counter("calls")
	logger.Info(fmt.Sprintf("API %s took %d milli seconds", action, took.Milliseconds()), "headers", headers)
}

// isSpecEqual checks whether the Spec of two objects are equal.
// Returns error if passed in objects have no method called `GetSpec`.
func isSpecEqual(lhs, rhs any) (bool, error) {
	lSpec, err := getSpec(lhs)
	if err != nil {
		return false, err
	}
	rSpec, err := getSpec(rhs)
	if err != nil {
		return false, err
	}

	return reflect.DeepEqual(lSpec, rSpec), nil
}

const _getSpecMethodName = "GetSpec"

func getSpec(object any) (any, error) {
	rv := reflect.ValueOf(object)
	t := rv.Type()
	_, ok := t.MethodByName(_getSpecMethodName)
	if !ok {
		return nil, fmt.Errorf("object of type %s does not have a method named %s", t.Name(), _getSpecMethodName)
	}
	return rv.MethodByName(_getSpecMethodName).Call([]reflect.Value{})[0].Interface(), nil
}

// handleMetadataStorageForCreate handles metadata storage logic during create operations
func (handler *apiHandler) handleMetadataStorageForCreate(ctx context.Context, obj ctrlRTClient.Object, objMeta metav1.Object) error {
	if !storage.EnableMetadataStorage(&handler.conf) {
		return nil
	}

	// Check if the object exists in MetadataStorage
	tmpObj := obj
	err := handler.metadataHandler.Get(ctx, objMeta.GetNamespace(), objMeta.GetName(), tmpObj)
	if err == nil {
		return status.Errorf(codes.AlreadyExists, "failed to create API object. An object of the same name already exists. namespace:%v, name: %v",
			objMeta.GetNamespace(), objMeta.GetName())
	}

	// Add ingester finalizer
	ctrlRTUtil.AddFinalizer(objMeta.(ctrlRTClient.Object), api.IngesterFinalizer)
	return nil
}

// handleBlobStorageForGet handles blob storage logic during get operations
func (handler *apiHandler) handleBlobStorageForGet(ctx context.Context, obj ctrlRTClient.Object, log logr.Logger) {
	if handler.blobHandler.IsObjectInteresting(obj) {
		if terrablobErr := handler.blobHandler.MergeWithExternalBlob(ctx, obj); terrablobErr != nil {
			log.Error(terrablobErr, "Failed to merging with blob storage")
		}
	}
}
