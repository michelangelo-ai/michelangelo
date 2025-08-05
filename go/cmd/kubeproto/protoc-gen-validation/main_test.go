package main

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dave/dst/decorator"

	testpb "github.com/michelangelo-ai/michelangelo/proto/test/kubeproto"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestGen(t *testing.T) {
	data := testpb.GetProtocReqData()
	resp := generate(data)

	var validationUTFile *pluginpb.CodeGeneratorResponse_File
	for _, f := range resp.GetFile() {
		if strings.HasSuffix(*f.Name, "validation_ut.pb.validation.go") {
			validationUTFile = f
		}
	}

	assert.True(t, validationUTFile != nil)
	goFile := validationUTFile.GetContent()
	_, err := decorator.Parse(goFile)
	assert.NoError(t, err)

	// Test that validation functions are generated
	assert.True(t, strings.Contains(goFile, "func (this *ValidationMsg1) Validate(prefix string) error"))
	assert.True(t, strings.Contains(goFile, "func (this *ValidationMsg2) Validate(prefix string) error"))
	assert.True(t, strings.Contains(goFile, "func (this *ValidationMsg3) Validate(prefix string) error"))

	// Test that extension variables are generated
	assert.True(t, strings.Contains(goFile, "var ValidationMsg1ValidateExt func(*ValidationMsg1) error"))
	assert.True(t, strings.Contains(goFile, "var ValidationMsg2ValidateExt func(*ValidationMsg2) error"))
	assert.True(t, strings.Contains(goFile, "var ValidationMsg3ValidateExt func(*ValidationMsg3) error"))

	// Test that extension calls are generated in validation functions
	assert.True(t, strings.Contains(goFile, "if ValidationMsg1ValidateExt != nil {"))
	assert.True(t, strings.Contains(goFile, "if err := ValidationMsg1ValidateExt(this); err != nil {"))
	assert.True(t, strings.Contains(goFile, "if ValidationMsg2ValidateExt != nil {"))
	assert.True(t, strings.Contains(goFile, "if err := ValidationMsg2ValidateExt(this); err != nil {"))

	// Test that registration functions are generated
	assert.True(t, strings.Contains(goFile, "func RegisterValidationMsg1ValidateExt(f func(*ValidationMsg1) error) {"))
	assert.True(t, strings.Contains(goFile, "ValidationMsg1ValidateExt = f"))
	assert.True(t, strings.Contains(goFile, "func RegisterValidationMsg2ValidateExt(f func(*ValidationMsg2) error) {"))
	assert.True(t, strings.Contains(goFile, "ValidationMsg2ValidateExt = f"))
}

func TestNoDefault(t *testing.T) {
	m1 := testpb.ValidationMsg1{}
	err := m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be a non-zero number"), err)
	m1.F1 = 50
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f3 custom error message"), err)
	m1.F3 = "test"
	m1.F4 = testpb.E1_C
	err = m1.Validate("")
	assert.NoError(t, err)

	m2 := testpb.ValidationMsg2{}
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be a non-zero number"), err)
	m2.F1 = 3
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f3 cannot be empty"), err)
	m2.F3 = "test"
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f4 must be a non-zero value"), err)
	m2.F4 = testpb.E1_B
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f5 cannot be nil"), err)
	m2.F5 = &testpb.ValidationMsg3{}
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f6 must be a non-zero number"), err)
	m2.F6 = 100
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f7 must be a non-zero number"), err)
	m2.F7 = 0.7
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f9 cannot be empty"), err)
	m2.F9 = []int64{100, 200}
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f10 cannot be empty"), err)
	m2.F10 = []byte{'A'}
	err = m2.Validate("")
	assert.NoError(t, err)
}

func TestMinMax(t *testing.T) {
	m1 := testpb.ValidationMsg1{}
	m1.F1 = 1
	m1.F3 = "test"
	err := m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be in the range [10,100]"), err)
	m1.F1 = 101
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be in the range [10,100]"), err)
	m1.F1 = 50
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f4 must be >= 2"), err)
	m1.F4 = 2
	err = m1.Validate("")
	assert.NoError(t, err)

	m2 := testpb.ValidationMsg2{
		F1:  10,
		F3:  "test",
		F4:  testpb.E1_B,
		F5:  &testpb.ValidationMsg3{},
		F6:  1,
		F7:  1.01,
		F8:  1,
		F9:  []int64{1, 2, 3},
		F10: []byte{'A'},
	}
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be <= 5"), err)
	m2.F1 = 5
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f7 custom msg max"), err)
	m2.F7 = -1
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f7 custom msg min"), err)
	m2.F7 = -0.99999
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f8 must be in the range [-0.1,0.5)"), err)
	m2.F8 = 0.5
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f8 must be in the range [-0.1,0.5)"), err)
	m2.F8 = -0.1
	err = m2.Validate("")
	assert.NoError(t, err)
}

