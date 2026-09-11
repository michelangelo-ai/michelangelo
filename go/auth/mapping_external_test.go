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

package auth_test

// An external test package: proto-go imports go/auth, so these
// cross-checks cannot live inside package auth without an import cycle.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/michelangelo-ai/michelangelo/go/auth"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestAPIGroupMatchesGeneratedGroup(t *testing.T) {
	assert.Equal(t, v2pb.GroupVersion.Group, auth.APIGroup)
}

// TestResourceForKindMatchesCRDs keeps the kind-to-plural table in lockstep
// with the generated CRD manifests: every generated kind must be present,
// with exactly the plural the CRD declares, and nothing extra.
func TestResourceForKindMatchesCRDs(t *testing.T) {
	require.NotEmpty(t, v2pb.YamlSchemas)
	assert.Len(t, auth.ResourceForKindForTest, len(v2pb.YamlSchemas))

	for kind, yamlSchema := range v2pb.YamlSchemas {
		crd := apiextv1.CustomResourceDefinition{}
		require.NoError(t, yaml.NewYAMLToJSONDecoder(strings.NewReader(yamlSchema)).Decode(&crd), kind)
		assert.Equal(t, crd.Spec.Names.Plural, auth.ResourceForKindForTest[kind],
			"resourceForKind[%q] must match the CRD's spec.names.plural", kind)
	}
}

// TestVerbForActionCoversAllActions pins the verb table to the six Action
// values the generated handlers can pass in.
func TestVerbForActionCoversAllActions(t *testing.T) {
	assert.Equal(t, map[auth.Action]string{
		auth.Create:           "create",
		auth.Get:              "get",
		auth.Update:           "update",
		auth.Delete:           "delete",
		auth.DeleteCollection: "deletecollection",
		auth.List:             "list",
	}, auth.VerbForActionForTest)
}
