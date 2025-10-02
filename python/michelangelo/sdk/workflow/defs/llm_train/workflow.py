from uber.ai.michelangelo.sdk.workflow.tasks.llm_feature_prep.task import llm_feature_prep
from uber.ai.michelangelo.sdk.workflow.tasks.llm_trainer import task_function as llm_trainer
from uber.ai.michelangelo.sdk.workflow.tasks.llm_assembler import task_function as llm_assembler
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher

from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig
from uber.ai.uniflow.core import workflow


@workflow()
def llm_train(workflow_config: WorkflowConfig, task_configs: dict[str, TaskConfig]):  # noqa: ARG001
    train_data, validation_data, feature_quality_report = llm_feature_prep(task_configs["llm_feature_prep"])
    raw_model = llm_trainer(task_configs["llm_trainer"], train_data, validation_data)
    assembled_model = llm_assembler(task_configs["llm_assembler"], raw_model)

    assembled_model.feature_quality_report = feature_quality_report

    artifacts = {"model": assembled_model}

    pusher(task_configs["pusher"], artifacts)
