package oss

import (
	"go.uber.org/fx"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Module for fx dependency injection
var Module = fx.Options(
	fx.Invoke(Register),
)

// Register registers the OSS plugin for all target types and subtypes
func Register(p Params) error {
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