func TestMinMaxItems(t *testing.T) {
	m := testpb.ValidationMsg4{
		F1: "test",
		F2: []int32{1},
		F3: []byte{'A'},
		F4: map[int64]string{},
	}

	err := m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f2 must contain at least 2 items"), err)
	m.F2 = []int32{1, 2}
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f4 must contain 1 to 3 items, inclusive"), err)
	m.F4 = map[int64]string{1: "one", 2: "two", 3: "three", 4: "four"}
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f4 must contain 1 to 3 items, inclusive"), err)
	m.F4 = map[int64]string{1: "one", 2: "two", 3: "three"}
	err = m.Validate("")
	assert.NoError(t, err)
}

func TestMinMaxLength(t *testing.T) {
	m := testpb.ValidationMsg4{
		F1: "ab",
		F2: []int32{1, 2},
		F3: []byte{},
		F4: map[int64]string{1: "one", 2: "two", 3: "three"},
	}

	err := m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be 3 to 10 characters long, inclusive"), err)
	m.F1 = "1234567890a"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be 3 to 10 characters long, inclusive"), err)
	m.F1 = "123"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f3 must be 1 to 100 bytes long, inclusive"), err)
	m.F3 = bytes.Repeat([]byte{'A'}, 101)
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f3 must be 1 to 100 bytes long, inclusive"), err)
	m.F3 = bytes.Repeat([]byte{'A'}, 100)
	err = m.Validate("")
	assert.NoError(t, err)
	m.F1 = "1234567890"
	err = m.Validate("")
	assert.NoError(t, err)
}

func TestPattern(t *testing.T) {
	m := testpb.ValidationMsg5{
		F1: "abc",
	}

	err := m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must match regular expression pattern /[ab]+/"), err)
	m.F1 = "aba"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f2 must be a positive number"), err)
	m.F2 = "100"
	err = m.Validate("")
	assert.NoError(t, err)

	m.F1 = "aaba"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must not match regular expression pattern /aa.*/"), err)
	m.F1 = "ababa"
	err = m.Validate("")
	assert.NoError(t, err)
}

func TestIn(t *testing.T) {
	m := testpb.ValidationMsg6{
		F1: "",
		F2: 0,
		F3: 0,
		F4: "a",
		F5: 100,
		F6: 2.3,
	}

	err := m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be either \"a\", or \"b\""), err)
	m.F1 = "b"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f2 custom msg"), err)
	m.F2 = 1
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f3 must be either 1, or 2"), err)
	m.F3 = 2
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f4 cannot be \"a\", nor \"b\""), err)
	m.F4 = "c"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f4 cannot be \"c\""), err)
	m.F4 = "x"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f5 cannot be 100, 200, nor 300"), err)
	m.F5 = 10
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f6 must be either 1.0, or 2.0"), err)
	m.F6 = 2.0
	err = m.Validate("")
	assert.NoError(t, err)
}

func TestWellKnownFormats(t *testing.T) {
	m := testpb.ValidationMsg7{}

	err := m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be a valid UUID"), err)

	m.F1 = "764af75f-040a-4037-883c-4b5eecd0cada"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f2 must be a valid email address"), err)
	m.F2 = "yingz@uber.com"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f3 must be a valid URI"), err)
	m.F3 = "https://uber.com"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f4 must be a valid IPv4 address"), err)
	m.F4 = "127.0.0.1"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f5 must be a valid IPv6 address"), err)
	m.F5 = "127.0.0.1"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f5 must be a valid IPv6 address"), err)
	m.F5 = "::ffff:127.0.0.1"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f6 must be a valid IP address"), err)
	m.F6 = "10.0.0.1"
	err = m.Validate("")
	assert.NoError(t, err)
	m.F6 = "::1"
	err = m.Validate("")
	assert.NoError(t, err)
}

func TestMessageFields(t *testing.T) {
	m := testpb.ValidationMsg8{}

	err := m.Validate("")
	assert.NoError(t, err)

	m.F1 = &testpb.ValidationMsg9{}
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1.f1 must be a non-zero number"), err)

	m.F1.F1 = 1
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1.f2 cannot be nil"), err)

	m.F1.F2 = &testpb.ValidationMsg5{}
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1.f2.f1 must match regular expression pattern /[ab]+/"), err)

	m.F1.F2.F1 = "abab"
	m.F1.F2.F2 = "1"
	err = m.Validate("")
	assert.NoError(t, err)

	m.F2 = &testpb.ValidationMsg5{}
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f2.f1 must match regular expression pattern /[ab]+/"), err)

	m.F2.F1 = "abab"
	m.F2.F2 = "1"
	err = m.Validate("")
	assert.NoError(t, err)
}

