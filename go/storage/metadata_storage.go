//go:generate mamockgen MetadataStorage
package storage

import (
	"context"
	"time"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// MetadataStorage is the indexed metadata storage of Michelangelo.
//
// When MetadataStorage is enabled, all the Michelangelo API objects that are created / modified in k8s/ETCD are
// actively synced to the MetadataStorage for faster search.
// Michelangelo API objects that has reached a final state (cannot be updated anymore) may be removed from ETCD and
// stored solely in the MetadataStorage to save ETCD space and avoid unnecessary reconciling.
//
// Each function of this interface has to be an atomic operation.
type MetadataStorage interface {
	// Upsert adds a new object or update an existing one.
	// This method may be called by ingester, to sync an update from ETCD (directUpdate = false);
	// or called directly by API server / controller (direct = true) to update an object that is only
	// stored in the MetadataStorage.
	// When direct = true, the method will
	// 1) only update labels, annotations, and resource version
	// 2) check the ResourceVersion for "optimistically concurrency control"
	// 3) return a new ResourceVersion when success. The ResourceVersion returned in this case should not be
	//    used for listing or watching.
	// If any fields of the object are indexed, the caller has to provide a list of indexed fields key value pairs
	// for the MetadataStorage to build / update indexes.
	Upsert(ctx context.Context, object runtime.Object, direct bool, indexedFields []IndexedField) error

	// GetByName retrieves an object by its namespace and name.
	GetByName(ctx context.Context, namespace string, name string, object runtime.Object) error

	// GetByID retrieves an object by its UID.
	GetByID(ctx context.Context, uid string, object runtime.Object) error

	// List objects.
	// If namespace is empty, this function will search all namespaces
	List(ctx context.Context, typeMeta *v1.TypeMeta, namespace string, listOptions *v1.ListOptions,
		listOptionsExt *apipb.ListOptionsExt, listResponse *ListResponse) error

	// Delete an object.
	// Maybe called by ingester, if the object is deleted through k8s/ETCD.
	// Or directly called by API server or controller, if the object is only stored in the MetadataStorage.
	Delete(ctx context.Context, typeMeta *v1.TypeMeta, namespace string, name string) error

	// DeleteCollection deletes a collection of objects.
	// Maybe called by ingester, if the object is deleted through k8s/ETCD.
	// Or directly called by API server or controller, if the object is only stored in the MetadataStorage.
	DeleteCollection(ctx context.Context, namespace string, deleteOptions *v1.DeleteOptions, listOptions *v1.ListOptions) error

	// QueryByTemplateID queries objects with a predefined query template
	// Query parameters are provided by listOptionsExt
	QueryByTemplateID(ctx context.Context, typeMeta *v1.TypeMeta, templateID string, listOptionsExt *apipb.ListOptionsExt,
		listResponse *ListResponse) error

	// Backfill performs backfill operation specified by createFn using the options given by opts.
	// It backfills for a Kind (specified by "kind" in BackfillOptions) by the order of creation timestamp.
	// The backfill starts from the object after "StartTime", and it backfills at least "BatchSize" objects.
	// If there are more objects to be backfilled, "endTime" is returned, and objects that have creationTimestamp less
	// than or equal to endTime are backfilled.  Otherwise, "endTime" is set to nil.
	// Sets err to nil if successful, otherwise a gRPC status error is returned.
	Backfill(ctx context.Context, createFn PrepareBackfillParams, opts BackfillOptions) (endTime *time.Time,
		err error)

	// Close DB connection
	Close()
}

// ListResponse response of MetadataStorage.List().
type ListResponse struct {
	// Continue may be set if the user set a limit on the number of items returned, and indicates that
	// the server has more data available. The value is opaque and may be used to issue another request
	// to the endpoint that served this list to retrieve the next set of available objects. Continuing a
	// consistent list may not be possible if the server configuration has changed or more than a few
	// minutes have passed.
	Continue string

	// Items are the objects returned by the MetadataStorage.List() call.
	Items []runtime.Object
}

// IndexedObject interface defines a list of functions that an API object may implement to allow indexing in metadata
// storage.
type IndexedObject interface {
	// GetIndexedKeyValuePairs returns all the indexed fields of this object
	GetIndexedKeyValuePairs() []IndexedField

	// IsImmutableKind returns whether the Kind of this object is an "immutable Kind". Immutable kind refers to API
	// Kinds for which all objects are immutable, meaning they cannot be modified or updated after they have been
	// created.
	IsImmutableKind() bool
}

// IndexedField represents an indexed field in a CRD.
type IndexedField struct {
	// Key uniquely identifies an index in a CRD, and it represents the column name of the indexed field
	// in the metadata storage.
	Key string

	// Value is the value of the indexed field.
	Value interface{}
}

// ObjectWithBlobFields defines a list of functions that an API object may implement to allow handling of blob fields.
type ObjectWithBlobFields interface {
	// HasBlobFields returns whether the CRD kind has any blob fields
	HasBlobFields() bool

	// ClearBlobFields clears blob fields
	ClearBlobFields()

	// FillBlobFields fills blob fields from the object obtained externally.
	FillBlobFields(runtime.Object)
}

// BackfillParams contains data required to perform backfill operations.
// For example, to backfill the index, set the IndexedFields to be the indices for the given Object.
type BackfillParams struct {
	Object        runtime.Object
	IndexedFields []IndexedField
}

// PrepareBackfillParams creates backfill params from a runtime Object.
type PrepareBackfillParams = func(runtime.Object) (*BackfillParams, error)

// BackfillOptions stores options about backfill job
type BackfillOptions struct {
	// CRD kind to backfill
	Kind string `yaml:"kind"`
	// The type of backfill command, e.g., index, blob, etc.
	Command string `yaml:"command"`
	// If provided, items having creation time >= StartTime are backfilled
	StartTime time.Time `yaml:"startTime"`
	// Whether to exclude (soft-)deleted items from backfilling; default is false
	ExcludeDeleted bool `yaml:"excludeDeleted"`
	// If provided, only items under the given namespaces are backfilled
	NameSpaces []string `yaml:"namespaces"`
	// The size of the batch for backfill
	BatchSize int
}
