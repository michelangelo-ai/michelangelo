package v2_ext_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v2_ext "github.com/michelangelo-ai/michelangelo/proto/api/v2_ext"
)

func TestDataSchemaExtValidation(t *testing.T) {
	t.Run("DataSchemaItem_RequiredFields", func(t *testing.T) {
		// Test that required fields are validated
		item := &v2_ext.DataSchemaItem{}

		// Should fail with empty name
		err := item.Validate("")
		assert.Error(t, err, "Should fail when name is empty")
		assert.Contains(t, err.Error(), "name")
		assert.Contains(t, err.Error(), "is required")

		// Should fail with invalid name pattern
		item.Name = "123invalid" // Starts with number
		item.DataType = v2_ext.DataType_DATA_TYPE_STRING
		err = item.Validate("")
		assert.Error(t, err, "Should fail with invalid name pattern")
		assert.Contains(t, err.Error(), "must match pattern")

		// Should pass with valid name
		item.Name = "valid_field_name"
		err = item.Validate("")
		assert.NoError(t, err, "Should pass with valid name and data type")
	})

	t.Run("DataSchemaItem_NameValidation", func(t *testing.T) {
		item := &v2_ext.DataSchemaItem{
			DataType: v2_ext.DataType_DATA_TYPE_INT,
		}

		// Test empty name fails
		item.Name = ""
		err := item.Validate("")
		assert.Error(t, err, "Should fail with empty name")
		assert.Contains(t, err.Error(), "is required")

		// Test invalid pattern fails
		item.Name = "123invalid" // Starts with number
		err = item.Validate("")
		assert.Error(t, err, "Should fail with invalid pattern")
		assert.Contains(t, err.Error(), "must match pattern")

		// Test valid name passes
		item.Name = "valid_name"
		err = item.Validate("")
		assert.NoError(t, err, "Should pass with valid name")
	})

	t.Run("DataSchemaItem_ShapeValidation", func(t *testing.T) {
		item := &v2_ext.DataSchemaItem{
			Name:     "tensor_field",
			DataType: v2_ext.DataType_DATA_TYPE_FLOAT,
		}

		// Test valid shape values
		item.Shape = []int32{10, 20, 30}
		err := item.Validate("")
		assert.NoError(t, err, "Should pass with valid shape values")

		// Test shape with negative value (should fail)
		item.Shape = []int32{10, -1, 30}
		err = item.Validate("")
		assert.Error(t, err, "Should fail with negative shape value")
		assert.Contains(t, err.Error(), "shape")
		assert.Contains(t, err.Error(), "must be greater than 0")

		// Test shape with value exceeding max
		item.Shape = []int32{10, 20000, 30}
		err = item.Validate("")
		assert.Error(t, err, "Should fail with shape value exceeding 10000")
		assert.Contains(t, err.Error(), "must be less than 10000")
	})

	t.Run("DirectValidation", func(t *testing.T) {
		// Test direct validation of DataSchemaItem
		item := &v2_ext.DataSchemaItem{
			Name:     "test_field",
			DataType: v2_ext.DataType_DATA_TYPE_STRING,
		}

		err := item.Validate("")
		assert.NoError(t, err, "Should validate successfully")

		// Test with invalid data
		invalidItem := &v2_ext.DataSchemaItem{
			Name: "", // Invalid: empty name
		}
		err = invalidItem.Validate("")
		assert.Error(t, err, "Should fail validation")
	})
	
	t.Run("UserInfoValidation", func(t *testing.T) {
		// Test UserInfo validation
		user := &v2_ext.UserInfo{
			Name: "test_user",
		}

		err := user.Validate("")
		assert.NoError(t, err, "Should validate successfully")

		// Test with invalid name
		invalidUser := &v2_ext.UserInfo{
			Name: "", // Invalid: empty name
		}
		err = invalidUser.Validate("")
		assert.Error(t, err, "Should fail validation")
		assert.Contains(t, err.Error(), "is required")

		// Test with invalid pattern
		invalidPatternUser := &v2_ext.UserInfo{
			Name: "123invalid", // Invalid: starts with number
		}
		err = invalidPatternUser.Validate("")
		assert.Error(t, err, "Should fail pattern validation")
		assert.Contains(t, err.Error(), "must match pattern")
	})

	t.Run("PipelineExecutionParametersValidation", func(t *testing.T) {
		// Test valid parameters
		params := &v2_ext.PipelineExecutionParameters{
			ParameterMap: map[string]string{
				"learning_rate": "0.01",
				"batch_size":    "32",
				"epochs":        "100",
			},
		}

		err := params.Validate("")
		assert.NoError(t, err, "Should validate successfully")

		// Test invalid parameter key (starts with number)
		invalidParams := &v2_ext.PipelineExecutionParameters{
			ParameterMap: map[string]string{
				"123invalid": "value",
			},
		}
		err = invalidParams.Validate("")
		assert.Error(t, err, "Should fail validation for invalid key pattern")
		assert.Contains(t, err.Error(), "must match pattern")

		// Test empty parameter key
		invalidParams2 := &v2_ext.PipelineExecutionParameters{
			ParameterMap: map[string]string{
				"": "value",
			},
		}
		err = invalidParams2.Validate("")
		assert.Error(t, err, "Should fail validation for empty key")
		assert.Contains(t, err.Error(), "must match pattern")
	})

	t.Run("CommitInfoValidation", func(t *testing.T) {
		// Test valid commit info
		commit := &v2_ext.CommitInfo{
			GitRef: "abc123def456",
			Branch: "main",
		}

		err := commit.Validate("")
		assert.NoError(t, err, "Should validate successfully")

		// Test valid with branch name
		commit2 := &v2_ext.CommitInfo{
			GitRef: "feature/my-feature",
			Branch: "feature/my-feature",
		}

		err = commit2.Validate("")
		assert.NoError(t, err, "Should validate successfully")

		// Test empty git_ref
		invalidCommit := &v2_ext.CommitInfo{
			GitRef: "",
			Branch: "main",
		}
		err = invalidCommit.Validate("")
		assert.Error(t, err, "Should fail validation for empty git_ref")
		assert.Contains(t, err.Error(), "is required")

		// Test empty branch
		invalidCommit2 := &v2_ext.CommitInfo{
			GitRef: "abc123",
			Branch: "",
		}
		err = invalidCommit2.Validate("")
		assert.Error(t, err, "Should fail validation for empty branch")
		assert.Contains(t, err.Error(), "is required")

		// Test invalid characters in git_ref
		invalidCommit3 := &v2_ext.CommitInfo{
			GitRef: "abc 123",  // space not allowed
			Branch: "main",
		}
		err = invalidCommit3.Validate("")
		assert.Error(t, err, "Should fail validation for invalid characters")
		assert.Contains(t, err.Error(), "must match pattern")
	})

	t.Run("NotificationValidation", func(t *testing.T) {
		// Test valid notification
		notification := &v2_ext.Notification{
			NotificationType: v2_ext.Notification_NOTIFICATION_TYPE_EMAIL,
			EventTypes: []v2_ext.Notification_EventType{
				v2_ext.Notification_EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED,
				v2_ext.Notification_EVENT_TYPE_PIPELINE_RUN_STATE_FAILED,
			},
			ResourceType: v2_ext.Notification_RESOURCE_TYPE_PIPELINE_RUN,
			Emails:       []string{"user@example.com", "admin@company.org"},
		}

		err := notification.Validate("")
		assert.NoError(t, err, "Should validate successfully")

		// Test slack notification
		slackNotification := &v2_ext.Notification{
			NotificationType: v2_ext.Notification_NOTIFICATION_TYPE_SLACK,
			EventTypes: []v2_ext.Notification_EventType{
				v2_ext.Notification_EVENT_TYPE_TRIGGER_RUN_STATE_FAILED,
			},
			ResourceType:      v2_ext.Notification_RESOURCE_TYPE_TRIGGER_RUN,
			SlackDestinations: []string{"#alerts", "#team-notifications"},
		}

		err = slackNotification.Validate("")
		assert.NoError(t, err, "Should validate successfully")

		// Test invalid notification type
		invalidNotification := &v2_ext.Notification{
			NotificationType: v2_ext.Notification_NOTIFICATION_TYPE_INVALID,
			EventTypes: []v2_ext.Notification_EventType{
				v2_ext.Notification_EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED,
			},
			ResourceType: v2_ext.Notification_RESOURCE_TYPE_PIPELINE_RUN,
		}
		err = invalidNotification.Validate("")
		assert.Error(t, err, "Should fail validation for invalid notification type")
		assert.Contains(t, err.Error(), "is required")

		// Test invalid email format
		invalidEmail := &v2_ext.Notification{
			NotificationType: v2_ext.Notification_NOTIFICATION_TYPE_EMAIL,
			EventTypes: []v2_ext.Notification_EventType{
				v2_ext.Notification_EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED,
			},
			ResourceType: v2_ext.Notification_RESOURCE_TYPE_PIPELINE_RUN,
			Emails:       []string{"invalid-email"},
		}
		err = invalidEmail.Validate("")
		assert.Error(t, err, "Should fail validation for invalid email format")
		assert.Contains(t, err.Error(), "must match pattern")
	})
}
