from unittest import TestCase
from unittest.mock import patch, MagicMock
from uber.ai.michelangelo.sdk.workflow.variables import DatasetVariable
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.evaluator import EvaluatorConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.pusher import PusherConfig, PusherPluginConfig, DatasetPluginConfig
from uber.ai.michelangelo.sdk.workflow.defs.tabular_eval import workflow_function as tabular_eval
from uber.ai.michelangelo.sdk.workflow.variables.types import EvaluationResult, EvaluationMetrics


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
        self.pusher_config = PusherConfig(
            items=[PusherPluginConfig(name="prediction_result", dataset_plugin=DatasetPluginConfig())],
        )

        # Mock return values
        self.mock_inference_result = {"dataset": self.dataset}

        self.mock_evaluation_result = EvaluationResult(
            metrics={
                "dataset": EvaluationMetrics(
                    instance_metrics=DatasetVariable.create(None),
                    summary_metrics=DatasetVariable.create(None),
                )
            },
            reports={},
        )

        self.mock_comparator_result = MagicMock()

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.tabular_feature_prep")
    def test_basic_workflow_minimal_config(self, mock_feature_prep, mock_inference, mock_pusher):
        """Test basic workflow with minimal required configuration (no evaluator or comparator)."""
        # Setup mocks
        mock_feature_prep.return_value = (self.dataset, self.validation_dataset, self.test_dataset, None)
        mock_inference.with_overrides.return_value.return_value = self.mock_inference_result

        # Create minimal task configs
        task_configs = {
            "tabular_feature_prep": self.tabular_feature_prep_config,
            "inference": self.inference_config,
            "pusher": TaskConfig(task_function="pusher", config=self.pusher_config),
        }

        # Execute workflow
        result = tabular_eval(self.workflow_config, task_configs)

        # Verify all required steps were called
        mock_feature_prep.assert_called_once_with(config=self.tabular_feature_prep_config)
        mock_inference.with_overrides.assert_called_once_with(alias="inference")
        mock_pusher.assert_called_once_with(config=task_configs["pusher"], items={"prediction_result": self.dataset})

        # Verify return value when no comparator
        self.assertEqual(result, {})

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.evaluator")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.tabular_feature_prep")
    def test_workflow_with_evaluator_only(self, mock_feature_prep, mock_inference, mock_evaluator, mock_pusher):
        """Test workflow with evaluator but no comparator."""
        # Setup mocks
        mock_feature_prep.return_value = (self.dataset, self.validation_dataset, self.test_dataset, None)
        mock_inference.with_overrides.return_value.return_value = self.mock_inference_result
        mock_evaluator.return_value = self.mock_evaluation_result

        # Create task configs with evaluator
        evaluator_config = EvaluatorConfig()
        task_configs = {
            "tabular_feature_prep": self.tabular_feature_prep_config,
            "inference": self.inference_config,
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "pusher": TaskConfig(task_function="pusher", config=self.pusher_config),
        }

        # Execute workflow
        result = tabular_eval(self.workflow_config, task_configs)

        # Verify evaluator was called
        mock_evaluator.assert_called_once_with(config=task_configs["evaluator"], datasets={"dataset": self.dataset})

        # Verify artifacts include evaluation metrics but no comparator_result
        expected_artifacts = {
            "prediction_result": self.dataset,
            "summary_metrics": self.mock_evaluation_result.metrics["dataset"].summary_metrics,
            "instance_metrics": self.mock_evaluation_result.metrics["dataset"].instance_metrics,
        }
        mock_pusher.assert_called_once_with(config=task_configs["pusher"], items=expected_artifacts)

        # Verify return value when no comparator
        self.assertEqual(result, {})

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.comparator")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.tabular_feature_prep")
    def test_workflow_with_comparator_only(self, mock_feature_prep, mock_inference, mock_comparator, mock_pusher):
        """Test workflow with comparator but no evaluator."""
        # Setup mocks
        mock_feature_prep.return_value = (self.dataset, self.validation_dataset, self.test_dataset, None)
        mock_inference.with_overrides.return_value.return_value = self.mock_inference_result
        mock_comparator.return_value = self.mock_comparator_result

        # Create task configs with comparator only
        task_configs = {
            "tabular_feature_prep": self.tabular_feature_prep_config,
            "inference": self.inference_config,
            "comparator": TaskConfig(task_function="comparator", config={}),
            "pusher": TaskConfig(task_function="pusher", config=self.pusher_config),
        }

        # Execute workflow
        result = tabular_eval(self.workflow_config, task_configs)

        # Verify comparator was called with inference results
        mock_comparator.assert_called_once_with(config=task_configs["comparator"], datasets={"dataset": self.dataset})

        # Verify artifacts include comparator result
        expected_artifacts = {
            "prediction_result": self.dataset,
            "comparator_result": self.mock_comparator_result,
        }
        mock_pusher.assert_called_once_with(config=task_configs["pusher"], items=expected_artifacts)

        # Verify return value when comparator exists
        self.assertEqual(result, {"comparator_result": self.mock_comparator_result})

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.comparator")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.evaluator")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.tabular_feature_prep")
    def test_workflow_with_both_evaluator_and_comparator(self, mock_feature_prep, mock_inference, mock_evaluator, mock_comparator, mock_pusher):
        """Test workflow with both evaluator and comparator."""
        # Setup mocks
        mock_feature_prep.return_value = (self.dataset, self.validation_dataset, self.test_dataset, None)
        mock_inference.with_overrides.return_value.return_value = self.mock_inference_result
        mock_evaluator.return_value = self.mock_evaluation_result
        mock_comparator.return_value = self.mock_comparator_result

        # Create task configs with both evaluator and comparator
        evaluator_config = EvaluatorConfig()
        task_configs = {
            "tabular_feature_prep": self.tabular_feature_prep_config,
            "inference": self.inference_config,
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "comparator": TaskConfig(task_function="comparator", config={}),
            "pusher": TaskConfig(task_function="pusher", config=self.pusher_config),
        }

        # Execute workflow
        result = tabular_eval(self.workflow_config, task_configs)

        # Verify both evaluator and comparator were called
        mock_evaluator.assert_called_once_with(config=task_configs["evaluator"], datasets={"dataset": self.dataset})
        mock_comparator.assert_called_once_with(config=task_configs["comparator"], datasets={"dataset": self.dataset})

        # Verify artifacts include both evaluation metrics and comparator result
        expected_artifacts = {
            "prediction_result": self.dataset,
            "summary_metrics": self.mock_evaluation_result.metrics["dataset"].summary_metrics,
            "instance_metrics": self.mock_evaluation_result.metrics["dataset"].instance_metrics,
            "comparator_result": self.mock_comparator_result,
        }
        mock_pusher.assert_called_once_with(config=task_configs["pusher"], items=expected_artifacts)

        # Verify return value when comparator exists
        self.assertEqual(result, {"comparator_result": self.mock_comparator_result})

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.evaluator")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.tabular_feature_prep")
    def test_workflow_evaluator_with_no_metrics(self, mock_feature_prep, mock_inference, mock_evaluator, mock_pusher):
        """Test workflow when evaluator returns no metrics."""
        # Setup mocks
        mock_feature_prep.return_value = (self.dataset, self.validation_dataset, self.test_dataset, None)
        mock_inference.with_overrides.return_value.return_value = self.mock_inference_result

        # Create evaluation result with no metrics
        evaluation_result_no_metrics = EvaluationResult(
            metrics={},  # No metrics
            reports={},
        )
        mock_evaluator.return_value = evaluation_result_no_metrics

        # Create task configs with evaluator
        evaluator_config = EvaluatorConfig()
        task_configs = {
            "tabular_feature_prep": self.tabular_feature_prep_config,
            "inference": self.inference_config,
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "pusher": TaskConfig(task_function="pusher", config=self.pusher_config),
        }

        # Execute workflow
        result = tabular_eval(self.workflow_config, task_configs)

        # Verify only prediction_result is in artifacts (no metrics)
        expected_artifacts = {
            "prediction_result": self.dataset,
        }
        mock_pusher.assert_called_once_with(config=task_configs["pusher"], items=expected_artifacts)

        # Verify return value when no comparator
        self.assertEqual(result, {})

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.evaluator")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.tabular_feature_prep")
    def test_workflow_evaluator_with_none_metrics(self, mock_feature_prep, mock_inference, mock_evaluator, mock_pusher):
        """Test workflow when evaluator returns None for dataset metrics."""
        # Setup mocks
        mock_feature_prep.return_value = (self.dataset, self.validation_dataset, self.test_dataset, None)
        mock_inference.with_overrides.return_value.return_value = self.mock_inference_result

        # Create evaluation result with None metrics for dataset
        evaluation_result_none_metrics = EvaluationResult(
            metrics={
                "dataset": None  # None metrics
            },
            reports={},
        )
        mock_evaluator.return_value = evaluation_result_none_metrics

        # Create task configs with evaluator
        evaluator_config = EvaluatorConfig()
        task_configs = {
            "tabular_feature_prep": self.tabular_feature_prep_config,
            "inference": self.inference_config,
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "pusher": TaskConfig(task_function="pusher", config=self.pusher_config),
        }

        # Execute workflow
        result = tabular_eval(self.workflow_config, task_configs)

        # Verify only prediction_result is in artifacts (no metrics added)
        expected_artifacts = {
            "prediction_result": self.dataset,
        }
        mock_pusher.assert_called_once_with(config=task_configs["pusher"], items=expected_artifacts)

        # Verify return value when no comparator
        self.assertEqual(result, {})

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.tabular_feature_prep")
    def test_workflow_inference_returns_none_dataset(self, mock_feature_prep, mock_inference, mock_pusher):
        """Test workflow when tabular_inference returns None for dataset."""
        # Setup mocks
        mock_feature_prep.return_value = (self.dataset, self.validation_dataset, self.test_dataset, None)
        mock_inference.with_overrides.return_value.return_value = {"dataset": None}

        # Create minimal task configs
        task_configs = {
            "tabular_feature_prep": self.tabular_feature_prep_config,
            "inference": self.inference_config,
            "pusher": TaskConfig(task_function="pusher", config=self.pusher_config),
        }

        # Execute workflow
        result = tabular_eval(self.workflow_config, task_configs)

        # Verify artifacts handle None dataset
        expected_artifacts = {
            "prediction_result": None,
        }
        mock_pusher.assert_called_once_with(config=task_configs["pusher"], items=expected_artifacts)

        # Verify return value when no comparator
        self.assertEqual(result, {})

    @patch("uber.ai.michelangelo.sdk.workflow.tasks.pusher.task.DataSetPusherPlugin.execute")
    def test_eval(self, mock_dataset_pusher_plugin_execute):
        """Original test maintained for backward compatibility."""
        train_dataset = DatasetVariable.create(None)
        validation_dataset = DatasetVariable.create(None)
        test_dataset = DatasetVariable.create(None)

        with (
            patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.tabular_feature_prep") as mock_tabular_feature_prep,
            patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.inference") as mock_inference,
            patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_eval.workflow.evaluator") as mock_evaluator,
        ):
            mock_tabular_feature_prep.return_value = (train_dataset, validation_dataset, test_dataset, None)
            mock_inference.with_overrides.return_value.return_value = {"dataset": train_dataset}
            mock_evaluator.return_value = EvaluationResult(
                metrics={
                    "dataset": EvaluationMetrics(
                        # Instance metrics are metrics computed for each instance in the dataset.
                        instance_metrics=DatasetVariable.create(None),
                        # Summary metrics are metrics computed for the dataset as an aggregate
                        summary_metrics=DatasetVariable.create(None),
                    )
                },
                reports={},
            )

            evaluator_config = EvaluatorConfig()
            dataset_plugin_config = DatasetPluginConfig()
            pusher_config = PusherConfig(
                items=[PusherPluginConfig(name="summary_metrics", dataset_plugin=dataset_plugin_config)],
            )
            task_configs = {
                "tabular_feature_prep": TaskConfig(task_function="tabular_feature_prep", config={}),
                "inference": TaskConfig(task_function="inference", config={}),
                "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
                "pusher": TaskConfig(task_function="pusher", config=pusher_config),
            }

            result = tabular_eval(self.workflow_config, task_configs)

            mock_dataset_pusher_plugin_execute.assert_called()

            # Verify return value when no comparator
            self.assertEqual(result, {})
