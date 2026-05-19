# ruff: noqa: I001
import torch
import torch.nn as nn

from transformers import AutoModel


def get_total_training_memory_transformers(model: AutoModel, batch_size: int, sequence_length: int) -> float:
    """
    Get the total memory (in MB) required for training the model.
    This function is specific to transformers models.

    Reference: https://blog.eleuther.ai/transformer-math/
    """
    hidden_size = model.config.hidden_size  # Hidden size for activations
    num_layers = model.config.num_hidden_layers  # Number of layers in the model
    num_atten_heads = model.config.num_attention_heads  # Number of attention heads
    num_parameters = model.num_parameters()  # Total number of parameters
    dtype = model.config.torch_dtype  # Parameter type
    tensor_parallelism = 1  # Number of tensor parallelism

    bytes_per_parameter = torch.tensor([1]).to(dtype).element_size()  # Bytes per parameter

    # Calculating each memory component in MB
    # 1. Parameter Memory
    parameter_memory = (num_parameters * bytes_per_parameter) / (1024**2)

    # 2. Gradient Memory (same as Parameter Memory)
    gradient_memory = parameter_memory

    # 3. Optimizer Memory (assuming two states per parameter for AdamW optimizer)
    # Adam is magic, but it is highly memory inefficient.
    # In addition to requiring you to have a copy of the model parameters and the gradient parameters,
    # you also need to keep an additional three copies of the gradient parameters.
    optimizer_memory = 3 * parameter_memory

    # 4. Activations Memory
    # This is baseline formula for activations are stored in fp16.
    fp16_activation_memory_per_layer = (
        batch_size
        * sequence_length
        * hidden_size
        * (10 + 24 / tensor_parallelism + 5 * num_atten_heads * sequence_length / hidden_size / tensor_parallelism)
        / (1024**2)
    )

    # fp16 uses 2 bytes
    activation_memory_per_layer = bytes_per_parameter / 2 * fp16_activation_memory_per_layer
    activation_memory_total = activation_memory_per_layer * num_layers

    # Summing up and adding 20% for additional buffers and overheads for GPU memory fragmentation.
    total_memory = (parameter_memory + activation_memory_total + gradient_memory + optimizer_memory) * 1.2

    return total_memory


# Function to estimate activation memory on non-transformer layers
def estimate_activation_memory_non_transformer(layer_output_dims, batch_size, bytes_per_value):
    total_activation_memory_mb = 0

    for output_shape in layer_output_dims.values():
        # Calculate the number of elements in the activation for this layer
        num_elements = batch_size * output_shape[-1]
        # Calculate memory for this layer's activations in MB
        activation_memory_mb = (num_elements * bytes_per_value) / (1024**2)
        total_activation_memory_mb += activation_memory_mb

    return total_activation_memory_mb


def get_total_training_memory_nn_module(model: torch.nn.Module, batch_size: int, input_size: int) -> float:
    """
    Get the total memory (in MB) required for training the model.
    This function is specific to non-transformers models.
    """

    num_parameters = sum(p.numel() for p in model.parameters())

    dtype = None
    for param in model.parameters():
        dtype = param.dtype
        break

    bytes_per_parameter = torch.tensor([1]).to(dtype).element_size()  # Bytes per parameter

    # Calculating each memory component in MB
    # 1. Parameter Memory
    parameter_memory = (num_parameters * bytes_per_parameter) / (1024**2)

    # 2. Gradient Memory (same as Parameter Memory)
    gradient_memory = parameter_memory

    # 3. Optimizer Memory (assuming two states per parameter for AdamW optimizer)
    # Adam is magic, but it is highly memory inefficient.
    # In addition to requiring you to have a copy of the model parameters and the gradient parameters,
    # you also need to keep an additional three copies of the gradient parameters.
    optimizer_memory = 3 * parameter_memory

    layer_output_dims = {}

    # A hook function to capture the output dimensions of each layer
    def hook_fn(module, _input, output):
        layer_output_dims[module] = output.shape

    # Register hooks for each layer in the model
    # We only count Linear layers, Conv layers, Norm layers, and RNN layers
    hooks = []
    supported_layer_types = (nn.Linear, nn.modules.conv._ConvNd, nn.modules.batchnorm._NormBase, nn.modules.rnn.RNNBase)

    for layer in model.children():
        if isinstance(layer, supported_layer_types):
            hook = layer.register_forward_hook(hook_fn)
            hooks.append(hook)

    # Generate a sample input tensor
    inputs = torch.randn(batch_size, input_size)

    # Forward pass to capture output dimensions
    model(inputs)

    # Remove hooks
    for hook in hooks:
        hook.remove()

    # Use captured output dimensions to estimate activation memory
    total_activation_memory = estimate_activation_memory_non_transformer(layer_output_dims, batch_size, bytes_per_parameter)

    # Summing up and adding 20% for additional buffers and overheads for GPU memory fragmentation.
    total_memory_mb = (parameter_memory + total_activation_memory + gradient_memory + optimizer_memory) * 1.2

    return total_memory_mb
