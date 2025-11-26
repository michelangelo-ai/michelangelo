"""GPT-OSS-20B Fine-tuning Example.

Advanced fine-tuning demo for OpenAI's GPT-OSS-20B using Uniflow, PyTorch,
and distributed training.
"""

__version__ = "1.0.0"
__author__ = "Michelangelo Team"

# Import main workflow function
from .eval import evaluate_gpt_model
from .simple_workflow import simple_gpt_workflow

__all__ = ["evaluate_gpt_model", "simple_gpt_workflow"]
