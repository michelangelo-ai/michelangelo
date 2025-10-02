from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.document_loader import DocumentLoaderConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.inference import LLMSparkScorerConfig, LLMInferenceConfig
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.pusher import (
    PusherConfig,
    PusherPluginConfig,
    VectorDatasetHivePluginConfig,
    VectorDatasetOsPluginConfig,
    VectorDatasetPluginConfig,
)
from uber.ai.michelangelo.sdk.workflow.variables import DatasetVariable
from uber.ai.michelangelo.sdk.workflow.variables._private.message import MessageVariable
from uber.ai.michelangelo.sdk.workflow.variables.metadata.dataset import DatasetMetadata
from uber.ai.michelangelo.sdk.workflow.variables.types import PusherPluginResult, RawVectorDataset
from uber.ai.michelangelo.sdk.workflow.defs.vector_gen import workflow_function as vector_gen
from uber.ai.michelangelo.sdk.workflow.tasks.llm_multistage_scorer.config.stage_configs import LLMMultistageScorerConfig


class WorkflowTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_online")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_offline")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.scorer")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.document_loader")
    def test_vector_gen_workflow(self, mock_document_loader, mock_scorer, mock_pusher_offline, mock_pusher_online, mock_pusher):
        loaded_documents = DatasetVariable.create(None)
        embeddings = DatasetVariable.create(None)
        embeddings.metadata = DatasetMetadata(output_column_metadata=[])
        raw_vector_dataset = RawVectorDataset(
            offline_dataset_schema="test",
            offline_dataset=MessageVariable.create(VectorDatasetHivePluginConfig()),
            output_column_metadata=[],
        )
        offline_plugin_result = PusherPluginResult(name="offline", plugin="offline", value=(None, None))
        online_plugin_result = PusherPluginResult(name="online", plugin="online", value=raw_vector_dataset)

        mock_scorer.with_overrides.return_value = mock_scorer
        mock_pusher_offline.with_overrides.return_value = mock_pusher_offline
        mock_pusher_online.with_overrides.return_value = mock_pusher_online

        mock_document_loader.return_value = (loaded_documents, raw_vector_dataset)
        mock_scorer.return_value = embeddings
        mock_pusher_offline.return_value = [offline_plugin_result]
        mock_pusher_online.return_value = [online_plugin_result]

        document_loader_config = DocumentLoaderConfig()
        scorer_config = LLMSparkScorerConfig()
        hive_pusher_config = VectorDatasetHivePluginConfig()
        os_pusher_config = VectorDatasetOsPluginConfig()
        uapi_pusher_config = VectorDatasetPluginConfig()
        pusher_offline_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_hive_plugin=hive_pusher_config)])
        pusher_online_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_os_plugin=os_pusher_config)])
        pusher_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_plugin=uapi_pusher_config)])

        task_configs = {
            "document_loader": TaskConfig(task_function="document_loader", config=document_loader_config),
            "scorer": TaskConfig(task_function="scorer", config=scorer_config),
            "pusher_offline": TaskConfig(task_function="pusher_offline", config=pusher_offline_config),
            "pusher_online": TaskConfig(task_function="pusher_online", config=pusher_online_config),
            "pusher": TaskConfig(task_function="pusher", config=pusher_config),
        }

        vector_gen(WorkflowConfig(), task_configs)
        mock_document_loader.assert_called_once()
        mock_scorer.assert_called_once()
        mock_pusher_offline.assert_called_once()
        mock_pusher_online.assert_called_once()
        mock_pusher.assert_called_once()

    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_online")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_offline")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.scorer")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.document_loader")
    def test_vector_gen_workflow_without_pusher_online(self, mock_document_loader, mock_scorer, mock_pusher_offline, mock_pusher_online, mock_pusher):
        loaded_documents = DatasetVariable.create(None)
        embeddings = DatasetVariable.create(None)
        embeddings.metadata = DatasetMetadata(output_column_metadata=[])
        raw_vector_dataset = RawVectorDataset(
            offline_dataset_schema="test",
            offline_dataset=MessageVariable.create(VectorDatasetHivePluginConfig()),
            output_column_metadata=[],
        )
        offline_plugin_result = PusherPluginResult(name="offline", plugin="offline", value=(None, None))
        online_plugin_result = PusherPluginResult(name="online", plugin="online", value=raw_vector_dataset)

        mock_scorer.with_overrides.return_value = mock_scorer
        mock_pusher_offline.with_overrides.return_value = mock_pusher_offline
        mock_pusher_online.with_overrides.return_value = mock_pusher_online

        mock_document_loader.return_value = (loaded_documents, raw_vector_dataset)
        mock_scorer.return_value = embeddings
        mock_pusher_offline.return_value = [offline_plugin_result]
        mock_pusher_online.return_value = [online_plugin_result]

        document_loader_config = DocumentLoaderConfig()
        scorer_config = LLMSparkScorerConfig()
        hive_pusher_config = VectorDatasetHivePluginConfig()
        pusher_offline_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_hive_plugin=hive_pusher_config)])

        task_configs = {
            "document_loader": TaskConfig(task_function="document_loader", config=document_loader_config),
            "scorer": TaskConfig(task_function="scorer", config=scorer_config),
            "pusher_offline": TaskConfig(task_function="pusher_offline", config=pusher_offline_config),
        }

        raw_vector_dataset_artifact = vector_gen(WorkflowConfig(), task_configs)
        mock_document_loader.assert_called_once()
        mock_scorer.assert_called_once()
        mock_pusher_offline.assert_called_once()
        mock_pusher_online.assert_not_called()
        mock_pusher.assert_not_called()
        self.assertEqual(raw_vector_dataset_artifact, {"default": raw_vector_dataset})

    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_online")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_offline")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.document_loader")
    def test_vector_gen_workflow_with_inference(self, mock_document_loader, mock_inference, mock_pusher_offline, mock_pusher_online, mock_pusher):
        loaded_documents = DatasetVariable.create(None)
        embeddings = DatasetVariable.create(None)
        embeddings.metadata = DatasetMetadata(output_column_metadata=[])
        raw_vector_dataset = RawVectorDataset(
            offline_dataset_schema="test",
            offline_dataset=MessageVariable.create(VectorDatasetHivePluginConfig()),
            output_column_metadata=[],
        )
        offline_plugin_result = PusherPluginResult(name="offline", plugin="offline", value=(None, None))
        online_plugin_result = PusherPluginResult(name="online", plugin="online", value=raw_vector_dataset)

        mock_inference.with_overrides.return_value = mock_inference
        mock_pusher_offline.with_overrides.return_value = mock_pusher_offline
        mock_pusher_online.with_overrides.return_value = mock_pusher_online

        mock_document_loader.return_value = (loaded_documents, raw_vector_dataset)
        mock_inference.return_value = {"documents": embeddings}
        mock_pusher_offline.return_value = [offline_plugin_result]
        mock_pusher_online.return_value = [online_plugin_result]

        document_loader_config = DocumentLoaderConfig()
        inference_config = LLMInferenceConfig()
        hive_pusher_config = VectorDatasetHivePluginConfig()
        os_pusher_config = VectorDatasetOsPluginConfig()
        uapi_pusher_config = VectorDatasetPluginConfig()
        pusher_offline_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_hive_plugin=hive_pusher_config)])
        pusher_online_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_os_plugin=os_pusher_config)])
        pusher_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_plugin=uapi_pusher_config)])

        task_configs = {
            "document_loader": TaskConfig(task_function="document_loader", config=document_loader_config),
            "inference": TaskConfig(task_function="inference", config=inference_config),
            "pusher_offline": TaskConfig(task_function="pusher_offline", config=pusher_offline_config),
            "pusher_online": TaskConfig(task_function="pusher_online", config=pusher_online_config),
            "pusher": TaskConfig(task_function="pusher", config=pusher_config),
        }

        vector_gen(WorkflowConfig(), task_configs)
        mock_document_loader.assert_called_once()
        mock_inference.assert_called_once()
        mock_pusher_offline.assert_called_once()
        mock_pusher_online.assert_called_once()
        mock_pusher.assert_called_once()

    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_online")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_offline")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.multistage_scorer")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.document_loader")
    def test_vector_gen_workflow_with_multistage_scorer(
        self, mock_document_loader, mock_multistage_scorer, mock_pusher_offline, mock_pusher_online, mock_pusher
    ):
        """Test vector_gen workflow with multistage_scorer instead of scorer."""
        loaded_documents = DatasetVariable.create(None)
        embeddings = DatasetVariable.create(None)
        embeddings.metadata = DatasetMetadata(output_column_metadata=[])
        raw_vector_dataset = RawVectorDataset(
            offline_dataset_schema="test",
            offline_dataset=MessageVariable.create(VectorDatasetHivePluginConfig()),
            output_column_metadata=[],
        )
        offline_plugin_result = PusherPluginResult(name="offline", plugin="offline", value=(None, None))
        online_plugin_result = PusherPluginResult(name="online", plugin="online", value=raw_vector_dataset)

        mock_multistage_scorer.with_overrides.return_value = mock_multistage_scorer
        mock_pusher_offline.with_overrides.return_value = mock_pusher_offline
        mock_pusher_online.with_overrides.return_value = mock_pusher_online

        mock_document_loader.return_value = (loaded_documents, raw_vector_dataset)
        mock_multistage_scorer.return_value = embeddings
        mock_pusher_offline.return_value = [offline_plugin_result]
        mock_pusher_online.return_value = [online_plugin_result]

        document_loader_config = DocumentLoaderConfig()
        multistage_scorer_config = LLMMultistageScorerConfig(stages=[], fail_fast=True, preserve_intermediate=False, parallel_processing=True)
        hive_pusher_config = VectorDatasetHivePluginConfig()
        os_pusher_config = VectorDatasetOsPluginConfig()
        uapi_pusher_config = VectorDatasetPluginConfig()
        pusher_offline_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_hive_plugin=hive_pusher_config)])
        pusher_online_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_os_plugin=os_pusher_config)])
        pusher_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_plugin=uapi_pusher_config)])

        task_configs = {
            "document_loader": TaskConfig(task_function="document_loader", config=document_loader_config),
            "multistage_scorer": TaskConfig(task_function="multistage_scorer", config=multistage_scorer_config),
            "pusher_offline": TaskConfig(task_function="pusher_offline", config=pusher_offline_config),
            "pusher_online": TaskConfig(task_function="pusher_online", config=pusher_online_config),
            "pusher": TaskConfig(task_function="pusher", config=pusher_config),
        }

        vector_gen(WorkflowConfig(), task_configs)
        mock_document_loader.assert_called_once()
        mock_multistage_scorer.assert_called_once()
        mock_pusher_offline.assert_called_once()
        mock_pusher_online.assert_called_once()
        mock_pusher.assert_called_once()

    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_offline")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.multistage_scorer")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.document_loader")
    def test_vector_gen_workflow_multistage_scorer_without_online_pusher(self, mock_document_loader, mock_multistage_scorer, mock_pusher_offline):
        """Test vector_gen workflow with multistage_scorer and no online pusher."""
        loaded_documents = DatasetVariable.create(None)
        embeddings = DatasetVariable.create(None)
        embeddings.metadata = DatasetMetadata(output_column_metadata=[])
        raw_vector_dataset = RawVectorDataset(
            offline_dataset_schema="test",
            offline_dataset=MessageVariable.create(VectorDatasetHivePluginConfig()),
            output_column_metadata=[],
        )
        offline_plugin_result = PusherPluginResult(name="offline", plugin="offline", value=(None, None))

        mock_multistage_scorer.with_overrides.return_value = mock_multistage_scorer
        mock_pusher_offline.with_overrides.return_value = mock_pusher_offline

        mock_document_loader.return_value = (loaded_documents, raw_vector_dataset)
        mock_multistage_scorer.return_value = embeddings
        mock_pusher_offline.return_value = [offline_plugin_result]

        document_loader_config = DocumentLoaderConfig()
        multistage_scorer_config = LLMMultistageScorerConfig(stages=[], fail_fast=True, preserve_intermediate=False, parallel_processing=True)
        hive_pusher_config = VectorDatasetHivePluginConfig()
        pusher_offline_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_hive_plugin=hive_pusher_config)])

        task_configs = {
            "document_loader": TaskConfig(task_function="document_loader", config=document_loader_config),
            "multistage_scorer": TaskConfig(task_function="multistage_scorer", config=multistage_scorer_config),
            "pusher_offline": TaskConfig(task_function="pusher_offline", config=pusher_offline_config),
        }

        raw_vector_dataset_artifact = vector_gen(WorkflowConfig(), task_configs)
        mock_document_loader.assert_called_once()
        mock_multistage_scorer.assert_called_once()
        mock_pusher_offline.assert_called_once()
        self.assertEqual(raw_vector_dataset_artifact, {"default": raw_vector_dataset})

    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_offline")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.document_loader")
    def test_vector_gen_workflow_without_scorer_or_inference(self, mock_document_loader, mock_pusher_offline):
        """Test vector_gen workflow without any scorer or inference task."""
        loaded_documents = DatasetVariable.create(None)
        raw_vector_dataset = RawVectorDataset(
            offline_dataset_schema="test",
            offline_dataset=MessageVariable.create(VectorDatasetHivePluginConfig()),
            output_column_metadata=[],
        )
        offline_plugin_result = PusherPluginResult(name="offline", plugin="offline", value=(None, None))

        mock_pusher_offline.with_overrides.return_value = mock_pusher_offline

        mock_document_loader.return_value = (loaded_documents, raw_vector_dataset)
        mock_pusher_offline.return_value = [offline_plugin_result]

        document_loader_config = DocumentLoaderConfig()
        hive_pusher_config = VectorDatasetHivePluginConfig()
        pusher_offline_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_hive_plugin=hive_pusher_config)])

        task_configs = {
            "document_loader": TaskConfig(task_function="document_loader", config=document_loader_config),
            "pusher_offline": TaskConfig(task_function="pusher_offline", config=pusher_offline_config),
        }

        raw_vector_dataset_artifact = vector_gen(WorkflowConfig(), task_configs)
        mock_document_loader.assert_called_once()
        mock_pusher_offline.assert_called_once()
        # embeddings should be None, so raw_vector_dataset.output_column_metadata should remain as is
        # The test should still pass even though embeddings is None
        self.assertEqual(raw_vector_dataset_artifact, {"default": raw_vector_dataset})

    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_online")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.pusher_offline")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.vector_gen.workflow.document_loader")
    def test_vector_gen_workflow_inference_without_online_pusher(
        self, mock_document_loader, mock_inference, mock_pusher_offline, mock_pusher_online, mock_pusher
    ):
        """Test vector_gen workflow with inference but no online pusher configuration."""
        loaded_documents = DatasetVariable.create(None)
        embeddings = DatasetVariable.create(None)
        embeddings.metadata = DatasetMetadata(output_column_metadata=[])
        raw_vector_dataset = RawVectorDataset(
            offline_dataset_schema="test",
            offline_dataset=MessageVariable.create(VectorDatasetHivePluginConfig()),
            output_column_metadata=[],
        )
        offline_plugin_result = PusherPluginResult(name="offline", plugin="offline", value=(None, None))

        mock_inference.with_overrides.return_value = mock_inference
        mock_pusher_offline.with_overrides.return_value = mock_pusher_offline

        mock_document_loader.return_value = (loaded_documents, raw_vector_dataset)
        mock_inference.return_value = {"documents": embeddings}
        mock_pusher_offline.return_value = [offline_plugin_result]

        document_loader_config = DocumentLoaderConfig()
        inference_config = LLMInferenceConfig()
        hive_pusher_config = VectorDatasetHivePluginConfig()
        pusher_offline_config = PusherConfig(items=[PusherPluginConfig(name="default", vector_dataset_hive_plugin=hive_pusher_config)])

        task_configs = {
            "document_loader": TaskConfig(task_function="document_loader", config=document_loader_config),
            "inference": TaskConfig(task_function="inference", config=inference_config),
            "pusher_offline": TaskConfig(task_function="pusher_offline", config=pusher_offline_config),
        }

        raw_vector_dataset_artifact = vector_gen(WorkflowConfig(), task_configs)
        mock_document_loader.assert_called_once()
        mock_inference.assert_called_once()
        mock_pusher_offline.assert_called_once()
        mock_pusher_online.assert_not_called()
        mock_pusher.assert_not_called()
        self.assertEqual(raw_vector_dataset_artifact, {"default": raw_vector_dataset})
