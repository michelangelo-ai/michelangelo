// Package v2_ext provides extension validation for v2 API types.
// This file manually registers ext validators that map base v2 types
// to their stricter ext validation types.
package v2_ext

import (
	"github.com/michelangelo-ai/michelangelo/go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func init() {
	registerModelExtValidator()
	registerProjectExtValidator()
}

// registerModelExtValidator registers ext validation for v2.Model
func registerModelExtValidator() {
	api.RegisterExtValidator("*v2.Model", func(obj interface{}) error {
		model, ok := obj.(*v2.Model)
		if !ok || model == nil {
			return nil
		}

		// Get spec using the getter method (handles nil safely)
		spec := model.GetSpec()

		// Validate ModelSpec fields with stricter rules
		ext := &ModelSpecExt{
			Description:       spec.GetDescription(),
			Kind:              int32(spec.GetKind()),
			Algorithm:         spec.GetAlgorithm(),
			TrainingFramework: spec.GetTrainingFramework(),
		}
		if err := ext.Validate("spec."); err != nil {
			return err
		}

		// Validate LLMSpec if present
		llmSpec := spec.GetLlmSpec()
		if llmSpec != nil {
			ext := &LLMSpecExt{
				Vendor:           llmSpec.GetVendor(),
				ModelName:        llmSpec.GetModelName(),
				FineTunedModelId: llmSpec.GetFineTunedModelId(),
			}
			if err := ext.Validate("spec.llm_spec."); err != nil {
				return err
			}
		}

		return nil
	})
}

// registerProjectExtValidator registers ext validation for v2.Project
func registerProjectExtValidator() {
	api.RegisterExtValidator("*v2.Project", func(obj interface{}) error {
		project, ok := obj.(*v2.Project)
		if !ok || project == nil {
			return nil
		}

		// Get spec using the getter method
		spec := project.GetSpec()

		// Get owner info
		owner := spec.GetOwner()

		// Validate ProjectSpec fields with stricter rules
		ext := &ProjectSpecExt{
			Description: spec.GetDescription(),
			OwningTeam:  owner.GetOwningTeam(),
			Owners:      owner.GetOwners(),
			Tier:        spec.GetTier(),
			GitRepo:     spec.GetGitRepo(),
		}
		if err := ext.Validate("spec."); err != nil {
			return err
		}

		return nil
	})
}
