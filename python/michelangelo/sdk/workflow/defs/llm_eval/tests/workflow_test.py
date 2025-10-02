from unittest import TestCase
from unittest.mock import patch

from uber.ai.michelangelo.sdk.workflow.defs.llm_eval import workflow_function as llm_eval
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.inference import LLMInferenceConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.evaluator import EvaluatorConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.pusher import PusherConfig, PusherPluginConfig, DatasetPluginConfig

from uber.ai.michelangelo.sdk.workflow.variables import DatasetVariable


class WorkflowTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.workflow.defs.llm_eval.workflow.llm_feature_prep")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.llm_eval.workflow.llm_inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.llm_eval.workflow.evaluator")
    @patch("uber.ai.michelangelo.sdk.workflow.tasks.pusher.task.DataSetPusherPlugin.execute")
    def test_llm_eval(
        self,
        mock_dataset_pusher_plugin_execute,
        mock_evaluator,
        mock_llm_inference,
        mock_llm_feature_prep,
    ):
        train_dataset = DatasetVariable.create(None)

        mock_llm_feature_prep.return_value = (train_dataset, None, None)

        inference_results = {
            "dataset": DatasetVariable.create(None),
        }

        mock_llm_inference.return_value = inference_results

        mock_evaluator.return_value = None

        inference_config = LLMInferenceConfig()
        evaluator_config = EvaluatorConfig()
        dataset_plugin_config = DatasetPluginConfig()

        pusher_config = PusherConfig(
            items=[
                PusherPluginConfig(name="dataset_inference_result", dataset_plugin=dataset_plugin_config),
            ],
        )
        task_configs = {
            "llm_feature_prep": TaskConfig(task_function="llm_feature_prep", config={}),
            "llm_inference": TaskConfig(task_function="llm_inference", config=inference_config),
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "pusher": TaskConfig(task_function="pusher", config=pusher_config),
        }

        llm_eval(WorkflowConfig(), task_configs)

        mock_llm_inference.assert_called_once_with(config=task_configs["llm_inference"], datasets={"dataset": train_dataset})
        mock_evaluator.assert_called_once_with(config=task_configs["evaluator"], datasets=inference_results)
        mock_dataset_pusher_plugin_execute.assert_called()
