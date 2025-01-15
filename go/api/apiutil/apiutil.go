package apiutil

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"regexp"
	"strings"
)

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

// ToSnakeCase converts a string from CamelCase to snake_case
func ToSnakeCase(camelStr string) string {
	snake := matchFirstCap.ReplaceAllString(camelStr, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

// IsNotFoundError checks if the error is not found error
func IsNotFoundError(err error) bool {
	if strings.Contains(err.Error(), "not found") {
		return true
	} else if e, ok := status.FromError(err); ok {
		return e.Code() == codes.NotFound
	}
	return false
}