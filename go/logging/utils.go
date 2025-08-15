package logging

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

// MarshalToString marshals the input message into JSON form and convert into string.
func MarshalToString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// MarshalToStringForLogging marshals the input message to JSON while filtering out sensitive fields.
// Fields containing sensitive keywords or marked as sensitive will be redacted.
func MarshalToStringForLogging(v interface{}) string {
	filtered := filterSensitiveFields(v)
	b, _ := json.Marshal(filtered)
	return string(b)
}

// sensitiveFieldPatterns contains patterns that indicate a field contains sensitive data
var sensitiveFieldPatterns = []string{
	"password",
	"secret",
	"token",
	"key",
	"credential",
	"auth",
	"private",
}

// registeredSensitiveFields contains field names that have been explicitly marked as sensitive
// This can be used to register fields that had [(michelangelo.api.sensitive) = true] in protobuf
var registeredSensitiveFields = make(map[string]bool)

// RegisterSensitiveField registers a field name as sensitive for logging purposes
// This is useful for protobuf fields marked with [(michelangelo.api.sensitive) = true]
//
// Example usage:
//   // For a protobuf field: repeated RiskAssessmentCategory high_risk_assessment = 18 [(michelangelo.api.sensitive) = true];
//   logging.RegisterSensitiveField("high_risk_assessment")
func RegisterSensitiveField(fieldName string) {
	registeredSensitiveFields[fieldName] = true
}

// UnregisterSensitiveField removes a field name from the sensitive fields registry
func UnregisterSensitiveField(fieldName string) {
	delete(registeredSensitiveFields, fieldName)
}

// ClearSensitiveFields clears all registered sensitive fields
func ClearSensitiveFields() {
	registeredSensitiveFields = make(map[string]bool)
}

// isSensitiveField determines if a field should be considered sensitive based on various criteria
func isSensitiveField(fieldName string, fieldType reflect.StructField) bool {
	// Check if field is explicitly registered as sensitive
	// This handles fields marked with [(michelangelo.api.sensitive) = true] in protobuf
	if registeredSensitiveFields[fieldName] {
		return true
	}

	// Check JSON tag for sensitive marker
	jsonTag := fieldType.Tag.Get("json")
	if jsonTag != "" {
		tagParts := strings.Split(jsonTag, ",")
		for _, part := range tagParts {
			if strings.Contains(strings.ToLower(part), "sensitive") {
				return true
			}
		}
	}

	// Check for custom "sensitive" tag (for manually tagged fields)
	if sensitiveTag := fieldType.Tag.Get("sensitive"); sensitiveTag == "true" {
		return true
	}

	// Check field name for sensitive keywords
	fieldNameLower := strings.ToLower(fieldName)
	for _, pattern := range sensitiveFieldPatterns {
		if strings.Contains(fieldNameLower, pattern) {
			return true
		}
	}
	
	return false
}

// isSensitiveMapKey determines if a map key indicates sensitive data
func isSensitiveMapKey(key string) bool {
	// Check if key is explicitly registered as sensitive
	if registeredSensitiveFields[key] {
		return true
	}

	keyLower := strings.ToLower(key)
	for _, pattern := range sensitiveFieldPatterns {
		if strings.Contains(keyLower, pattern) {
			return true
		}
	}
	return false
}

// filterSensitiveFields recursively filters out sensitive fields from a struct
func filterSensitiveFields(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	val := reflect.ValueOf(v)
	typ := reflect.TypeOf(v)

	// Handle pointers
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		return filterSensitiveFields(val.Elem().Interface())
	}

	// Handle structs
	if val.Kind() == reflect.Struct {
		result := make(map[string]interface{})

		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			fieldType := typ.Field(i)

			// Skip unexported fields
			if !field.CanInterface() {
				continue
			}

			// Get JSON tag
			jsonTag := fieldType.Tag.Get("json")
			if jsonTag == "" {
				jsonTag = fieldType.Name
			}

			// Parse JSON tag to get field name and options
			tagParts := strings.Split(jsonTag, ",")
			fieldName := tagParts[0]

			// Skip if field is marked to be ignored in JSON
			if fieldName == "-" {
				continue
			}

			// Check if field is marked as sensitive
			isSensitive := isSensitiveField(fieldName, fieldType)

			if isSensitive {
				result[fieldName] = "[REDACTED]"
			} else {
				// Recursively filter nested structs
				result[fieldName] = filterSensitiveFields(field.Interface())
			}
		}
		return result
	}

	// Handle slices
	if val.Kind() == reflect.Slice {
		result := make([]interface{}, val.Len())
		for i := 0; i < val.Len(); i++ {
			result[i] = filterSensitiveFields(val.Index(i).Interface())
		}
		return result
	}

	// Handle maps
	if val.Kind() == reflect.Map {
		result := make(map[string]interface{})
		for _, key := range val.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())

			// Check if map key contains sensitive keywords
			isSensitive := isSensitiveMapKey(keyStr)

			if isSensitive {
				result[keyStr] = "[REDACTED]"
			} else {
				result[keyStr] = filterSensitiveFields(val.MapIndex(key).Interface())
			}
		}
		return result
	}

	// For all other types, return as is
	return v
}

// GetLogrLoggerOrPanic returns a logr logger
func GetLogrLoggerOrPanic() logr.Logger {
	zc := zap.NewProductionConfig()
	z, err := zc.Build()
	if err != nil {
		panic(fmt.Sprintf("Can't get logr logger, error %s", err.Error()))
	}
	return zapr.NewLogger(z)
}
