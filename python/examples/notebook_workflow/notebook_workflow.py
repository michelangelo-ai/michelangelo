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

    # STEP 1: Data Validation
    # - task_values: Shares actual data statistics
    # - exit_value: Returns validation status for workflow decision
    validation_status, validation_shared_data = notebook_executor(
        notebook_path="examples/notebook_workflow/data_validation.ipynb",
        parameters={
            "data_size": str(data_size),
            "seed": str(seed)
        }
    )

    if validation_status.get("status") == "PASSED":
        analysis_notebook = "examples/notebook_workflow/advanced_analysis.ipynb"
    else:
        analysis_notebook = "examples/notebook_workflow/basic_analysis.ipynb"

    # Execute the chosen analysis (pass task_values from validation as input)
    analysis_recommendation, analysis_shared_data = notebook_executor(
        notebook_path=analysis_notebook,
        parameters=validation_shared_data  # Pass task_values as input to next notebook
    )

    return analysis_recommendation


if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.environ["UF_PLUGIN_RAY_USE_FSSPEC"] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"

    # Test with good data (should pass validation)
    print("=" * 80)
    print("🧪 TEST 1: Good data (should trigger advanced analysis)")
    print("=" * 80)
    result1 = ctx.run(conditional_notebook_workflow, data_size=100, seed=42)
