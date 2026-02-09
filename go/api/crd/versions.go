package crd

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// NewObjectForGVK creates a new empty object for the given GVK using the global scheme.Scheme.
// The CRD Go type must have been registered via AddToScheme.
func NewObjectForGVK(gvk schema.GroupVersionKind) (ctrlRTClient.Object, error) {
	// Create a new runtime.Object for this GVK.
	obj, err := scheme.Scheme.New(gvk)
	if err != nil {
		// This usually means the GVK is not registered in the scheme.
		return nil, fmt.Errorf("failed to create object for GVK %s: %w", gvk.String(), err)
	}

	// Convert to controller-runtime Object.
	crObj, ok := obj.(ctrlRTClient.Object)
	if !ok {
		return nil, fmt.Errorf("object for GVK %s does not implement client.Object (type %T)", gvk.String(), obj)
	}
	return crObj, nil
}

// ConvertObjectVersion converts src into dst using controller-runtime conversion interfaces.
// Either src or dst must implement conversion.Hub. The other side must implement
// conversion.Convertible.
func ConvertObjectVersion(src runtime.Object, dst runtime.Object) error {
	if src == nil {
		return fmt.Errorf("source object is nil")
	}
	if dst == nil {
		return fmt.Errorf("destination object is nil")
	}

	if dstHub, ok := dst.(conversion.Hub); ok {
		if srcHub, ok := src.(conversion.Hub); ok {
			return copyRuntimeObject(srcHub, dst)
		}
		srcConv, ok := src.(conversion.Convertible)
		if !ok {
			return fmt.Errorf("source %s (%T) does not implement conversion.Convertible", gvkString(src), src)
		}
		if err := srcConv.ConvertTo(dstHub); err != nil {
			return fmt.Errorf("failed to convert %s to hub %s: %w", gvkString(src), gvkString(dst), err)
		}
		return nil
	}

	if srcHub, ok := src.(conversion.Hub); ok {
		dstConv, ok := dst.(conversion.Convertible)
		if !ok {
			return fmt.Errorf("destination %s (%T) does not implement conversion.Convertible", gvkString(dst), dst)
		}
		if err := dstConv.ConvertFrom(srcHub); err != nil {
			return fmt.Errorf("failed to convert hub %s to %s: %w", gvkString(src), gvkString(dst), err)
		}
		return nil
	}

	return fmt.Errorf("neither source nor destination implements conversion.Hub (src %s, dst %s)", gvkString(src), gvkString(dst))
}

func gvkString(obj runtime.Object) string {
	gvks, _, err := scheme.Scheme.ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		return "unknown"
	}
	return gvks[0].String()
}

func copyRuntimeObject(src runtime.Object, dst runtime.Object) error {
	if src == dst {
		return nil
	}

	srcVal := reflect.ValueOf(src)
	dstVal := reflect.ValueOf(dst)
	if dstVal.Kind() != reflect.Ptr || dstVal.IsNil() {
		return fmt.Errorf("destination must be a non-nil pointer, got %T", dst)
	}
	if srcVal.Type() != dstVal.Type() {
		return fmt.Errorf("source and destination types differ: %T -> %T", src, dst)
	}
	if srcVal.Kind() != reflect.Ptr || srcVal.IsNil() {
		return fmt.Errorf("source must be a non-nil pointer, got %T", src)
	}

	dstVal.Elem().Set(srcVal.Elem())
	return nil
}
