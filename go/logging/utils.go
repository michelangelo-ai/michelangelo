package logging

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

// MarshalToString marshals the input message into JSON form and conversion into string.
func MarshalToString(v interface{}) string {
	b, _ := json.Marshal(v)
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
