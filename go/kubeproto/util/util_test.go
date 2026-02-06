package util_test

import (
	"bytes"
	"strings"
	"testing"

	testpb "github.com/michelangelo-ai/michelangelo/proto-go/test/kubeproto"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"

	"github.com/dave/dst/decorator"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestTags(t *testing.T) {
	expParameter := `Mgoogle/protobuf/any.proto=github.com/gogo/protobuf/types;types,` +
		`Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types;types`
	parameter := "Mgoogle/protobuf/any.proto=github.com/gogo/protobuf/types," +
		"Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types"
	req := &pluginpb.CodeGeneratorRequest{
		Parameter: &parameter,
	}

	util.ReplaceImportPath(req)
	assert.Equal(t, expParameter, req.GetParameter())
}

func marshalGenericAny(msg proto.Message) ([]byte, error) {
	var buf bytes.Buffer
	err := (&jsonpb.Marshaler{
		EnumsAsInts:  false,
		EmitDefaults: false,
		OrigName:     false,
		AnyResolver:  &util.GenericResolver{},
	}).Marshal(&buf, msg)
	return buf.Bytes(), err
}

func unmarshalGenericAny(msg proto.Message, b []byte) error {
	return (&jsonpb.Unmarshaler{
		AllowUnknownFields: false,
		AnyResolver:        &util.GenericResolver{},
	}).Unmarshal(bytes.NewReader(b), msg)
}

func marshalDefaultAny(msg proto.Message) ([]byte, error) {
	var buf bytes.Buffer
	err := (&jsonpb.Marshaler{
		EnumsAsInts:  false,
		EmitDefaults: false,
		OrigName:     false,
	}).Marshal(&buf, msg)
	return buf.Bytes(), err
}

func unmarshalDefaultAny(msg proto.Message, b []byte) error {
	return (&jsonpb.Unmarshaler{
		AllowUnknownFields: false,
	}).Unmarshal(bytes.NewReader(b), msg)
}

func TestApplyInlineFields(t *testing.T) {
	tests := []struct {
		name     string
		jsonData []byte
		fields   []util.InlineFieldMapping
		want     []byte
		wantErr  bool
	}{
		{
			name: "simple inline field",
			jsonData: []byte(`{
                "name": "test",
                "details": {
                    "info": "data"
                }
            }`),
			fields: []util.InlineFieldMapping{
				{Path: "details", FieldToBeTrimmed: "info"},
			},
			want: []byte(`{
                "name": "test",
                "details": "data"
            }`),
			wantErr: false,
		},
		{
			name: "nested inline field",
			jsonData: []byte(`{
                "name": "test",
                "details": {
                    "info": {
                        "data": "value"
                    }
                }
            }`),
			fields: []util.InlineFieldMapping{
				{Path: "details.info", FieldToBeTrimmed: "data"},
			},
			want: []byte(`{
                "name": "test",
                "details": {
                    "info": "value"
                }
            }`),
			wantErr: false,
		},
		{
			name: "array inline field",
			jsonData: []byte(`{
                "items": [
					{
						"name": "test",
						"details": {
							"data": "value"
						}
					},
					{
						"name": "test2",
						"details": {
							"data": "value2"
						}
					}
                ]
            }`),
			fields: []util.InlineFieldMapping{
				{Path: "items.#", FieldToBeTrimmed: "details"},
			},
			want: []byte(`{
                "items": [
					{"data":"value","details":{"data":"value"},"name":"test"},
					{"data":"value2","details":{"data":"value2"},"name":"test2"}
                ]
            }`),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := util.ApplyInlineFields(tt.jsonData, tt.fields)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyInlineFields() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("ApplyInlineFields() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestAnyMarshal(t *testing.T) {
	const randomTestString = "ma2022"
	const randomTestString2 = "ma20api"
	const marshalledGenericResolver = `{"any":{"@type":"type.googleapis.com/michelangelo.test.kubeproto.TestMsg","value":"EgZtYTIwMjIaB21hMjBhcGk="}}`
	const marshalledDefaultResolver = `{"any":{"@type":"type.googleapis.com/michelangelo.test.kubeproto.TestMsg","name":"ma2022","projectId":"ma20api"}}`

	foo := &testpb.TestMsg{
		Name:      randomTestString,
		ProjectId: randomTestString2,
	}
	anyFoo, err := types.MarshalAny(foo)
	assert.Nil(t, err)

	object := &testpb.TestObjectSpec{
		Any: anyFoo,
	}

	// Test generic resolver
	jsonBytesGeneric, err := marshalGenericAny(object)
	assert.Nil(t, err)
	assert.Equal(t, marshalledGenericResolver, string(jsonBytesGeneric))

	object2 := &testpb.TestObjectSpec{}
	err = unmarshalGenericAny(object2, jsonBytesGeneric)
	assert.Nil(t, err)

	bar2 := &testpb.TestMsg{}
	err = types.UnmarshalAny(object2.Any, bar2)
	assert.Nil(t, err)
	assert.Equal(t, randomTestString, bar2.Name)
	assert.Equal(t, randomTestString2, bar2.ProjectId)

	// Test default resolver
	jsonBytesDefault, err := marshalDefaultAny(object)
	assert.Nil(t, err)
	assert.Equal(t, marshalledDefaultResolver, string(jsonBytesDefault))

	object3 := &testpb.TestObjectSpec{}
	err = unmarshalDefaultAny(object3, jsonBytesDefault)
	assert.Nil(t, err)

	bar3 := &testpb.TestMsg{}
	err = types.UnmarshalAny(object3.Any, bar3)
	assert.Nil(t, err)
	assert.Equal(t, randomTestString, bar3.Name)
	assert.Equal(t, randomTestString2, bar3.ProjectId)
}

func TestSetPackageAlias(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		pkgPath  string
		newAlias string
		want     string
	}{
		{
			name: "explicit alias",
			input: `package main

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	obj := metav1.ObjectMeta{
		Name: "test",
	}
	var metav1 = "metav1"
	fmt.Println(metav1)
	_ = metav1.Now()
}`,
			pkgPath:  "k8s.io/apimachinery/pkg/apis/meta/v1",
			newAlias: "v1",
			want: `package main

import (
	"fmt"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	obj := v1.ObjectMeta{
		Name: "test",
	}
	var metav1 = "metav1"
	fmt.Println(metav1)
	_ = v1.Now()
}
`,
		},
		{
			name: "implicit alias",
			input: `package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	obj := v1.ObjectMeta{
		Name: "test",
	}

	// Function with same name
	v1 := func() string {
		return "local func"
	}
	println(v1())
	_ = v1.Now()
}
`,
			pkgPath:  "k8s.io/apimachinery/pkg/apis/meta/v1",
			newAlias: "metav1",
			want: `package main

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	obj := metav1.ObjectMeta{
		Name: "test",
	}

	// Function with same name
	v1 := func() string {
		return "local func"
	}
	println(v1())
	_ = metav1.Now()
}
`,
		},
		{
			name: "no package reference updates",
			input: `package main

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func main() {
	metav1 := "test"
	var x metav1
	type metav1 struct{}
}
`,
			pkgPath:  "k8s.io/apimachinery/pkg/apis/meta/v1",
			newAlias: "meta1",
			want: `package main

import meta1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func main() {
	metav1 := "test"
	var x metav1
	type metav1 struct{}
}
`,
		},
		{
			name: "multiple references",
			input: `package main

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func main() {
	_ = metav1.ObjectMeta{}
	_ = metav1.Now()
	_ = metav1.TypeMeta{}
	if x := metav1.Now(); true {
		_ = metav1.DeleteOptions{}
	}
}
`,
			pkgPath:  "k8s.io/apimachinery/pkg/apis/meta/v1",
			newAlias: "meta1",
			want: `package main

import meta1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func main() {
	_ = meta1.ObjectMeta{}
	_ = meta1.Now()
	_ = meta1.TypeMeta{}
	if x := meta1.Now(); true {
		_ = meta1.DeleteOptions{}
	}
}
`,
		},
		{
			name: "package not found",
			input: `package main

import "fmt"

func main() {
	fmt.Println("k8s.io/apimachinery/pkg/apis/meta/v1")
}
`,
			pkgPath:  "k8s.io/apimachinery/pkg/apis/meta/v1",
			newAlias: "meta1",
			want: `package main

import "fmt"

func main() {
	fmt.Println("k8s.io/apimachinery/pkg/apis/meta/v1")
}
`,
		},
		{
			name: "blank import",
			input: `package main

import _ "k8s.io/apimachinery/pkg/apis/meta/v1"

func main() {
}
`,
			pkgPath:  "k8s.io/apimachinery/pkg/apis/meta/v1",
			newAlias: "meta1",
			want: `package main

import _ "k8s.io/apimachinery/pkg/apis/meta/v1"

func main() {
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the input
			f, err := decorator.Parse(tt.input)
			if err != nil {
				t.Fatalf("failed to parse input: %v", err)
			}

			// Apply the transformation
			util.SetPackageAlias(f, tt.pkgPath, tt.newAlias)

			// Convert the result back to string
			var output strings.Builder
			if err := decorator.Fprint(&output, f); err != nil {
				t.Fatalf("failed to print output: %v", err)
			}

			// Compare with expected output
			assert.Equal(t, tt.want, output.String())
		})
	}
}
