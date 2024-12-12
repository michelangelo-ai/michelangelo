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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func returnNil() error {
	return nil
}

func TestUnifiedLog(t *testing.T) {
	type args struct {
		ctx    context.Context
		logger interface{}
		msg    string
		err    error
	}
	message := "This is a test message for "
	errSample := errors.New("Test Error")
	rawCtx := context.Background()
	logNil := returnNil()
	args0 := args{ctx: rawCtx, logger: logNil, msg: message + "raw log", err: returnNil()}

	log1 := GetServiceLoggerOrPanic()
	log1.Infow("This is test log", "Id", "123", "Name", "daniel yao")
	args1 := args{ctx: rawCtx, logger: log1, msg: message + "sugar log with no error", err: returnNil()}

	log11 := GetServiceLoggerOrPanic()
	log11.Infow("This is test log", "Id", "123", "Name", "daniel yao")
	args11 := args{ctx: rawCtx, logger: log11, msg: message + "sugar log with sample error", err: errSample}

	log2 := GetLogrLoggerOrPanic()
	args2 := args{ctx: rawCtx, logger: log2, msg: message + "logr log with no error", err: returnNil()}

	log22 := GetLogrLoggerOrPanic()
	args22 := args{ctx: rawCtx, logger: log22, msg: message + "logr log with sample error", err: errSample}

	tests := []struct {
		name    string
		arg     args
		wantErr error
	}{
		{name: "nil log input",
			arg:     args0,
			wantErr: returnNil(),
		},
		{name: "sugar log",
			arg:     args1,
			wantErr: returnNil(),
		},
		{name: "logr log",
			arg:     args2,
			wantErr: returnNil(),
		},
		{name: "sugar log error",
			arg:     args11,
			wantErr: returnNil(),
		},
		{name: "logr log error",
			arg:     args22,
			wantErr: returnNil(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualErr := returnNil()
			if tt.arg.err != nil {
				actualErr = UnifiedLogError(tt.arg.ctx, tt.arg.logger, tt.arg.msg, tt.arg.err)
			} else {
				actualErr = UnifiedLogInfo(tt.arg.ctx, tt.arg.logger, tt.arg.msg)
			}
			assert.Equal(t, tt.wantErr, actualErr)
		})
	}
}
