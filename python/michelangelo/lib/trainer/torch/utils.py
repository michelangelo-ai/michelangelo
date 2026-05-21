"""Memory-footprint estimators for PyTorch / Transformers models."""

from __future__ import annotations

from typing import TYPE_CHECKING

import torch
import torch.nn as nn

if TYPE_CHECKING:
    from transformers import PreTrainedModel


def get_total_training_memory_transformers(
    model: PreTrainedModel,
    batch_size: int,
    sequence_length: int,
) -> float:
    """Estimate the total training memory (in MB) for a Transformers model.

    Uses the formula from the EleutherAI Transformer Math reference.

    Args:
        model: A Hugging Face ``PreTrainedModel`` with ``config.hidden_size`` /
            ``num_hidden_layers`` / ``num_attention_heads`` and ``torch_dtype``.
        batch_size: Training batch size.
        sequence_length: Input sequence length per sample.

    Returns:
        Estimated total training memory in MB, including a 20% buffer for
        fragmentation overhead.

    Reference:
        https://blog.eleuther.ai/transformer-math/
    """
    hidden_size = model.config.hidden_size
    num_layers = model.config.num_hidden_layers
    num_atten_heads = model.config.num_attention_heads
    num_parameters = model.num_parameters()
    dtype = model.config.torch_dtype
    tensor_parallelism = 1

    bytes_per_parameter = torch.tensor([1]).to(dtype).element_size()

    parameter_memory = (num_parameters * bytes_per_parameter) / (1024**2)
    gradient_memory = parameter_memory
    # AdamW: 3 extra copies of the parameters (two optimizer states + gradient buffer).
    optimizer_memory = 3 * parameter_memory

    # Baseline fp16 activations formula.
    fp16_activation_memory_per_layer = (
        batch_size
        * sequence_length
        * hidden_size
        * (
            10
            + 24 / tensor_parallelism
            + 5 * num_atten_heads * sequence_length / hidden_size / tensor_parallelism
        )
        / (1024**2)
    )

    # fp16 uses 2 bytes.
    activation_memory_per_layer = (
        bytes_per_parameter / 2 * fp16_activation_memory_per_layer
    )
    activation_memory_total = activation_memory_per_layer * num_layers

    # Sum and add a 20% buffer for GPU memory fragmentation.
    total_memory = (
        parameter_memory + activation_memory_total + gradient_memory + optimizer_memory
    ) * 1.2
    return total_memory


def estimate_activation_memory_non_transformer(
    layer_output_dims: dict,
    batch_size: int,
    bytes_per_value: int,
) -> float:
    """Estimate activation memory (MB) given captured per-layer output shapes.

    Args:
        layer_output_dims: Mapping of ``nn.Module`` -> tensor ``shape`` captured
            via a forward hook.
        batch_size: Training batch size.
        bytes_per_value: Bytes per value in the activation tensor.

    Returns:
        Total activation memory in MB.
    """
    total_activation_memory_mb = 0
    for output_shape in layer_output_dims.values():
        num_elements = batch_size * output_shape[-1]
        activation_memory_mb = (num_elements * bytes_per_value) / (1024**2)
        total_activation_memory_mb += activation_memory_mb
    return total_activation_memory_mb


def get_total_training_memory_nn_module(
    model: torch.nn.Module,
    batch_size: int,
    input_size: int,
) -> float:
    """Estimate the total training memory (in MB) for a generic ``nn.Module``.

    Registers forward hooks on ``Linear`` / ``Conv*`` / ``Norm*`` / ``RNN*``
    layers to capture activation shapes, then sums parameter + gradient +
    optimizer + activation memory.

    Args:
        model: The model to size.
        batch_size: Training batch size.
        input_size: Flat input size used to generate a sample input tensor.

    Returns:
        Estimated total training memory in MB, including a 20% buffer for
        fragmentation overhead.
    """
    num_parameters = sum(p.numel() for p in model.parameters())

    dtype = None
    for param in model.parameters():
        dtype = param.dtype
        break

    bytes_per_parameter = torch.tensor([1]).to(dtype).element_size()

    parameter_memory = (num_parameters * bytes_per_parameter) / (1024**2)
    gradient_memory = parameter_memory
    # AdamW: 3 extra copies of the parameters (two optimizer states + gradient buffer).
    optimizer_memory = 3 * parameter_memory

    layer_output_dims = {}

    def hook_fn(module, _input, output):
        layer_output_dims[module] = output.shape

    # We only count Linear layers, Conv layers, Norm layers, and RNN layers.
    hooks = []
    supported_layer_types = (
        nn.Linear,
        nn.modules.conv._ConvNd,
        nn.modules.batchnorm._NormBase,
        nn.modules.rnn.RNNBase,
    )

    for layer in model.children():
        if isinstance(layer, supported_layer_types):
            hook = layer.register_forward_hook(hook_fn)
            hooks.append(hook)

    inputs = torch.randn(batch_size, input_size)
    model(inputs)

    for hook in hooks:
        hook.remove()

    total_activation_memory = estimate_activation_memory_non_transformer(
        layer_output_dims, batch_size, bytes_per_parameter
    )

    # Sum and add a 20% buffer for GPU memory fragmentation.
    total_memory_mb = (
        parameter_memory + total_activation_memory + gradient_memory + optimizer_memory
    ) * 1.2
    return total_memory_mb
