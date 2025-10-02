"""
API based inference pipeline def
"""

from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig
from uber.ai.michelangelo.sdk.workflow.tasks.llm_create_batch.task import llm_create_batch
from uber.ai.michelangelo.sdk.workflow.tasks.llm_retrieve_batch.task import llm_retrieve_batch
from uber.ai.michelangelo.sdk.workflow.tasks.llm_feature_prep import (
    task_function as llm_feature_prep,
)
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher
from uber.ai.uniflow.core import workflow


@workflow()
def llm_api_inference(task_configs: dict[str, TaskConfig]):
    """
    LLM Batch API based inference workflow
    """
    (
        dataset,
        _,
        _,
    ) = llm_feature_prep(config=task_configs["llm_feature_prep"])

    datasets = {"dataset": dataset}

    batch_result, batch_data = llm_create_batch(
        config=task_configs["create_batches"],
        datasets=datasets,
    )

    dataset_artifact = llm_retrieve_batch(
        config=task_configs["retrieve_batches"],
        datasets=batch_data,
        batches=batch_result,
    )

    pusher(config=task_configs["pusher"], items=dataset_artifact)
