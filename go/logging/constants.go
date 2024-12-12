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

// UAPIYARPCMetricScopeKey is the key for metric subscope for all Unified API Yarpc calls
const UAPIYARPCMetricScopeKey = "uapi_yarpc"

// YAPRCTypeTag is tag for type of yarpc calls
const YAPRCTypeTag = "yarpc_type"

// ProjectTag is the tag for project name
const ProjectTag = "project"

// EntityTag is the tag for entity name
const EntityTag = "entity"

// YARPCActorTag is the tag for project name
const YARPCActorTag = "actor"

// YARPCSourceTag is the tag for project name
const YARPCSourceTag = "source"

// PipelineTypeTag is the tag for project name
const PipelineTypeTag = "pipeline_type"

// maxLogSize is the max size of payload that'll be output to logging Kafka topic
const maxLogSize = 500000
const auditLogTopic = "hp-michelangelo-apiserver-audit-logger"
const auditLogTopicVersion = 1

// GRPCCallerKey is the caller metadata key for GRPC requests.
const GRPCCallerKey = "rpc-caller"

// CrdMethodKey is the key for GRPC method. This field is added to context in YARPC call for CRD
const CrdMethodKey = "crd-method"

// GRPCUserKey is the caller metadata key for GRPC requests.
const GRPCUserKey = "x-auth-params-email"

// GRPCSourceKey is the source metadata key for GRPC requests
const GRPCSourceKey = "x-uber-source"

// NamespaceKey is the caller metadata key for GRPC requests.
const NamespaceKey = "namespace"

// CrdNameKey is the caller metadata key for GRPC requests.
const CrdNameKey = "name"

// MlCodeKafkaTopic is the kafka topic for ML_CODE debug log (deprecated)
const MlCodeKafkaTopic = "ml-code"

// MaUUIDKey is the key for canvas-uuid in for Unified Log
const MaUUIDKey = "ma-uuid"

// UnifiedLogKey is the key to label logs as unified log
const UnifiedLogKey = "isUnifiedLog"
