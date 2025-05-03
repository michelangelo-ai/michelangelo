package client

import (
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
)

var (
	// Module FX
	Module = fx.Options(
		fx.Provide(register),
	)
)

func register(
	env env.Context,
	mgr manager.Manager,
) job.Client {
	scheme := runtime.NewScheme()
	if err := sparkv1beta2.AddToScheme(scheme); err != nil {
		panic("failed to add scheme: " + err.Error())
	}

	config := mgr.GetConfig()
	config.ContentConfig.GroupVersion = &sparkv1beta2.SchemeGroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme)
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		panic("failed to create REST client: " + err.Error())
	}
	return &SparkClient{
		K8sClient:      restClient,
		ParameterCodec: runtime.NewParameterCodec(scheme),
	}
}
