import os
from unittest import TestCase
from unittest.mock import patch, MagicMock
from uber.ai.michelangelo.sdk.workflow.variables import DatasetVariable, ModelVariable, FeaturePackageVariable
from uber.ai.michelangelo.sdk.workflow.variables.types import AssembledModel, TransformResult
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.transform import TabularTransformConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.trainer import TabularTrainerConfig, CustomTrainerConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.assembler import TabularAssemblerConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.inference import TabularInferenceConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.evaluator import EvaluatorConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.pusher import PusherConfig, PusherPluginConfig, ModelPluginConfig, DatasetPluginConfig
from uber.ai.michelangelo.sdk.workflow.defs.tabular_train import workflow_function as tabular_train


class WorkflowTest(TestCase):
    @patch.dict(os.environ, {"MA_NAMESPACE": "test"})
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_feature_prep")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_trainer")
    @patch("uber.ai.michelangelo.sdk.workflow.tasks.tabular_inference.task.DatasetVariable.load_ray_dataset")
    @patch("uber.ai.michelangelo.sdk.workflow.tasks.tabular_inference.task.DatasetVariable.save_ray_dataset")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_assembler")
    @patch("uber.ai.michelangelo.sdk.workflow.tasks.pusher.task.ModelPusherPlugin.execute")
    @patch("uber.ai.michelangelo.sdk.workflow.tasks.pusher.task.DataSetPusherPlugin.execute")
    def test_tabular_train(
        self,
        mock_dataset_pusher_plugin_execute,
        mock_model_pusher_plugin_execute,
        mock_tabular_assembler,
        mock_tabular_inference_save_ray_dataset,
        mock_tabular_inference_load_ray_dataset,
        mock_tabular_trainer,
        mock_tabular_feature_prep,
    ):
        train_dataset = DatasetVariable.create(None)
        validation_dataset = DatasetVariable.create(None)
        test_dataset = DatasetVariable.create(None)
        model = ModelVariable.create(None)

        mock_tabular_feature_prep.return_value = (train_dataset, validation_dataset, test_dataset, None)
        mock_tabular_trainer.return_value = model
        mock_tabular_assembler.return_value = AssembledModel(
            raw_model=model,
            deployable_model=model,
            performance_evaluation_report=None,
            feature_evaluation_report=None,
            feature_quality_report=None,
        )

        trainer_config = TabularTrainerConfig(
            custom=CustomTrainerConfig(
                train_class="foo.bar.Train",
            ),
        )
        assembler_config = TabularAssemblerConfig()
        inference_config = TabularInferenceConfig()
        evaluator_config = EvaluatorConfig()
        model_plugin_config = ModelPluginConfig()
        dataset_plugin_config = DatasetPluginConfig()
        pusher_config = PusherConfig(
            items=[
                PusherPluginConfig(name="model", model_plugin=model_plugin_config),
                PusherPluginConfig(name="train_summary_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="validation_summary_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="test_summary_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="train_instance_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="validation_instance_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="train_inference_result", dataset_plugin=dataset_plugin_config),
            ],
        )
        task_configs = {
            "tabular_feature_prep": TaskConfig(task_function="tabular_feature_prep", config={}),
            "tabular_trainer": TaskConfig(task_function="tabular_trainer", config=trainer_config),
            "tabular_assembler": TaskConfig(task_function="tabular_assembler", config=assembler_config),
            "tabular_inference": TaskConfig(task_function="tabular_inference", config=inference_config),
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "pusher": TaskConfig(task_function="pusher", config=pusher_config),
        }

        tabular_train(WorkflowConfig(), task_configs)

        mock_tabular_inference_load_ray_dataset.assert_called()
        mock_tabular_inference_save_ray_dataset.assert_not_called()
        mock_dataset_pusher_plugin_execute.assert_called()
        mock_model_pusher_plugin_execute.assert_called_once()
        mock_tabular_assembler.assert_called_once()

    @patch.dict(os.environ, {"MA_NAMESPACE": "test"})
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_feature_prep")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_trainer")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_transform")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_assembler")
    @patch("uber.ai.michelangelo.sdk.workflow.tasks.tabular_inference.task.DatasetVariable.load_ray_dataset")
    @patch("uber.ai.michelangelo.sdk.workflow.tasks.tabular_inference.task.DatasetVariable.save_ray_dataset")
    @patch("uber.ai.michelangelo.sdk.workflow.tasks.pusher.task.ModelPusherPlugin.execute")
    @patch("uber.ai.michelangelo.sdk.workflow.tasks.pusher.task.DataSetPusherPlugin.execute")
    def test_tabular_train_with_transform(
        self,
        mock_dataset_pusher_plugin_execute,
        mock_model_pusher_plugin_execute,
        mock_tabular_inference_save_ray_dataset,
        mock_tabular_inference_load_ray_dataset,
        mock_tabular_assembler,
        mock_tabular_transform,
        mock_tabular_trainer,
        mock_tabular_feature_prep,
    ):
        train_dataset = DatasetVariable.create(None)
        validation_dataset = DatasetVariable.create(None)
        test_dataset = DatasetVariable.create(None)
        model = ModelVariable.create(None)

        mock_tabular_feature_prep.return_value = (train_dataset, validation_dataset, test_dataset, None)
        mock_tabular_trainer.return_value = model
        mock_tabular_transform.return_value = ({"train": train_dataset, "validation": validation_dataset}, model)
        mock_tabular_transform.return_value = TransformResult(
            transformed_datasets={"train": train_dataset, "validation": validation_dataset},
            feature_package=FeaturePackageVariable.create(None),
        )
        mock_tabular_assembler.return_value = AssembledModel(
            raw_model=model,
            deployable_model=model,
            performance_evaluation_report=None,
            feature_evaluation_report=None,
            feature_quality_report=None,
        )
        transform_config = TabularTransformConfig()
        trainer_config = TabularTrainerConfig(
            custom=CustomTrainerConfig(
                train_class="foo.bar.Train",
            ),
        )
        assembler_config = TabularAssemblerConfig()
        inference_config = TabularInferenceConfig()
        evaluator_config = EvaluatorConfig()
        model_plugin_config = ModelPluginConfig()
        dataset_plugin_config = DatasetPluginConfig()
        pusher_config = PusherConfig(
            items=[
                PusherPluginConfig(name="model", model_plugin=model_plugin_config),
                PusherPluginConfig(name="train_summary_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="validation_summary_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="test_summary_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="train_instance_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="validation_instance_metrics", dataset_plugin=dataset_plugin_config),
                PusherPluginConfig(name="train_inference_result", dataset_plugin=dataset_plugin_config),
            ],
        )
        task_configs = {
            "tabular_feature_prep": TaskConfig(task_function="tabular_feature_prep", config={}),
            "tabular_transform": TaskConfig(task_function="tabular_transform", config=transform_config),
            "tabular_trainer": TaskConfig(task_function="tabular_trainer", config=trainer_config),
            "tabular_assembler": TaskConfig(task_function="tabular_assembler", config=assembler_config),
            "tabular_inference": TaskConfig(task_function="tabular_inference", config=inference_config),
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "pusher": TaskConfig(task_function="pusher", config=pusher_config),
        }

        tabular_train(WorkflowConfig(), task_configs)

        mock_tabular_inference_load_ray_dataset.assert_called()
        mock_tabular_inference_save_ray_dataset.assert_not_called()
        mock_tabular_assembler.assert_called_once()
        mock_dataset_pusher_plugin_execute.assert_called()
        mock_model_pusher_plugin_execute.assert_called_once()

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_feature_prep")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_trainer")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_assembler")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.evaluator")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.pusher")
    def test_tabular_train_with_no_metrics(
        self,
        mock_pusher,
        mock_evaluator,
        mock_tabular_inference,
        mock_tabular_assembler,
        mock_tabular_trainer,
        mock_tabular_feature_prep,
    ):
        """Test when evaluation_result.metrics.get() returns None for some datasets"""
        train_dataset = DatasetVariable.create(None)
        validation_dataset = DatasetVariable.create(None)
        test_dataset = DatasetVariable.create(None)
        model = ModelVariable.create(None)

        mock_tabular_feature_prep.return_value = (train_dataset, validation_dataset, test_dataset, None)
        mock_tabular_trainer.return_value = model
        mock_tabular_assembler.return_value = AssembledModel(
            raw_model=model,
            deployable_model=model,
            performance_evaluation_report=None,
            feature_evaluation_report=None,
            feature_quality_report=None,
        )
        mock_tabular_inference.return_value = {
            "train": train_dataset,
            "validation": validation_dataset,
            "test": test_dataset,
        }

        # Mock evaluator to return None metrics for some datasets
        mock_evaluation_result = MagicMock()
        mock_evaluation_result.metrics.get.side_effect = lambda _: None  # Always return None
        mock_evaluation_result.reports.get.return_value = None
        mock_evaluator.return_value = mock_evaluation_result

        # Mock pusher to return a model result
        mock_pusher_result = MagicMock()
        mock_pusher_result.name = "model"
        mock_pusher_result.plugin = "model_plugin"
        mock_pusher_result.value = "test_model_name"
        mock_pusher.return_value = [mock_pusher_result]

        trainer_config = TabularTrainerConfig(
            custom=CustomTrainerConfig(
                train_class="foo.bar.Train",
            ),
        )
        assembler_config = TabularAssemblerConfig()
        inference_config = TabularInferenceConfig()
        evaluator_config = EvaluatorConfig()
        model_plugin_config = ModelPluginConfig()
        pusher_config = PusherConfig(
            items=[
                PusherPluginConfig(name="model", model_plugin=model_plugin_config),
            ],
        )
        task_configs = {
            "tabular_feature_prep": TaskConfig(task_function="tabular_feature_prep", config={}),
            "tabular_trainer": TaskConfig(task_function="tabular_trainer", config=trainer_config),
            "tabular_assembler": TaskConfig(task_function="tabular_assembler", config=assembler_config),
            "tabular_inference": TaskConfig(task_function="tabular_inference", config=inference_config),
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "pusher": TaskConfig(task_function="pusher", config=pusher_config),
        }

        result = tabular_train(WorkflowConfig(), task_configs)

        # Verify that the workflow handles None metrics gracefully
        self.assertEqual(result["model_name"], "test_model_name")

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_feature_prep")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_trainer")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_assembler")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.evaluator")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.pusher")
    def test_tabular_train_with_empty_pusher_results(
        self,
        mock_pusher,
        mock_evaluator,
        mock_tabular_inference,
        mock_tabular_assembler,
        mock_tabular_trainer,
        mock_tabular_feature_prep,
    ):
        """Test when pusher returns empty results"""
        train_dataset = DatasetVariable.create(None)
        validation_dataset = DatasetVariable.create(None)
        test_dataset = DatasetVariable.create(None)
        model = ModelVariable.create(None)

        mock_tabular_feature_prep.return_value = (train_dataset, validation_dataset, test_dataset, None)
        mock_tabular_trainer.return_value = model
        mock_tabular_assembler.return_value = model
        mock_tabular_inference.return_value = {
            "train": train_dataset,
            "validation": validation_dataset,
            "test": test_dataset,
        }

        # Mock evaluator to return valid metrics
        mock_evaluation_result = MagicMock()
        mock_metrics = MagicMock()
        mock_metrics.summary_metrics = "summary"
        mock_metrics.instance_metrics = "instance"
        mock_evaluation_result.metrics.get.return_value = mock_metrics
        mock_evaluation_result.reports.get.return_value = None
        mock_evaluator.return_value = mock_evaluation_result

        # Mock pusher to return empty results
        mock_pusher.return_value = []

        trainer_config = TabularTrainerConfig(
            custom=CustomTrainerConfig(
                train_class="foo.bar.Train",
            ),
        )
        assembler_config = TabularAssemblerConfig()
        inference_config = TabularInferenceConfig()
        evaluator_config = EvaluatorConfig()
        model_plugin_config = ModelPluginConfig()
        pusher_config = PusherConfig(
            items=[
                PusherPluginConfig(name="model", model_plugin=model_plugin_config),
            ],
        )
        task_configs = {
            "tabular_feature_prep": TaskConfig(task_function="tabular_feature_prep", config={}),
            "tabular_trainer": TaskConfig(task_function="tabular_trainer", config=trainer_config),
            "tabular_assembler": TaskConfig(task_function="tabular_assembler", config=assembler_config),
            "tabular_inference": TaskConfig(task_function="tabular_inference", config=inference_config),
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "pusher": TaskConfig(task_function="pusher", config=pusher_config),
        }

        result = tabular_train(WorkflowConfig(), task_configs)

        # Verify that the workflow handles empty pusher results gracefully
        self.assertIsNone(result["model_name"])

    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_feature_prep")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_trainer")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_assembler")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.tabular_inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.evaluator")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.tabular_train.workflow.pusher")
    def test_tabular_train_with_no_matching_model_in_pusher_results(
        self,
        mock_pusher,
        mock_evaluator,
        mock_tabular_inference,
        mock_tabular_assembler,
        mock_tabular_trainer,
        mock_tabular_feature_prep,
    ):
        """Test when pusher results don't contain a matching model"""
        train_dataset = DatasetVariable.create(None)
        validation_dataset = DatasetVariable.create(None)
        test_dataset = DatasetVariable.create(None)
        model = ModelVariable.create(None)

        mock_tabular_feature_prep.return_value = (train_dataset, validation_dataset, test_dataset, None)
        mock_tabular_trainer.return_value = model
        mock_tabular_assembler.return_value = model
        mock_tabular_inference.return_value = {
            "train": train_dataset,
            "validation": validation_dataset,
            "test": test_dataset,
        }

        # Mock evaluator to return valid metrics
        mock_evaluation_result = MagicMock()
        mock_metrics = MagicMock()
        mock_metrics.summary_metrics = "summary"
        mock_metrics.instance_metrics = "instance"
        mock_evaluation_result.metrics.get.return_value = mock_metrics
        mock_evaluation_result.reports.get.return_value = None
        mock_evaluator.return_value = mock_evaluation_result

        # Mock pusher to return results without matching model
        mock_pusher_result1 = MagicMock()
        mock_pusher_result1.name = "other_artifact"
        mock_pusher_result1.plugin = "other_plugin"
        mock_pusher_result1.value = "other_value"

        mock_pusher_result2 = MagicMock()
        mock_pusher_result2.name = "model"
        mock_pusher_result2.plugin = "different_plugin"  # Wrong plugin
        mock_pusher_result2.value = "wrong_model"

        mock_pusher.return_value = [mock_pusher_result1, mock_pusher_result2]

        trainer_config = TabularTrainerConfig(
            custom=CustomTrainerConfig(
                train_class="foo.bar.Train",
            ),
        )
        assembler_config = TabularAssemblerConfig()
        inference_config = TabularInferenceConfig()
        evaluator_config = EvaluatorConfig()
        model_plugin_config = ModelPluginConfig()
        pusher_config = PusherConfig(
            items=[
                PusherPluginConfig(name="model", model_plugin=model_plugin_config),
            ],
        )
        task_configs = {
            "tabular_feature_prep": TaskConfig(task_function="tabular_feature_prep", config={}),
            "tabular_trainer": TaskConfig(task_function="tabular_trainer", config=trainer_config),
            "tabular_assembler": TaskConfig(task_function="tabular_assembler", config=assembler_config),
            "tabular_inference": TaskConfig(task_function="tabular_inference", config=inference_config),
            "evaluator": TaskConfig(task_function="evaluator", config=evaluator_config),
            "pusher": TaskConfig(task_function="pusher", config=pusher_config),
        }

        result = tabular_train(WorkflowConfig(), task_configs)

        # Verify that the workflow handles non-matching pusher results gracefully
        self.assertIsNone(result["model_name"])
