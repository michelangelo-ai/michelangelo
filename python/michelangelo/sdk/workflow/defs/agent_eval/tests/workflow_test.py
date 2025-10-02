# tests/test_agent_eval_workflow.py
from unittest import TestCase
from unittest.mock import patch

from uber.ai.michelangelo.sdk.workflow.defs.agent_eval import (
    workflow_function as agent_eval,
)
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import (
    TaskConfig,
    WorkflowConfig,
)
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.inference import (
    AgentInferenceConfig,
)
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.evaluator import (
    GenAIEvaluatorConfig,
)
from uber.ai.michelangelo.sdk.workflow.variables import DatasetVariable


class AgentEvalWorkflowTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.workflow.defs.agent_eval.workflow.document_loader")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.agent_eval.workflow.llm_inference")
    @patch("uber.ai.michelangelo.sdk.workflow.defs.agent_eval.workflow.genai_evaluator")
    def test_agent_eval(
        self,
        mock_genai_evaluator,
        mock_llm_inference,
        mock_document_loader,
    ):
        # Fake dataset produced by document_loader
        loaded_dataset = DatasetVariable.create(None)
        mock_document_loader.return_value = (loaded_dataset, None)

        # Fake inference result returned by llm_inference
        inference_results = {"llm_inference": DatasetVariable.create(None)}
        mock_llm_inference.return_value = inference_results

        # evaluator has no return
        mock_genai_evaluator.return_value = None

        task_configs = {
            "document_loader": TaskConfig(task_function="document_loader", config={}),
            "llm_inference": TaskConfig(
                task_function="llm_inference",
                config=AgentInferenceConfig(),
            ),
            "genai_evaluator": TaskConfig(
                task_function="genai_evaluator",
                config=GenAIEvaluatorConfig(),
            ),
        }

        agent_eval(WorkflowConfig(), task_configs)

        mock_document_loader.assert_called_once_with(config=task_configs["document_loader"])

        mock_llm_inference.assert_called_once_with(
            config=task_configs["llm_inference"],
            datasets={"llm_inference": loaded_dataset},
        )

        mock_genai_evaluator.assert_called_once_with(
            config=task_configs["genai_evaluator"],
            dataset=inference_results["llm_inference"],
        )
