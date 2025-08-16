package logging_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/michelangelo-ai/michelangelo/go/logging"
	"github.com/michelangelo-ai/michelangelo/proto/test/kubeproto"
)

func TestMarshalToStringForLogging_EndToEndIntegration(t *testing.T) {
	t.Run("real protobuf struct with sensitive field from project_ut.proto", func(t *testing.T) {
		// Clean up after test
		defer logging.ClearSensitiveFields()
		
		// This simulates what the kubeyarpc generator would do:
		// Register the field that has [(michelangelo.api.sensitive) = true] in the .proto file
		logging.RegisterSensitiveField("sensitiveField")
		
		// Create actual protobuf struct from project_ut.proto
		projectSpec := &kubeproto.ProjectSpec{
			Meta: &kubeproto.Metadata{
				Name:      "test-project",
				ProjectId: "proj-123",
			},
			SensitiveField: "secret-api-key-data",
			DataType:       kubeproto.DATA_TYPE_NUMERIC,
		}
		
		// Test the end-to-end logging flow
		result := logging.MarshalToStringForLogging(projectSpec)
		
		// Should contain non-sensitive fields
		assert.Contains(t, result, `"name":"test-project"`)
		assert.Contains(t, result, `"projectId":"proj-123"`)
		assert.Contains(t, result, `"dataType":2`)
		
		// Should redact the sensitive field
		assert.Contains(t, result, `"sensitiveField":"[REDACTED]"`)
		assert.NotContains(t, result, "secret-api-key-data")
	})
	
	t.Run("protobuf struct without sensitive field registration", func(t *testing.T) {
		// Clean up after test
		defer logging.ClearSensitiveFields()
		
		// Do NOT register the sensitive field - simulate what happens when
		// the protobuf field doesn't have [(michelangelo.api.sensitive) = true]
		
		projectSpec := &kubeproto.ProjectSpec{
			Meta: &kubeproto.Metadata{
				Name:      "test-project",
				ProjectId: "proj-123",
			},
			SensitiveField: "this-should-not-be-redacted",
			DataType:       kubeproto.DATA_TYPE_BOOLEAN,
		}
		
		result := logging.MarshalToStringForLogging(projectSpec)
		
		// Should contain all fields including the sensitive one
		assert.Contains(t, result, `"name":"test-project"`)
		assert.Contains(t, result, `"projectId":"proj-123"`)
		assert.Contains(t, result, `"dataType":4`)
		assert.Contains(t, result, `"sensitiveField":"this-should-not-be-redacted"`)
		
		// Should NOT redact the field since it's not registered
		assert.NotContains(t, result, `"[REDACTED]"`)
	})
	
	t.Run("verify protobuf JSON field names match expectations", func(t *testing.T) {
		// This test verifies that the protobuf generates the expected JSON field names
		// which our registration system depends on
		projectSpec := &kubeproto.ProjectSpec{
			Meta: &kubeproto.Metadata{
				Name:      "test",
				ProjectId: "123",
			},
			SensitiveField: "secret",
			DataType:       kubeproto.DATA_TYPE_ARRAY,
		}
		
		// Use standard JSON marshaling to verify field names (no redaction)
		result := logging.MarshalToString(projectSpec)
		
		// Verify the JSON field names match what we expect to register
		assert.Contains(t, result, `"sensitiveField":"secret"`)
		assert.Contains(t, result, `"projectId":"123"`)
		assert.Contains(t, result, `"dataType":"DATA_TYPE_ARRAY"`)
	})
}

// Mock implementations for testing
type mockAPIHandler struct{}

func (m *mockAPIHandler) Create(ctx interface{}, obj interface{}, opts interface{}) error {
	return nil
}

func (m *mockAPIHandler) Get(ctx interface{}, namespace, name string, opts interface{}, result interface{}) error {
	return nil
}

func (m *mockAPIHandler) Update(ctx interface{}, obj interface{}, opts interface{}) error {
	return nil
}

func (m *mockAPIHandler) Delete(ctx interface{}, obj interface{}, opts interface{}) error {
	return nil
}

func (m *mockAPIHandler) DeleteCollection(ctx interface{}, obj interface{}, namespace string, deleteOpts interface{}, listOpts interface{}) error {
	return nil
}

func (m *mockAPIHandler) List(ctx interface{}, namespace string, opts interface{}, optsExt interface{}, result interface{}) error {
	return nil
}

func (m *mockAPIHandler) Patch(ctx interface{}, obj interface{}, patch interface{}, opts interface{}) error {
	return nil
}

type mockAuditLog struct{}

func (m *mockAuditLog) Emit(ctx interface{}, event *logging.AuditLogEvent) {}

type mockAuth struct{}

func (m *mockAuth) UserAuthenticated(ctx interface{}) (bool, error) {
	return true, nil
}

func (m *mockAuth) UserAuthorized(ctx interface{}, project string, action auth.Action, resource string) (bool, error) {
	return true, nil
}