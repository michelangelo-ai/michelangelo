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
	"fmt"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/google/uuid"
	opentracing "github.com/opentracing/opentracing-go"
	jaeger "github.com/uber/jaeger-client-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/metadata"
	"strings"
)

// GetServiceLoggerOrPanic returns a sugared logger that uses for development.
func GetServiceLoggerOrPanic() *zap.SugaredLogger {
	zapper, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("Can't get zap suggar logger, error %s", err.Error()))
	}
	return zapper.Sugar()
}

// GetLogrLoggerOrPanic returns a logr logger that uses for development.
func GetLogrLoggerOrPanic() logr.Logger {
	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zapcore.Level(-2))
	z, err := zc.Build()
	if err != nil {
		panic(fmt.Sprintf("Can't get logr logger, error %s", err.Error()))
	}
	return zapr.NewLogger(z)
}

// GetTraceID returns the jaeger trace ID from the context
func GetTraceID(ctx context.Context) string {
	opentracingSpan := opentracing.SpanFromContext(ctx)
	if jaegerSpan, ok := opentracingSpan.(*jaeger.Span); ok {
		opentracingCtx := jaegerSpan.Context()
		if jaegerCtx, ok := opentracingCtx.(jaeger.SpanContext); ok {
			return jaegerCtx.TraceID().String()
		}
	}
	return uuid.New().String()
}

// GetMaUUID retrieve ma-uuid from context metadata.
// Strategy to get Ma UUID:
// 1. find MaUUID in Context metadata
// 2. If 1 fail, use trace ID as MA UUID
// 3. If there is no trace ID in context, use an empty ID ToDO: in-situ generate a uuid for this case.
func GetMaUUID(ctx context.Context) string {

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if maUUID, ok := md[MaUUIDKey]; ok {
			return strings.Join(maUUID, " ")
		}
	}

	return GetTraceID(ctx)
}

// GetActor retrieve actor from context metadata
func GetActor(ctx context.Context) string {

	actor := "default"
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if userVal, ok := md[GRPCUserKey]; ok {
			actor = userVal[0]
		}
	}
	return actor
}

// GetCaller retrieve actor from context metadata
func GetCaller(ctx context.Context) string {

	caller := "default"
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if callerVal, ok := md[GRPCCallerKey]; ok {
			caller = callerVal[0]
		}
	}
	return caller
}

// GetSource retrieve source from context metadata
func GetSource(ctx context.Context) string {

	source := "default"
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if sourceVal, ok := md[GRPCSourceKey]; ok {
			source = sourceVal[0]
		}
	}
	return source
}
