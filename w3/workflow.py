load('@plugin', __concurrent__='concurrent')
load('/notebook_task.star', __notebook_task__='task')

def pipeline_1775804768096():
    """Testing concurrent callable fix in cauldron-admin"""
    # Parallel execution of tasks
    extract_alpha_data_source = "alpha_db"
    extract_alpha_batch_size = "100"
    extract_beta_data_source = "beta_db"
    extract_beta_batch_size = "200"
    batch = __concurrent__.batch_run([__concurrent__.new_callable(__notebook_task__("/notebooks/extract_data.ipynb", alias="extract_alpha", git_ref="HEAD"), batch_size=extract_alpha_batch_size, data_source=extract_alpha_data_source), __concurrent__.new_callable(__notebook_task__("/notebooks/extract_data.ipynb", alias="extract_beta", git_ref="HEAD"), data_source=extract_beta_data_source, batch_size=extract_beta_batch_size)])
    results = batch.get()
    extract_alpha_result = results[0]
    extract_beta_result = results[1]
    return {"extract_alpha": extract_alpha_result, "extract_beta": extract_beta_result}
