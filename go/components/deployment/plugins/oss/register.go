package oss

import (
	"go.uber.org/config"
	"go.uber.org/fx"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Module for fx dependency injection
var Module = fx.Options(
	fx.Provide(newDeploymentConfig),
	fx.Invoke(register),
)

func newDeploymentConfig(provider config.Provider) (maconfig.DeploymentConfig, error) {
	return maconfig.GetDeploymentConfig(provider)
}

// Register registers the OSS plugin for all target types and subtypes
func register(p Params) error {
	return registerPlugins(p)
}

// registerPlugins is the implementation for plugin registration
func registerPlugins(p Params) error {
	ossPlugin := NewPlugin(p)

	// Register for inference server with realtime-serving subtype
	if err := p.Registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "realtime-serving", ossPlugin); err != nil {
		return err
	}

	// Register for inference server with batch-serving subtype
	if err := p.Registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "batch-serving", ossPlugin); err != nil {
		return err
	}

	// Register for inference server with empty subtype (default)
	if err := p.Registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "", ossPlugin); err != nil {
		return err
	}

	return nil
}
