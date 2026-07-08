package client

import (
	"testing"

	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	"github.com/stretchr/testify/assert"
)

func TestSparkApplicationType(t *testing.T) {
	cases := []struct {
		name                string
		mainApplicationFile string
		expected            sparkv1beta2.SparkApplicationType
	}{
		{
			name:                "python script",
			mainApplicationFile: "s3://bucket/custom_job.py",
			expected:            sparkv1beta2.PythonApplicationType,
		},
		{
			name:                "R script uppercase extension",
			mainApplicationFile: "s3://bucket/analysis.R",
			expected:            sparkv1beta2.RApplicationType,
		},
		{
			name:                "R script lowercase extension",
			mainApplicationFile: "s3://bucket/analysis.r",
			expected:            sparkv1beta2.RApplicationType,
		},
		{
			name:                "jar with main class defaults to Java",
			mainApplicationFile: "local:///opt/spark/examples/jars/spark-examples_2.12-3.5.5.jar",
			expected:            sparkv1beta2.JavaApplicationType,
		},
		{
			name:                "empty file defaults to Java",
			mainApplicationFile: "",
			expected:            sparkv1beta2.JavaApplicationType,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, sparkApplicationType(tc.mainApplicationFile))
		})
	}
}
