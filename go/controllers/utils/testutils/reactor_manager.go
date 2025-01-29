package testutils

import (
	"sync"

	v1 "k8s.io/api/apps/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// ReactorManager holds the shared in-memory store and provides reusable reactor logic.
type ReactorManager struct {
	store sync.Map // Thread-safe store for CRDs
}

// CreateReactor returns a reusable "create" reactor for a specific CRD type.
func (rm *ReactorManager) CreateReactor() k8stesting.ReactionFunc {
	return func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		obj := createAction.GetObject()

		// Save the object to the in-memory store
		metadata := obj.(metav1.Object)
		rm.store.Store(metadata.GetName(), obj)

		// Return the created object
		return true, obj, nil
	}
}

// GetReactor returns a reusable "get" reactor for a specific CRD type.
func (rm *ReactorManager) GetReactor() k8stesting.ReactionFunc {
	return func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		getAction := action.(k8stesting.GetAction)
		name := getAction.GetName()
		resource := getAction.GetResource().Resource

		// Retrieve the object from the in-memory store
		obj, ok := rm.store.Load(name)
		if !ok {
			return true, nil, apiErrors.NewNotFound(v1.Resource(resource), name)
		}

		println("************************")
		// Return the retrieved object
		return true, obj.(runtime.Object), nil
	}
}
