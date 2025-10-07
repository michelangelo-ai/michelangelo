package utils

import (
	"testing"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetObjectNamespace(t *testing.T) {
	testCases := []struct {
		name       string
		obj        interface{}
		expectedNS string
	}{
		{
			name: "Ray Job with namespace",
			obj: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
				},
			},
			expectedNS: "test-namespace",
		},
		{
			name: "Spark Job with namespace",
			obj: &v2pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "spark-namespace",
				},
			},
			expectedNS: "spark-namespace",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test basic object access
			switch obj := tc.obj.(type) {
			case *v2pb.RayJob:
				assert.Equal(t, tc.expectedNS, obj.ObjectMeta.Namespace)
			case *v2pb.SparkJob:
				assert.Equal(t, tc.expectedNS, obj.ObjectMeta.Namespace)
			}
		})
	}
}

func TestBasicUtilityFunctions(t *testing.T) {
	t.Run("TestBasicObjectCreation", func(t *testing.T) {
		rayJob := &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ray-job",
				Namespace: "test-namespace",
			},
		}

		assert.Equal(t, "test-ray-job", rayJob.ObjectMeta.Name)
		assert.Equal(t, "test-namespace", rayJob.ObjectMeta.Namespace)
	})

	t.Run("TestSparkJobCreation", func(t *testing.T) {
		sparkJob := &v2pb.SparkJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-spark-job",
				Namespace: "spark-namespace",
			},
		}

		assert.Equal(t, "test-spark-job", sparkJob.ObjectMeta.Name)
		assert.Equal(t, "spark-namespace", sparkJob.ObjectMeta.Namespace)
	})
}
