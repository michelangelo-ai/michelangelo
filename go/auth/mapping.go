// Copyright (c) 2023 Uber Technologies, Inc.
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

package auth

import (
	"fmt"

	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

// APIGroup is the Kubernetes API group of the generated Michelangelo
// resources. It must match v2pb.GroupVersion.Group; that package imports
// this one, so the constant is restated here and cross-checked by
// TestAPIGroupMatchesGeneratedGroup.
const APIGroup = "michelangelo.api"

// verbForAction maps Action values onto Kubernetes RBAC verbs. Unknown
// actions fail closed in Attributes.
var verbForAction = map[Action]string{
	Create:           "create",
	Get:              "get",
	Update:           "update",
	Delete:           "delete",
	DeleteCollection: "deletecollection",
	List:             "list",
}

// resourceForKind maps the generated API kinds onto their CRD plural
// resource names, exactly as declared in the CRD manifests
// (spec.names.plural). Note "modelfamilys": the CRD plural is derived by
// lowercasing the kind and appending "s", so the grammatically tempting
// "modelfamilies" would never match an RBAC rule.
// TestResourceForKindMatchesCRDs keeps this table in lockstep with the
// generated CRD manifests.
var resourceForKind = map[string]string{
	"CachedOutput":     "cachedoutputs",
	"Cluster":          "clusters",
	"Deployment":       "deployments",
	"EvaluationReport": "evaluationreports",
	"InferenceServer":  "inferenceservers",
	"Model":            "models",
	"ModelFamily":      "modelfamilys",
	"Pipeline":         "pipelines",
	"PipelineRun":      "pipelineruns",
	"Project":          "projects",
	"RayCluster":       "rayclusters",
	"RayJob":           "rayjobs",
	"Revision":         "revisions",
	"SparkJob":         "sparkjobs",
	"TriggerRun":       "triggerruns",
}

// Attributes builds the authorizer attributes for a generated handler's
// (namespace, action, kind) triple. An empty namespace expresses a
// cluster-wide request (for example List across all namespaces) and
// therefore requires a cluster-scoped RBAC grant. Unknown actions and
// kinds fail closed with an error.
func Attributes(userInfo user.Info, namespace string, action Action, kind string) (authorizer.Attributes, error) {
	verb, ok := verbForAction[action]
	if !ok {
		return nil, fmt.Errorf("auth: unknown action %q", string(action))
	}
	resource, ok := resourceForKind[kind]
	if !ok {
		return nil, fmt.Errorf("auth: unknown resource kind %q", kind)
	}
	return authorizer.AttributesRecord{
		User:            userInfo,
		Verb:            verb,
		APIGroup:        APIGroup,
		Resource:        resource,
		Namespace:       namespace,
		ResourceRequest: true,
	}, nil
}
