package ext

import (
	"fmt"
	"go.uber.org/zap"
)

// ZapError zap error related fields
func ZapError(err error) []zap.Field {
	return []zap.Field{
		zap.Error(err),
		ZapType("error_type", err),
	}
}

// ZapType zap field for the value type
func ZapType(key string, value any) zap.Field {
	return zap.String(key, fmt.Sprintf("%T", value))
}

// ErrorFields returns an array of structured log fields in []interface{} format.
func ErrorFields(err error) []interface{} {
	return []interface{}{
		"error", err.Error(),
		"error_type", TypeString(err),
	}
}

// TypeString returns the type of a value as a string.
func TypeString(value any) string {
	return fmt.Sprintf("%T", value)
}
