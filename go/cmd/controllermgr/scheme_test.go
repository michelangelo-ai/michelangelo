package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TestScheme_RegistersCRDsInGlobalScheme: api.Handler resolves list kinds through
// client-go's global scheme, so controllers listing CRDs (e.g. TriggerRun listing its
// child PipelineRuns) fail unless scheme() also registers the CRD types there.
func TestScheme_RegistersCRDsInGlobalScheme(t *testing.T) {
	s, err := scheme()
	require.NoError(t, err)

	for _, list := range []runtime.Object{&v2pb.PipelineRunList{}, &v2pb.TriggerRunList{}} {
		_, err := apiutil.GVKForObject(list, s)
		require.NoError(t, err, "manager scheme")
		_, err = apiutils.GetObjectTypeMetaFromList(list, kubescheme.Scheme)
		require.NoError(t, err, "global scheme used by api.Handler")
	}
}
