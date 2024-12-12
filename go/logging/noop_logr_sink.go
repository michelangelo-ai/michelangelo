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

import "github.com/go-logr/logr"

var _ logr.LogSink = &NoopLogSink{}

// NoopLogSink is the log sink that does nothing. It should only ever be used for testing purposes.
type NoopLogSink struct{}

// Init initializes the logger. In this case, it is a no-op.
func (n *NoopLogSink) Init(info logr.RuntimeInfo) {
}

// Enabled determines whether the logger is enabled for the provided level. In this implementation
// we will always return false.
func (n *NoopLogSink) Enabled(level int) bool {
	return false
}

// Info logs an informational log. This implementation will do nothing.
func (n *NoopLogSink) Info(level int, msg string, keysAndValues ...interface{}) {
}

// Error logs an error log. This implementation will do nothing.
func (n *NoopLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
}

// WithValues adds tags to annotate the log. This implementation will do nothing.
func (n *NoopLogSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return n
}

// WithName adds a name to annotate the log. This implementation will do nothing.
func (n *NoopLogSink) WithName(name string) logr.LogSink {
	return n
}
