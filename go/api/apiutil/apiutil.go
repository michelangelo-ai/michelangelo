package apiutil

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
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
// It handles grpc not found error and k8s client not found error
func IsNotFoundError(err error) bool {
	if e, ok := status.FromError(err); ok {
		return e.Code() == codes.NotFound
	}
	// Handle Kubernetes REST client errors
	if errors.IsNotFound(err) {
		return true
	}
	return false
}