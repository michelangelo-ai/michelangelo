from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.workflow.variables import DatasetVariable
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig
from uber.ai.michelangelo.sdk.workflow.defs.tabular_inference import workflow_function as tabular_inference


class WorkflowTest(TestCase):
    def setUp(self):
        """Set up common test fixtures."""
        self.workflow_config = WorkflowConfig()
        self.dataset = DatasetVariable.create(None)
        self.validation_dataset = DatasetVariable.create(None)
        self.test_dataset = DatasetVariable.create(None)

        # Common task configs
        self.tabular_feature_prep_config = TaskConfig(task_function="tabular_feature_prep", config={})
        self.inference_config = TaskConfig(task_function="inference", config={})
        self.pusher_config = TaskConfig(task_function="pusher", config={})

        # Mock return values
        self.mock_inference_result = {"dataset": self.dataset}

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_inference.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_inference.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_inference.workflow.tabular_feature_prep")
    def test_tabular_inference(self, mock_feature_prep, mock_inference, mock_pusher):
        """Test basic workflow execution with valid inputs."""
        # Setup mocks
        mock_feature_prep.return_value = (self.dataset, None, None, None)
        mock_inference.with_overrides.return_value.return_value = self.mock_inference_result

        # Create task configs
        task_configs = {
            "tabular_feature_prep": self.tabular_feature_prep_config,
            "inference": self.inference_config,
            "pusher": self.pusher_config,
        }

        tabular_inference(self.workflow_config, task_configs)

        mock_feature_prep.assert_called_once_with(config=self.tabular_feature_prep_config)
        mock_inference.return_value = self.mock_inference_result

        expected_artifacts = {
            "inference_result": self.dataset,
        }
        mock_pusher.assert_called_once_with(config=self.pusher_config, items=expected_artifacts)
