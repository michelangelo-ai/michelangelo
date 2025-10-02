from uber.ai.uniflow.core import workflow
from uber.ai.michelangelo.sdk.workflow.tasks.document_loader import task_function as document_loader
from uber.ai.michelangelo.sdk.workflow.tasks.llm_spark_scorer import task_function as scorer
from uber.ai.michelangelo.sdk.workflow.tasks.llm_multistage_scorer import task_function as multistage_scorer
from uber.ai.michelangelo.sdk.workflow.tasks.llm_inference import task_function as inference
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher_offline
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher_online
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig


@workflow()
def vector_gen(workflow_config: WorkflowConfig, task_configs: dict[str, TaskConfig]):  # noqa: ARG001
    loaded_documents, raw_vector_dataset = document_loader(task_configs["document_loader"])

    # Support both traditional scorer and new multistage_scorer
    embeddings = None
    if "scorer" in task_configs:
        embeddings = scorer.with_overrides(alias="scorer")(task_configs["scorer"], loaded_documents)
    elif "multistage_scorer" in task_configs:
        embeddings = multistage_scorer.with_overrides(alias="multistage_scorer")(task_configs["multistage_scorer"], loaded_documents)

    if "inference" in task_configs:
        inference_results = inference.with_overrides(alias="inference")(task_configs["inference"], {"documents": loaded_documents})
        embeddings = inference_results["documents"]

    if embeddings != None:  # noqa: E711
        raw_vector_dataset.output_column_metadata = embeddings.metadata.output_column_metadata

    embedding_artifacts = {"default": embeddings}

    pusher_plugin_result_list = pusher_offline.with_overrides(alias="pusher_offline")(task_configs["pusher_offline"], embedding_artifacts)
    offline_pusher_results = pusher_plugin_result_list[0]
    offline_dataset_spec, offline_dataset_schema = offline_pusher_results.value

    raw_vector_dataset.offline_dataset = offline_dataset_spec
    raw_vector_dataset.offline_dataset_schema = offline_dataset_schema

    raw_vector_dataset_artifact = {"default": raw_vector_dataset}

    if "pusher_online" in task_configs:
        online_pusher_result_list = pusher_online.with_overrides(alias="pusher_online")(task_configs["pusher_online"], raw_vector_dataset_artifact)
        online_pusher_results = online_pusher_result_list[0]
        raw_vector_dataset = online_pusher_results.value

        raw_vector_dataset_artifact = {"default": raw_vector_dataset}

        pusher(task_configs["pusher"], raw_vector_dataset_artifact)
    else:
        return raw_vector_dataset_artifact
