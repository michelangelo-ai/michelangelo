// Package notification module provides FX dependency injection for PipelineRun notifications.
package notification

import (
	"go.uber.org/fx"
)

var (
	// Module is the Uber FX module for PipelineRun notification functionality.
	//
	// This module provides the PipelineRunNotifier instance with proper dependency
	// injection of the workflow client and logger. It should be included
	// in applications that need pipeline run notification capabilities.
	//
	// To use this module, include it in your FX application:
	//   fx.New(
	//       notification.Module,
	//       // other modules...
	//   )
	Module = fx.Options(
		fx.Provide(NewPipelineRunNotifier),
	)
)