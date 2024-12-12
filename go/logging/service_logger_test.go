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
	"time"

	"testing"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	jaeger "github.com/uber/jaeger-client-go"
	"github.com/uber/tchannel-go/thrift"
	"google.golang.org/grpc/metadata"
)

func TestGetTraceID(t *testing.T) {
	jaegerTrace := jaeger.TraceID{Low: 1}
	testCtx, cancel := thrift.NewContext(5 * time.Second)
	defer cancel()

	traceIDSpanContext := jaeger.NewSpanContext(jaegerTrace, 1, 1, false, nil)
	tracer, closer := jaeger.NewTracer(
		"x",
		jaeger.NewConstSampler(false),
		jaeger.NewLoggingReporter(jaeger.StdLogger),
	)
	defer closer.Close()
	span := tracer.StartSpan("span", jaeger.SelfRef(traceIDSpanContext))
	defer span.Finish()
	contextWithSpan := thrift.Wrap(opentracing.ContextWithSpan(testCtx, span))

	tests := []struct {
		name    string
		ctx     context.Context
		traceID string
	}{

		{
			name:    "success, generate trace id from span",
			ctx:     contextWithSpan,
			traceID: jaegerTrace.String(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultTraceID := GetTraceID(tt.ctx)
			assert.Equal(t, tt.traceID, resultTraceID)
		})
	}
}

func TestGetMaUUID(t *testing.T) {

	metaData := metadata.Pairs(
		MaUUIDKey, "canvas123",
	)

	contextWithMeta := metadata.NewIncomingContext(context.Background(), metaData)

	jaegerTrace := jaeger.TraceID{Low: 1}
	testCtx, cancel := thrift.NewContext(5 * time.Second)
	defer cancel()

	traceIDSpanContext := jaeger.NewSpanContext(jaegerTrace, 1, 1, false, nil)
	tracer, closer := jaeger.NewTracer(
		"x",
		jaeger.NewConstSampler(false),
		jaeger.NewLoggingReporter(jaeger.StdLogger),
	)
	defer closer.Close()
	span := tracer.StartSpan("span", jaeger.SelfRef(traceIDSpanContext))
	defer span.Finish()
	contextWithSpan := thrift.Wrap(opentracing.ContextWithSpan(testCtx, span))

	tests := []struct {
		name   string
		ctx    context.Context
		maUUID string
	}{
		{
			name:   "success, get trace id from span",
			ctx:    contextWithSpan,
			maUUID: jaegerTrace.String(),
		},
		{
			name:   "success, get canvas uuid from meta",
			ctx:    contextWithMeta,
			maUUID: "canvas123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultCanvasUUID := GetMaUUID(tt.ctx)
			assert.Equal(t, tt.maUUID, resultCanvasUUID)
		})
	}
}

func TestGetActor(t *testing.T) {

	metaData := metadata.Pairs(
		GRPCUserKey, "daniel.yao@uber.com",
	)

	contextWithMeta := metadata.NewIncomingContext(context.Background(), metaData)

	tests := []struct {
		name  string
		ctx   context.Context
		email string
	}{

		{
			name:  "success, get email from meta",
			ctx:   contextWithMeta,
			email: "daniel.yao@uber.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultEmail := GetActor(tt.ctx)
			assert.Equal(t, tt.email, resultEmail)
		})
	}
}

func TestGetCaller(t *testing.T) {

	metaData := metadata.Pairs(
		GRPCCallerKey, "ml-code",
	)

	contextWithMeta := metadata.NewIncomingContext(context.Background(), metaData)

	tests := []struct {
		name   string
		ctx    context.Context
		caller string
	}{

		{
			name:   "success, get caller from meta",
			ctx:    contextWithMeta,
			caller: "ml-code",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultCaller := GetCaller(tt.ctx)
			assert.Equal(t, tt.caller, resultCaller)
		})
	}
}

func TestGetSource(t *testing.T) {

	metaData := metadata.Pairs(
		GRPCSourceKey, "web",
	)

	contextWithMeta := metadata.NewIncomingContext(context.Background(), metaData)

	tests := []struct {
		name   string
		ctx    context.Context
		source string
	}{

		{
			name:   "success, get source from meta",
			ctx:    contextWithMeta,
			source: "web",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultSource := GetSource(tt.ctx)
			assert.Equal(t, tt.source, resultSource)
		})
	}
}
