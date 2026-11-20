package client

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	k8sptr "k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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

func TestToSparkPodSpec_EnvFrom(t *testing.T) {
	const credentialsSecret = "minio-credentials"
	credentials := corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
		Optional:             k8sptr.To(true),
	}}
	configMap := &v2pb.EnvFromSource{ConfigMapRef: &v2pb.ConfigMapEnvSource{
		LocalObjectReference: &v2pb.LocalObjectReference{Name: "michelangelo-config"},
	}}
	coreConfigMap := corev1.EnvFromSource{ConfigMapRef: &corev1.ConfigMapEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: "michelangelo-config"},
	}}

	cases := []struct {
		name              string
		credentialsSecret string
		envFrom           []*v2pb.EnvFromSource
		expected          []corev1.EnvFromSource
	}{
		{
			name:     "no credentials secret configured -> envFrom converted as is",
			envFrom:  []*v2pb.EnvFromSource{configMap},
			expected: []corev1.EnvFromSource{coreConfigMap},
		},
		{
			name:              "credentials secret -> ahead of the config map",
			credentialsSecret: credentialsSecret,
			envFrom:           []*v2pb.EnvFromSource{configMap},
			expected:          []corev1.EnvFromSource{credentials, coreConfigMap},
		},
		{
			name:              "no envFrom -> credentials secret only",
			credentialsSecret: credentialsSecret,
			expected:          []corev1.EnvFromSource{credentials},
		},
		{
			name: "prefix and optional carried over",
			envFrom: []*v2pb.EnvFromSource{
				{Prefix: "APP_", SecretRef: &v2pb.SecretEnvSource{
					LocalObjectReference: &v2pb.LocalObjectReference{Name: "app-secret"},
					Optional:             true,
				}},
				{Prefix: "CFG_", ConfigMapRef: &v2pb.ConfigMapEnvSource{
					LocalObjectReference: &v2pb.LocalObjectReference{Name: "app-config"},
					Optional:             true,
				}},
				{SecretRef: &v2pb.SecretEnvSource{
					LocalObjectReference: &v2pb.LocalObjectReference{Name: "required-secret"},
				}},
			},
			expected: []corev1.EnvFromSource{
				{Prefix: "APP_", SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "app-secret"},
					Optional:             k8sptr.To(true),
				}},
				{Prefix: "CFG_", ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
					Optional:             k8sptr.To(true),
				}},
				{SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "required-secret"},
				}},
			},
		},
		{
			name: "nil local object references -> empty names, no panic",
			envFrom: []*v2pb.EnvFromSource{
				{SecretRef: &v2pb.SecretEnvSource{}},
				{ConfigMapRef: &v2pb.ConfigMapEnvSource{}},
			},
			expected: []corev1.EnvFromSource{
				{SecretRef: &corev1.SecretEnvSource{}},
				{ConfigMapRef: &corev1.ConfigMapEnvSource{}},
			},
		},
		{
			name:              "credentials secret already listed unprefixed -> not duplicated",
			credentialsSecret: credentialsSecret,
			envFrom: []*v2pb.EnvFromSource{
				configMap,
				{SecretRef: &v2pb.SecretEnvSource{
					LocalObjectReference: &v2pb.LocalObjectReference{Name: credentialsSecret},
				}},
			},
			expected: []corev1.EnvFromSource{
				coreConfigMap,
				{SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
				}},
			},
		},
		{
			name:              "credentials secret listed only with a prefix -> unprefixed one prepended",
			credentialsSecret: credentialsSecret,
			envFrom: []*v2pb.EnvFromSource{
				{Prefix: "LOGS_", SecretRef: &v2pb.SecretEnvSource{
					LocalObjectReference: &v2pb.LocalObjectReference{Name: credentialsSecret},
				}},
			},
			expected: []corev1.EnvFromSource{
				credentials,
				{Prefix: "LOGS_", SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
				}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := SparkClient{CredentialsSecret: tc.credentialsSecret}
			pod := &v2pb.PodSpec{Resource: &v2pb.ResourceSpec{Cpu: 1, Memory: "1g"}, EnvFrom: tc.envFrom}

			// CreateJob converts the driver, with its service account, and the
			// executor, without one, through the same function.
			serviceAccount := "spark-operator-spark"
			assert.Equal(t, tc.expected, client.toSparkPodSpec(pod, &serviceAccount).EnvFrom, "driver")
			assert.Equal(t, tc.expected, client.toSparkPodSpec(pod, nil).EnvFrom, "executor")
		})
	}
}

// configOnlyManager stubs the one manager.Manager method register calls.
type configOnlyManager struct {
	manager.Manager
}

func (configOnlyManager) GetConfig() *rest.Config {
	return &rest.Config{Host: "https://localhost"}
}

func TestModule_CredentialsSecret(t *testing.T) {
	cases := []struct {
		name     string
		provide  fx.Option
		expected string
	}{
		{
			name:     "log persistence config absent -> no credentials secret",
			provide:  fx.Options(),
			expected: "",
		},
		{
			name:     "log persistence config provided -> its credentials secret",
			provide:  fx.Supply(k8sengine.LogPersistenceConfig{CredentialsSecret: "minio-credentials"}),
			expected: "minio-credentials",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var client job.Client
			fxtest.New(t,
				fx.Supply(env.Context{}),
				fx.Provide(func() manager.Manager { return configOnlyManager{} }),
				tc.provide,
				Module,
				fx.Populate(&client),
			).RequireStart().RequireStop()

			require.IsType(t, &SparkClient{}, client)
			assert.Equal(t, tc.expected, client.(*SparkClient).CredentialsSecret)
		})
	}
}
