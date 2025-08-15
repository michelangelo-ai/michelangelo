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
// Fields marked with `json:"-"` or containing "sensitive" in their json tag will be redacted.
func MarshalToStringForLogging(v interface{}) string {
	filtered := filterSensitiveFields(v)
	b, _ := json.Marshal(filtered)
	return string(b)
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
			isSensitive := false
			
			// Check for custom "sensitive" tag
			sensitiveTag := fieldType.Tag.Get("sensitive")
			if sensitiveTag == "true" {
				isSensitive = true
			}
			
			// Check JSON tag for sensitive marker
			for _, part := range tagParts {
				if strings.Contains(strings.ToLower(part), "sensitive") {
					isSensitive = true
					break
				}
			}
			
			// Also check field name for sensitive keywords
			fieldNameLower := strings.ToLower(fieldName)
			if strings.Contains(fieldNameLower, "password") || 
			   strings.Contains(fieldNameLower, "secret") || 
			   strings.Contains(fieldNameLower, "token") || 
			   strings.Contains(fieldNameLower, "key") {
				isSensitive = true
			}
			
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
			result[keyStr] = filterSensitiveFields(val.MapIndex(key).Interface())
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
