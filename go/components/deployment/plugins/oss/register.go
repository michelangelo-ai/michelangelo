package oss

import (
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Module for fx dependency injection
var Module = fx.Options(
	fx.Invoke(Register),
)

// Params holds the dependencies for plugin registration
type Params struct {
	fx.In
	Registrar pluginmanager.Registrar[plugins.Plugin]
	Client    client.Client
	Gateway   gateways.Gateway
	BlobStore *blobstore.BlobStore
}

// Register registers the OSS plugin for all target types and subtypes
func Register(p Params) error {
	return registerPlugins(p.Registrar, p.Client, p.Gateway, p.BlobStore)
}

// registerPlugins is the implementation for plugin registration
func registerPlugins(registrar pluginmanager.Registrar[plugins.Plugin], client client.Client, gateway gateways.Gateway, blobstore *blobstore.BlobStore) error {
	ossPlugin := &Plugin{
		client:    client,
		gateway:   gateway,
		blobstore: blobstore,
	}

	// Register for inference server with realtime-serving subtype
	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "realtime-serving", ossPlugin); err != nil {
		return err
	}

	// Register for inference server with batch-serving subtype
	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "batch-serving", ossPlugin); err != nil {
		return err
	}

	// Register for inference server with empty subtype (default)
	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "", ossPlugin); err != nil {
		return err
	}

	return nil
}
