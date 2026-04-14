# YAML DAG vs Python Workflow Comparison

This document shows how our existing Python notebook workflow translates to YAML DAG format.

## Python Conditional Workflow (conditional_workflow.py)

```python
@uniflow.workflow()
def conditional_notebook_workflow(data_size: int = 100, seed: int = 42):
    # STEP 1: Data Validation
    validation_status, validation_shared_data = notebook_executor(
        notebook_path="examples/notebook_workflow/data_validation.ipynb",
        parameters={"data_size": str(data_size), "seed": str(seed)},
    )

    # STEP 2: Conditional Analysis (based on exit_value)
    if validation_status.get("status") == "PASSED":
        analysis_notebook = "examples/notebook_workflow/advanced_analysis.ipynb"
        analysis_type = "advanced"
    else:
        analysis_notebook = "examples/notebook_workflow/basic_analysis.ipynb"
        analysis_type = "basic"

    # Execute chosen analysis (pass task_values as input)
    analysis_recommendation, analysis_shared_data = notebook_executor(
        notebook_path=analysis_notebook,
        parameters=validation_shared_data,  # task_values → parameters
    )

    # STEP 3: Final Decision Logic
    validation_score = validation_status.get("data_quality_score", 0)
    analysis_confidence = analysis_recommendation.get("confidence", "NONE")
    # ... workflow logic ...

    return final_results
```

## Equivalent YAML DAG (conditional_notebook_dag.yaml)

```yaml
spec_version: 1.0
pipeline:
    name: "conditional_notebook_workflow_dag"
    tasks:
        # STEP 1: Data Validation
        - task_id: data_validation
          notebook_task:
              notebook_path: examples/notebook_workflow/data_validation.ipynb
              user_parameters:
                  data_size: "{{pipeline.parameters.data_size}}"
                  seed: "{{pipeline.parameters.seed}}"

        # STEP 2: Conditional Logic
        - task_id: validation_check
          if_else_task:
              condition:
                  op: EQUAL_TO
                  left: "{{tasks.data_validation.exit_value.status}}"
                  right: "PASSED"

        # STEP 3A: Advanced Analysis (if PASSED)
        - task_id: advanced_analysis
          depends_on:
              - task_id: validation_check
                outcome: "true"
          notebook_task:
              notebook_path: examples/notebook_workflow/advanced_analysis.ipynb
              user_parameters:
                  # task_values → user_parameters
                  raw_data_mean_x: "{{tasks.data_validation.task_values.raw_data_mean_x}}"
                  raw_data_mean_y: "{{tasks.data_validation.task_values.raw_data_mean_y}}"
                  # ... more task_values mappings

        # STEP 3B: Basic Analysis (if FAILED)
        - task_id: basic_analysis
          depends_on:
              - task_id: validation_check
                outcome: "false"
          notebook_task:
              notebook_path: examples/notebook_workflow/basic_analysis.ipynb
              user_parameters:
                  # Same task_values → user_parameters mapping
```

## Key Mapping Concepts

### 1. **Data Flow Translation**

| **Python Concept** | **YAML DAG Equivalent** |
|---------------------|-------------------------|
| `return exit_value, task_values` | `{{tasks.task_id.exit_value}}` & `{{tasks.task_id.task_values}}` |
| `parameters=validation_shared_data` | `user_parameters: {{tasks.validation.task_values.*}}` |
| `if validation_status.get("status") == "PASSED"` | `if_else_task: condition: op: EQUAL_TO` |
| Python function parameters | `user_parameters` in YAML |

### 2. **Conditional Logic Translation**

| **Python** | **YAML DAG** |
|-------------|--------------|
| ```python<br/>if validation_status.get("status") == "PASSED":<br/>    notebook = "advanced.ipynb"<br/>else:<br/>    notebook = "basic.ipynb"``` | ```yaml<br/>- task_id: validation_check<br/>  if_else_task:<br/>    condition:<br/>      left: "{{tasks.validation.exit_value.status}}"<br/>      right: "PASSED"<br/><br/>- task_id: advanced_analysis<br/>  depends_on:<br/>    - task_id: validation_check<br/>      outcome: "true"<br/><br/>- task_id: basic_analysis<br/>  depends_on:<br/>    - task_id: validation_check<br/>      outcome: "false"``` |

### 3. **Parameter Passing Translation**

| **Python** | **YAML DAG** |
|-------------|--------------|
| ```python<br/>notebook_executor(<br/>    notebook_path="validation.ipynb",<br/>    parameters={<br/>        "data_size": str(data_size),<br/>        "seed": str(seed)<br/>    }<br/>)``` | ```yaml<br/>notebook_task:<br/>  notebook_path: examples/notebook_workflow/validation.ipynb<br/>  user_parameters:<br/>    data_size: "{{pipeline.parameters.data_size}}"<br/>    seed: "{{pipeline.parameters.seed}}"``` |

### 4. **Task Value Propagation**

| **Python** | **YAML DAG** |
|-------------|--------------|
| ```python<br/># Return from upstream task<br/>exit_value, task_values = notebook_executor(...)<br/><br/># Pass to downstream task<br/>analysis_result = notebook_executor(<br/>    notebook_path="analysis.ipynb",<br/>    parameters=task_values  # Direct passing<br/>)``` | ```yaml<br/># Upstream task automatically exposes:<br/># {{tasks.data_validation.exit_value.*}}<br/># {{tasks.data_validation.task_values.*}}<br/><br/># Downstream task consumes:<br/>user_parameters:<br/>  raw_data_mean_x: "{{tasks.data_validation.task_values.raw_data_mean_x}}"<br/>  data_size_actual: "{{tasks.data_validation.task_values.data_size_actual}}"``` |

## Benefits of YAML DAG Format

### ✅ **Advantages over Python Workflows:**
1. **Declarative**: Clear task dependencies and data flow
2. **Visual**: Easy to understand the DAG structure
3. **Parallelizable**: Explicit task parallelism and dependencies
4. **Platform Agnostic**: Can be executed by different workflow engines
5. **Version Controllable**: YAML is easily diffable and trackable
6. **UI Renderable**: Can generate workflow visualizations

### ⚠️ **Considerations:**
1. **Template Complexity**: YAML templating can become complex for advanced logic
2. **Expression Limitations**: Complex conditional logic may require custom operators
3. **Type Safety**: Less compile-time checking compared to Python
4. **Debugging**: Harder to debug template expressions vs Python code

## Migration Strategy

To migrate from Python workflows to YAML DAGs:

1. **Identify Task Boundaries**: Each `notebook_executor()` call becomes a YAML task
2. **Map Conditionals**: `if/else` logic becomes `if_else_task` + `depends_on`
3. **Extract Parameters**: Function parameters become pipeline parameters
4. **Map Data Flow**: `task_values` become template expressions `{{tasks.*.task_values.*}}`
5. **Define Dependencies**: Sequential execution becomes `depends_on` relationships

Both approaches achieve the same result - the YAML format provides better visualization and platform portability, while Python offers more programming flexibility.