func TestListAndMap(t *testing.T) {
	m := testpb.ValidationMsg10{
		F1: []string{"test"},
		F2: nil,
		F3: []int32{0, 1000},
		F4: nil,
	}
	err := m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1[0] must match regular expression pattern /[ab]+/"), err)

	m.F1[0] = "aba"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f2 must contain at least 2 items"), err)

	m.F2 = make(map[int32]string)
	m.F2[0] = "zero"
	m.F2[2] = "two"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f2 key must be in the range [1,5]"), err)

	delete(m.F2, 0)
	m.F2[1] = ""
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f2[1] must match regular expression pattern /[a-z]+/"), err)

	m.F2[1] = "one"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f3[0] custom msg1"), err)

	m.F3[0] = 11
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f3[1] custom msg2"), err)

	m.F3[1] = 100
	err = m.Validate("")
	assert.NoError(t, err)

	m.F4 = []*testpb.ValidationMsg1{}
	err = m.Validate("")
	assert.NoError(t, err)

	m.F4 = []*testpb.ValidationMsg1{nil}
	err = m.Validate("")
	assert.NoError(t, err)

	m.F4 = []*testpb.ValidationMsg1{&testpb.ValidationMsg1{}}
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f4[0].f1 must be a non-zero number"), err)

	m.F4[0] = &testpb.ValidationMsg1{
		F1: 10,
		F3: "test",
		F4: 2,
	}
	err = m.Validate("")
	assert.NoError(t, err)

	m.F5 = make(map[string]*testpb.ValidationMsg1)
	err = m.Validate("")
	assert.NoError(t, err)

	m.F5["a"] = &testpb.ValidationMsg1{}
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f5[a].f1 must be a non-zero number"), err)

	m.F5["a"].F1 = 20
	m.F5["a"].F3 = "aaa"
	m.F5["a"].F4 = 3
	err = m.Validate("")
	assert.NoError(t, err)

	m.F6 = []string{"test", ""}
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f6[0] must be either \"one\", \"two\", or \"three\""), err)
	m.F6[0] = "one"
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f6[1] must be either \"one\", \"two\", or \"three\""), err)
	m.F6[1] = "three"
	err = m.Validate("")
	assert.NoError(t, err)

	m.F7 = make(map[string][]byte)
	m.F7["test"] = []byte("test")
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f7 key must be a valid UUID"), err)
	delete(m.F7, "test")
	err = m.Validate("")
	assert.NoError(t, err)
	m.F7["16503fc0-42ea-4e82-b1e4-f2e00bcb12cc"] = []byte{}
	err = m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f7[16503fc0-42ea-4e82-b1e4-f2e00bcb12cc] must be at least 3 bytes long"), err)
	m.F7["16503fc0-42ea-4e82-b1e4-f2e00bcb12cc"] = []byte("test")
	err = m.Validate("")
	assert.NoError(t, err)
}

func TestOneof(t *testing.T) {
	m := testpb.ValidationMsg11{}
	err := m.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "one field in oneof test(f1,f2) must be set"), err)

	m.Test = &testpb.ValidationMsg11_F1{}
	err = m.Validate("")
	assert.NoError(t, err)

	m1 := testpb.ValidationMsg12{}
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "one field in oneof test_oneof(f1,f2) must be set"), err)
	m1.TestOneof = &testpb.ValidationMsg12_F2{}
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f2 cannot be nil"), err)

	m1.TestOneof = &testpb.ValidationMsg12_F2{F2: &testpb.ValidationMsg1{
		F1: 10,
		F3: "test",
		F4: 2,
	}}
	err = m.Validate("")
	assert.NoError(t, err)
}

// Test extension functionality
func TestValidationExtension(t *testing.T) {
	// Test basic extension registration and execution
	m1 := testpb.ValidationMsg1{
		F1: 50,
		F3: "test",
		F4: testpb.E1_C,
	}

	// Should pass without extension
	err := m1.Validate("")
	assert.NoError(t, err)

	// Register extension that always fails
	testpb.RegisterValidationMsg1ValidateExt(func(msg *testpb.ValidationMsg1) error {
		return status.Error(codes.InvalidArgument, "extension validation failed")
	})

	// Should now fail due to extension
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "extension validation failed"), err)

	// Register extension that checks custom business logic
	testpb.RegisterValidationMsg1ValidateExt(func(msg *testpb.ValidationMsg1) error {
		if msg.F1 > 75 {
			return status.Error(codes.InvalidArgument, "f1 cannot exceed 75 in extension validation")
		}
		return nil
	})

	// Should pass with F1 = 50
	err = m1.Validate("")
	assert.NoError(t, err)

	// Should fail with F1 = 80
	m1.F1 = 80
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 cannot exceed 75 in extension validation"), err)

	// Clear extension
	testpb.RegisterValidationMsg1ValidateExt(nil)

	// Should pass again without extension
	err = m1.Validate("")
	assert.NoError(t, err)
}

