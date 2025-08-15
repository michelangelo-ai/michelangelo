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
// Only fields explicitly registered with RegisterSensitiveField will be redacted.
// This is specifically designed for protobuf fields marked with [(michelangelo.api.sensitive) = true].
func MarshalToStringForLogging(v interface{}) string {
	filtered := filterSensitiveFields(v)
	b, _ := json.Marshal(filtered)
	return string(b)
}

// registeredSensitiveFields contains field names that have been explicitly marked as sensitive
// This is used for fields that had [(michelangelo.api.sensitive) = true] in protobuf
var registeredSensitiveFields = make(map[string]bool)

// RegisterSensitiveField registers a field name as sensitive for logging purposes
// This is useful for protobuf fields marked with [(michelangelo.api.sensitive) = true]
//
// Example usage:
//
//	// For a protobuf field: repeated RiskAssessmentCategory high_risk_assessment = 18 [(michelangelo.api.sensitive) = true];
//	logging.RegisterSensitiveField("high_risk_assessment")
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

// isSensitiveField determines if a field should be considered sensitive
// Only fields explicitly registered (from protobuf [(michelangelo.api.sensitive) = true]) are sensitive
func isSensitiveField(fieldName string) bool {
	// Only check if field is explicitly registered as sensitive
	// This handles fields marked with [(michelangelo.api.sensitive) = true] in protobuf
	return registeredSensitiveFields[fieldName]
}

// isSensitiveMapKey determines if a map key indicates sensitive data
// Only keys explicitly registered (from protobuf [(michelangelo.api.sensitive) = true]) are sensitive
func isSensitiveMapKey(key string) bool {
	// Only check if key is explicitly registered as sensitive
	return registeredSensitiveFields[key]
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
			isSensitive := isSensitiveField(fieldName)

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
