"""
Unittest for llm_api_inference workflow def
"""

from unittest import TestCase
from unittest.mock import patch, MagicMock

from uber.ai.michelangelo.sdk.workflow.defs.llm_api_inference import (
    workflow_function as llm_api_inference,
)


_PKG_WF = "uber.ai.michelangelo.sdk.workflow.defs.llm_api_inference.workflow"


class WorkflowTest(TestCase):
    """
    Workflow unittest for llm api inference pipeline
    """

    @patch(f"{_PKG_WF}.pusher")
    @patch(f"{_PKG_WF}.llm_retrieve_batch")
    @patch(f"{_PKG_WF}.llm_create_batch")
    @patch(f"{_PKG_WF}.llm_feature_prep")
    def test_llm_api_inference(
        self,
        mock_llm_feature_prep: MagicMock,
        mock_create_batches: MagicMock,
        mock_retrieve_batches: MagicMock,
        mock_pusher: MagicMock,
    ):
        """
        Test with all mocked steps
        """
        # Set step i/o with mock
        task_configs = {
            "llm_feature_prep": MagicMock(),
            "create_batches": MagicMock(),
            "retrieve_batches": MagicMock(),
            "pusher": MagicMock(),
        }

        mock_dataset = MagicMock()
        mock_llm_feature_prep.return_value = (mock_dataset, None, None)

        mock_batch_res = MagicMock()
        mock_batch_data_res = MagicMock()
        mock_create_batches.return_value = (
            mock_batch_res,
            {"dataset": mock_batch_data_res},
        )
        mock_retrieve_res = MagicMock()
        mock_retrieve_batches.return_value = mock_retrieve_res

        # Act
        llm_api_inference(task_configs)

        # Assert
        mock_llm_feature_prep.assert_called_once_with(
            config=task_configs["llm_feature_prep"],
        )
        mock_create_batches.assert_called_once_with(
            config=task_configs["create_batches"],
            datasets={"dataset": mock_dataset},
        )
        mock_retrieve_batches.assert_called_once_with(
            config=task_configs["retrieve_batches"],
            datasets={"dataset": mock_batch_data_res},
            batches=mock_batch_res,
        )
        mock_pusher.assert_called_once_with(
            config=task_configs["pusher"],
            items=mock_retrieve_res,
        )
