// Package v2_ext provides extension validation for v2 proto types.
//
// When this package is imported, it automatically registers extension validators
// for v2 types (Model, Project, etc.). These validators apply stricter validation
// rules defined in the _ext.proto files.
//
// Usage:
//
//	import (
//	    _ "github.com/michelangelo-ai/michelangelo/proto/api/v2_ext"
//	)
//
// After importing, ext validation runs automatically for all v2 types when
// api.ValidateExt() is called (which happens in the validation handler).
package v2_ext

import (
	"github.com/michelangelo-ai/michelangelo/go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func init() {
	// Register ext validators for v2 types
	// These are called automatically by the validation handler
	registerModelValidator()
	registerProjectValidator()
}

// registerModelValidator registers the ext validator for v2.Model
func registerModelValidator() {
	api.RegisterExtValidator("*v2.Model", func(obj interface{}) error {
		model, ok := obj.(*v2.Model)
		if !ok || model == nil {
			return nil
		}

		// Validate ModelSpec if present
		if model.Spec != nil {
			ext := &ModelSpecExt{
				OwnerEmail:       getOwnerEmail(model.Spec.Owner),
				Description:      model.Spec.Description,
				Kind:             int32(model.Spec.Kind),
				Algorithm:        model.Spec.Algorithm,
				TrainingFramework: model.Spec.TrainingFramework,
			}
			if err := ext.Validate("spec."); err != nil {
				return err
			}

			// Validate LLMSpec if present
			if model.Spec.LlmSpec != nil {
				llmExt := &LLMSpecExt{
					Vendor:           model.Spec.LlmSpec.Vendor,
					ModelName:        model.Spec.LlmSpec.ModelName,
					FineTunedModelId: model.Spec.LlmSpec.FineTunedModelId,
				}
				if err := llmExt.Validate("spec.llm_spec."); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// registerProjectValidator registers the ext validator for v2.Project
func registerProjectValidator() {
	api.RegisterExtValidator("*v2.Project", func(obj interface{}) error {
		project, ok := obj.(*v2.Project)
		if !ok || project == nil {
			return nil
		}

		// Validate ProjectSpec if present
		if project.Spec != nil {
			ext := &ProjectSpecExt{
				Description: project.Spec.Description,
				Tier:        project.Spec.Tier,
				GitRepo:     project.Spec.GitRepo,
			}

			// Copy owner info if present
			if project.Spec.Owner != nil {
				ext.OwningTeam = project.Spec.Owner.OwningTeam
				ext.Owners = project.Spec.Owner.Owners
			}

			if err := ext.Validate("spec."); err != nil {
				return err
			}

			// Validate OwnerInfo if present
			if project.Spec.Owner != nil {
				ownerExt := &OwnerInfoExt{
					OwningTeam:  project.Spec.Owner.OwningTeam,
					Owners:      project.Spec.Owner.Owners,
					OwnerGroups: project.Spec.Owner.OwnerGroups,
				}
				if err := ownerExt.Validate("spec.owner."); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// getOwnerEmail extracts the email from UserInfo
func getOwnerEmail(owner *v2.UserInfo) string {
	if owner == nil {
		return ""
	}
	return owner.Email
}

