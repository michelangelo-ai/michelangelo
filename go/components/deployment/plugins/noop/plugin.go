package noop

import (
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/utils/pluginmanager"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// RegisterNoOpPlugins registers the no-op plugin for all target types
func RegisterNoOpPlugins(registrar pluginmanager.Registrar[plugins.Plugin]) error {
	noOpPlugin := plugins.NewNoOpPlugin()

	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "", noOpPlugin); err != nil {
		return err
	}

	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_OFFLINE.String(), "", noOpPlugin); err != nil {
		return err
	}

	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_MOBILE.String(), "", noOpPlugin); err != nil {
		return err
	}

	return nil
}
