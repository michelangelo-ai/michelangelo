"""Conditional workflow example showing exit_value vs task_values usage."""

import michelangelo.uniflow.core as uniflow
from examples.notebook_workflow.executor import notebook_executor


@uniflow.workflow()
def conditional_notebook_workflow(data_size: int = 100, seed: int = 42):
    """
    Conditional workflow demonstrating:
    - exit_value: Used for workflow decisions (what's next?)
    - task_values: Used for data sharing between notebooks
    """

    print(f"🚀 Starting conditional workflow with data_size={data_size}, seed={seed}")

    # STEP 1: Data Validation
    # - task_values: Shares actual data statistics
    # - exit_value: Returns validation status for workflow decision
    print("\n📋 STEP 1: Data Validation")
    validation_status, validation_shared_data = notebook_executor(
        notebook_path="examples/notebook_workflow/data_validation.ipynb",
        parameters={
            "data_size": str(data_size),
            "seed": str(seed)
        }
    )

    print(f"  📤 Exit Value (for workflow logic): {validation_status.get('status', 'UNKNOWN')}")
    print(f"  📊 Task Values (shared data): {len(validation_shared_data)} metrics shared")

    # STEP 2: Conditional Analysis (based on exit_value)
    print(f"\n🔀 STEP 2: Conditional Analysis")

    if validation_status.get("status") == "PASSED":
        print("  ✅ Data quality PASSED → Running ADVANCED analysis")
        analysis_notebook = "examples/notebook_workflow/advanced_analysis.ipynb"
        analysis_type = "advanced"
    else:
        print("  ⚠️  Data quality FAILED → Running BASIC analysis")
        analysis_notebook = "examples/notebook_workflow/basic_analysis.ipynb"
        analysis_type = "basic"

    # Execute the chosen analysis (pass task_values from validation as input)
    analysis_recommendation, analysis_shared_data = notebook_executor(
        notebook_path=analysis_notebook,
        parameters=validation_shared_data  # Pass task_values as input to next notebook
    )

    print(f"  📤 Exit Value (recommendation): {analysis_recommendation.get('recommendation', 'UNKNOWN')}")
    print(f"  📊 Task Values (analysis metrics): {len(analysis_shared_data)} metrics shared")

    # STEP 3: Final Decision (based on both exit_values)
    print(f"\n🎯 STEP 3: Final Workflow Decision")

    validation_score = validation_status.get("data_quality_score", 0)
    analysis_confidence = analysis_recommendation.get("confidence", "NONE")
    analysis_rec = analysis_recommendation.get("recommendation", "UNKNOWN")

    # Workflow logic using exit_values
    if validation_score > 0.8 and analysis_confidence == "HIGH":
        final_decision = "DEPLOY_TO_PRODUCTION"
        confidence = "HIGH"
    elif validation_score > 0.6 and analysis_confidence in ["HIGH", "MEDIUM"]:
        final_decision = "DEPLOY_TO_STAGING"
        confidence = "MEDIUM"
    elif analysis_rec in ["BASIC_MODELING", "ADDITIONAL_PREPROCESSING"]:
        final_decision = "MANUAL_REVIEW_REQUIRED"
        confidence = "LOW"
    else:
        final_decision = "REJECT_DATA"
        confidence = "NONE"

    print(f"  🎯 Final Decision: {final_decision} (confidence: {confidence})")

    # Compile final results
    final_results = {
        "workflow_status": "completed",
        "workflow_decision": final_decision,
        "confidence": confidence,

        # Results from exit_values (workflow logic)
        "validation_step": {
            "status": validation_status.get("status"),
            "next_step": validation_status.get("next_step"),
            "quality_score": validation_score
        },
        "analysis_step": {
            "type": analysis_type,
            "recommendation": analysis_rec,
            "confidence": analysis_confidence
        },

        # Summary of shared data (from task_values)
        "shared_data_summary": {
            "validation_metrics": len(validation_shared_data),
            "analysis_metrics": len(analysis_shared_data),
            "total_shared_metrics": len(validation_shared_data) + len(analysis_shared_data)
        },

        # All shared data combined
        "all_shared_data": {**validation_shared_data, **analysis_shared_data}
    }

    print(f"\n📊 Workflow Summary:")
    print(f"  • Validation: {validation_status.get('status')} → {analysis_type} analysis")
    print(f"  • Analysis: {analysis_rec} ({analysis_confidence} confidence)")
    print(f"  • Final: {final_decision}")
    print(f"  • Shared Data: {final_results['shared_data_summary']['total_shared_metrics']} metrics")

    return final_results


if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.environ["UF_PLUGIN_RAY_USE_FSSPEC"] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"

    # Test with good data (should pass validation)
    print("=" * 80)
    print("🧪 TEST 1: Good data (should trigger advanced analysis)")
    print("=" * 80)
    result1 = ctx.run(conditional_notebook_workflow, data_size=100, seed=42)

    # Test with bad data (should fail validation)
    print("\n" + "=" * 80)
    print("🧪 TEST 2: Questionable data (might trigger basic analysis)")
    print("=" * 80)
    result2 = ctx.run(conditional_notebook_workflow, data_size=50, seed=999)