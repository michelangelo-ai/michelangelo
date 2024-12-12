// Copyright (c) 2022 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
)

// UnifiedLogInfo logs an info log to MA unified log for customers to debug their jobs
func UnifiedLogInfo(ctx context.Context, logger interface{}, message string) error {

	return unifiedLog(ctx, logger, message, nil)
}

// UnifiedLogError logs an error log to MA unified log for customers to debug their jobs
func UnifiedLogError(ctx context.Context, logger interface{}, message string, err error) error {

	return unifiedLog(ctx, logger, message, err)
}

func unifiedLog(ctx context.Context, logger interface{}, message string, err error) error {

	serviceLogger := GetLogrLoggerOrPanic()

	//TODO: Deprecate and Clean up zap sugar log since we are not using it anymore.
	sugarLog := GetServiceLoggerOrPanic()
	logrLogger := GetLogrLoggerOrPanic()

	loggerType := reflect.TypeOf(logger)

	if loggerType == reflect.TypeOf(sugarLog) {
		sugarLog = logger.(*zap.SugaredLogger)
		if err == nil {
			sugarLog.Infow(message, zap.Bool(UnifiedLogKey, true))
		} else {
			sugarLog.With("error", err)
			sugarLog.Errorw(message, zap.Bool(UnifiedLogKey, true))
		}
		return nil
	}

	if loggerType == reflect.TypeOf(logrLogger) {
		logrLogger = logger.(logr.Logger)
		if err == nil {
			logrLogger.Info(message, UnifiedLogKey, true)
		} else {
			logrLogger.Error(err, message, UnifiedLogKey, true)
		}
		return nil

	}

	loggerBytes, err := json.Marshal(logger)
	if err != nil {
		return err
	}
	loggerJSON := string(loggerBytes)

	serviceLogger.Info("Unknown format log Intended to MA Unified Log", "log content", loggerJSON)
	return nil

}