func TestValidationExtensionWithComplexMessage(t *testing.T) {
	// Test extension with more complex message
	m2 := testpb.ValidationMsg2{
		F1:  3,
		F3:  "test",
		F4:  testpb.E1_B,
		F5:  &testpb.ValidationMsg3{},
		F6:  100,
		F7:  0.5,
		F9:  []int64{100, 200},
		F10: []byte{'A'},
	}

	// Should pass built-in validation
	err := m2.Validate("")
	assert.NoError(t, err)

	// Register extension that validates array contents
	testpb.RegisterValidationMsg2ValidateExt(func(msg *testpb.ValidationMsg2) error {
		if len(msg.F9) > 0 && msg.F9[0] < 50 {
			return status.Error(codes.InvalidArgument, "first element of f9 must be >= 50")
		}
		return nil
	})

	// Should pass with current values
	err = m2.Validate("")
	assert.NoError(t, err)

	// Should fail with small first element
	m2.F9[0] = 25
	err = m2.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "first element of f9 must be >= 50"), err)

	// Clear extension
	testpb.RegisterValidationMsg2ValidateExt(nil)
}

func TestMultipleValidationExtensions(t *testing.T) {
	// Test that extensions work independently for different message types
	m1 := testpb.ValidationMsg1{
		F1: 50,
		F3: "test",
		F4: testpb.E1_C,
	}

	m3 := testpb.ValidationMsg3{}

	// Register extensions for both types
	testpb.RegisterValidationMsg1ValidateExt(func(msg *testpb.ValidationMsg1) error {
		if msg.F3 == "forbidden" {
			return status.Error(codes.InvalidArgument, "f3 cannot be 'forbidden'")
		}
		return nil
	})

	testpb.RegisterValidationMsg3ValidateExt(func(msg *testpb.ValidationMsg3) error {
		return status.Error(codes.InvalidArgument, "ValidationMsg3 extension always fails")
	})

	// m1 should pass
	err := m1.Validate("")
	assert.NoError(t, err)

	// m3 should fail due to its extension
	err = m3.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "ValidationMsg3 extension always fails"), err)

	// m1 should fail when f3 is "forbidden"
	m1.F3 = "forbidden"
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f3 cannot be 'forbidden'"), err)

	// Clear extensions
	testpb.RegisterValidationMsg1ValidateExt(nil)
	testpb.RegisterValidationMsg3ValidateExt(nil)

	// Both should pass now (m1 might still fail built-in validation, but not extension)
	err = m3.Validate("")
	assert.NoError(t, err)
}

func TestValidationExtensionErrorPropagation(t *testing.T) {
	// Test that extension errors are properly propagated and don't interfere with built-in validation
	m1 := testpb.ValidationMsg1{
		F1: 5, // This will fail built-in validation (min is 10)
		F3: "test",
		F4: testpb.E1_C,
	}

	// Should fail built-in validation first
	err := m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be in the range [10,100]"), err)

	// Register extension
	testpb.RegisterValidationMsg1ValidateExt(func(msg *testpb.ValidationMsg1) error {
		return status.Error(codes.InvalidArgument, "extension validation error")
	})

	// Should still fail built-in validation first (extension not reached)
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "f1 must be in the range [10,100]"), err)

	// Fix built-in validation issue
	m1.F1 = 50

	// Now should fail extension validation
	err = m1.Validate("")
	assert.Error(t, err)
	assert.Equal(t, status.Error(codes.InvalidArgument, "extension validation error"), err)

	// Clear extension
	testpb.RegisterValidationMsg1ValidateExt(nil)

	// Should pass now
	err = m1.Validate("")
	assert.NoError(t, err)
}

func TestValidationExtensionNilSafety(t *testing.T) {
	// Test that nil extensions are handled safely
	m1 := testpb.ValidationMsg1{
		F1: 50,
		F3: "test",
		F4: testpb.E1_C,
	}

	// Explicitly set extension to nil
	testpb.RegisterValidationMsg1ValidateExt(nil)

	// Should pass without issues
	err := m1.Validate("")
	assert.NoError(t, err)

	// Register and then clear extension
	testpb.RegisterValidationMsg1ValidateExt(func(msg *testpb.ValidationMsg1) error {
		return nil
	})

	err = m1.Validate("")
	assert.NoError(t, err)

	// Clear extension again
	testpb.RegisterValidationMsg1ValidateExt(nil)

	err = m1.Validate("")
	assert.NoError(t, err)
}
