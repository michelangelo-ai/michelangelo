package conversion

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/r3labs/diff/v3"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1pb "github.com/michelangelo-ai/michelangelo/proto-go/test/kubeproto/conversion/v1"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/test/kubeproto/conversion/v2"
)

func TestGen(t *testing.T) {
	data := v1pb.GetProtocReqData()
	resp := Generate(data)
	assert.NotNil(t, resp)
	data2 := v2pb.GetProtocReqData()
	resp2 := Generate(data2)
	assert.NotNil(t, resp2)
}

func TestConvert(t *testing.T) {
	v2obj := &v2pb.TestObject{}
	assert.Implements(t, (*conversion.Hub)(nil), v2obj)
	v1obj := &v1pb.TestObject{}
	assert.Implements(t, (*conversion.Convertible)(nil), v1obj)
	v1pb.SetCustomTestObjectConvertor(&testConvertor{})
	v1obj = &v1pb.TestObject{
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
			IntList: []int32{
				123,
				321,
			},
		},
		Status: v1pb.TestObjectStatus{
			F1: v1pb.E1_2,
		},
	}
	err := v1obj.ConvertTo(v2obj)
	assert.NoError(t, err)
	v1objres := &v1pb.TestObject{}
	err = v1objres.ConvertFrom(v2obj)
	assert.NoError(t, err)
	change, err := diff.Diff(v1obj, v1objres)
	assert.NoError(t, err)
	assert.Empty(t, change)
	assert.True(t, reflect.DeepEqual(v1obj, v1objres)) // reflect.DeepEqual() is more restricted than diff.Diff()
}

func TestConvertOneofString(t *testing.T) {
	v1obj := &v1pb.TestObject{
		Spec: v1pb.TestObjectSpec{
			F1:        1,
			TestOneof: &v1pb.TestObjectSpec_OneofStr{OneofStr: "hello"},
		},
		Status: v1pb.TestObjectStatus{},
	}
	v2obj := &v2pb.TestObject{}
	err := v1obj.ConvertTo(v2obj)
	assert.NoError(t, err)

	v2Str, ok := v2obj.Spec.TestOneof.(*v2pb.TestObjectSpec_OneofStr)
	assert.True(t, ok)
	assert.Equal(t, "hello", v2Str.OneofStr)

	v1objres := &v1pb.TestObject{}
	err = v1objres.ConvertFrom(v2obj)
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(v1obj, v1objres))
}

func TestConvertOneofInt(t *testing.T) {
	v1obj := &v1pb.TestObject{
		Spec: v1pb.TestObjectSpec{
			F1:        1,
			TestOneof: &v1pb.TestObjectSpec_OneofInt{OneofInt: 42},
		},
		Status: v1pb.TestObjectStatus{},
	}
	v2obj := &v2pb.TestObject{}
	err := v1obj.ConvertTo(v2obj)
	assert.NoError(t, err)

	v2Int, ok := v2obj.Spec.TestOneof.(*v2pb.TestObjectSpec_OneofInt)
	assert.True(t, ok)
	assert.Equal(t, int32(42), v2Int.OneofInt)

	v1objres := &v1pb.TestObject{}
	err = v1objres.ConvertFrom(v2obj)
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(v1obj, v1objres))
}

func TestConvertOneofMessage(t *testing.T) {
	v1obj := &v1pb.TestObject{
		Spec: v1pb.TestObjectSpec{
			F1: 1,
			TestOneof: &v1pb.TestObjectSpec_OneofMsg{
				OneofMsg: &v1pb.M1{
					F1: []v1pb.E1{v1pb.E1_2, v1pb.E1_3},
					F2: map[string]string{"key": "val"},
				},
			},
		},
		Status: v1pb.TestObjectStatus{},
	}
	v2obj := &v2pb.TestObject{}
	err := v1obj.ConvertTo(v2obj)
	assert.NoError(t, err)

	v2Msg, ok := v2obj.Spec.TestOneof.(*v2pb.TestObjectSpec_OneofMsg)
	assert.True(t, ok)
	assert.NotNil(t, v2Msg.OneofMsg)
	assert.Equal(t, []v2pb.E1{v2pb.E1_2, v2pb.E1_3}, v2Msg.OneofMsg.F1)
	assert.Equal(t, map[string]string{"key": "val"}, v2Msg.OneofMsg.F2)

	v1objres := &v1pb.TestObject{}
	err = v1objres.ConvertFrom(v2obj)
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(v1obj, v1objres))
}

func TestConvertOneofNil(t *testing.T) {
	v1obj := &v1pb.TestObject{
		Spec: v1pb.TestObjectSpec{
			F1: 1,
		},
		Status: v1pb.TestObjectStatus{},
	}
	v2obj := &v2pb.TestObject{}
	err := v1obj.ConvertTo(v2obj)
	assert.NoError(t, err)
	assert.Nil(t, v2obj.Spec.TestOneof)

	v1objres := &v1pb.TestObject{}
	err = v1objres.ConvertFrom(v2obj)
	assert.NoError(t, err)
	assert.Nil(t, v1objres.Spec.TestOneof)
}

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
