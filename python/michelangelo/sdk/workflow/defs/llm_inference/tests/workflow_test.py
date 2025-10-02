from unittest import TestCase
from unittest.mock import patch, MagicMock

from uber.ai.michelangelo.sdk.workflow.defs.llm_inference import workflow_function as llm_inference


class WorkflowTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.workflow.defs.llm_inference.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.llm_inference.workflow.llm_inference_task")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.llm_inference.workflow.llm_feature_prep")
    def test_llm_inference(self, mock_llm_feature_prep, mock_llm_inference_task, mock_pusher):
        # Arrange
        task_configs = {"llm_feature_prep": MagicMock(), "llm_inference": MagicMock(), "pusher": MagicMock()}

        mock_dataset = MagicMock()
        mock_llm_feature_prep.return_value = (mock_dataset, None, None)

        mock_inference_results = MagicMock()
        mock_llm_inference_task.return_value = mock_inference_results

        # Act
        llm_inference(task_configs)

        # Assert
        mock_llm_feature_prep.assert_called_once_with(config=task_configs["llm_feature_prep"])
        mock_llm_inference_task.assert_called_once_with(config=task_configs["llm_inference"], datasets={"train": mock_dataset})
        mock_pusher.assert_called_once_with(config=task_configs["pusher"], items={"dataset": mock_inference_results})
