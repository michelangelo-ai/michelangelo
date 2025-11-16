"""
GPT-OSS-20B Fine-tuning Example
Advanced fine-tuning demo for OpenAI's GPT-OSS-20B using Uniflow, PyTorch, and distributed training
"""

__version__ = "1.0.0"
__author__ = "Michelangelo Team"

# Import main workflow function
from .simple_workflow import simple_gpt_workflow

__all__ = [
    "simple_gpt_workflow"
]