package logging

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

// MarshalToString marshals the input message into JSON form and convert into string.
// If marshaling fails, returns an error message string instead of silently ignoring the error.
func MarshalToString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<error marshaling to JSON: %v>", err)
	}
	return string(b)
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
