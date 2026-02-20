package crd

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"

	v1pb "github.com/michelangelo-ai/michelangelo/proto-go/test/kubeproto/conversion/v1"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/test/kubeproto/conversion/v2"
)

type nonClientObject struct{}

func (n *nonClientObject) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (n *nonClientObject) DeepCopyObject() runtime.Object {
	return &nonClientObject{}
}

func TestNewObjectForGVK_KnownType(t *testing.T) {
	gvk := corev1.SchemeGroupVersion.WithKind("ConfigMap")

	obj, err := NewObjectForGVK(gvk)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if obj == nil {
		t.Fatal("expected object, got nil")
	}
	if _, ok := obj.(*corev1.ConfigMap); !ok {
		t.Fatalf("expected *corev1.ConfigMap, got %T", obj)
	}
}

func TestNewObjectForGVK_UnknownType(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "unknown.michelangelo.ai",
		Version: "v1",
		Kind:    "DoesNotExist",
	}

	_, err := NewObjectForGVK(gvk)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create object for GVK") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestNewObjectForGVK_NonClientObject(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "nonclient.michelangelo.ai",
		Version: "v1",
		Kind:    "NonClientObject",
	}
	if scheme.Scheme.Recognizes(gvk) {
		t.Skip("GVK already registered in scheme")
	}

	scheme.Scheme.AddKnownTypeWithName(gvk, &nonClientObject{})

	_, err := NewObjectForGVK(gvk)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "does not implement client.Object") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

var setCustomConvertorOnce sync.Once

type testConvertor struct{}

func (c *testConvertor) ConvertToHub(src *v1pb.TestObject, dst *v2pb.TestObject) error {
	for _, i := range src.Spec.IntList {
		dst.Spec.StringList = append(dst.Spec.StringList, strconv.Itoa(int(i)))
	}
	return nil
}

func (c *testConvertor) ConvertFromHub(src *v2pb.TestObject, dst *v1pb.TestObject) error {
	for _, s := range src.Spec.StringList {
		i, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		dst.Spec.IntList = append(dst.Spec.IntList, int32(i))
	}
	return nil
}

func ensureCustomConvertor() {
	setCustomConvertorOnce.Do(func() {
		v1pb.SetCustomTestObjectConvertor(&testConvertor{})
	})
}

func TestConvertObjectVersion_SpokeHubRoundTrip(t *testing.T) {
	ensureCustomConvertor()

	src := &v1pb.TestObject{
		Spec: v1pb.TestObjectSpec{
			F1: 123,
			F2: []string{"A", "BC"},
			F3: &v1pb.M1{
				F1: []v1pb.E1{v1pb.E1_3},
				F2: map[string]string{
					"ABC": "DEF",
					"123": "",
				},
				F3: map[string]v1pb.E1{
					"1": v1pb.E1_2,
					"3": v1pb.E1_3,
				},
			},
			F4: []*v1pb.M1{},
			F5: map[string]*v1pb.M2{
				"A": nil,
				"B": {
					F1: []*v1pb.M3{
						{
							F1: []v1pb.E2{v1pb.A, v1pb.B},
						},
						{
							F1: []v1pb.E2{v1pb.B},
						},
					},
				},
			},
			IntList: []int32{123, 321},
		},
		Status: v1pb.TestObjectStatus{
			F1: v1pb.E1_2,
		},
	}

	hub := &v2pb.TestObject{}
	if err := ConvertObjectVersion(src, hub); err != nil {
		t.Fatalf("convert spoke to hub failed: %v", err)
	}

	dst := &v1pb.TestObject{}
	if err := ConvertObjectVersion(hub, dst); err != nil {
		t.Fatalf("convert hub to spoke failed: %v", err)
	}

	if !reflect.DeepEqual(src, dst) {
		t.Fatalf("round trip mismatch: src=%+v dst=%+v", src, dst)
	}
}

func TestConvertObjectVersion_HubToHubCopy(t *testing.T) {
	ensureCustomConvertor()

	src := &v2pb.TestObject{
		Spec: v2pb.TestObjectSpec{
			F1: 456,
			F2: []string{"X", "YZ"},
			StringList: []string{
				"10",
				"20",
			},
		},
		Status: v2pb.TestObjectStatus{
			F1: v2pb.E1_1,
		},
	}
	dst := &v2pb.TestObject{}

	if err := ConvertObjectVersion(src, dst); err != nil {
		t.Fatalf("convert hub to hub failed: %v", err)
	}
	if !reflect.DeepEqual(src, dst) {
		t.Fatalf("hub copy mismatch: src=%+v dst=%+v", src, dst)
	}
}

func TestConvertObjectVersion_NoHub(t *testing.T) {
	ensureCustomConvertor()

	src := &v1pb.TestObject{}
	dst := &v1pb.TestObject{}

	if err := ConvertObjectVersion(src, dst); err == nil {
		t.Fatal("expected error, got nil")
	}
}
