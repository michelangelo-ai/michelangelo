---
sidebar_label: utils
title: michelangelo.lib.trainer.torch.utils
---

Memory-footprint estimators for PyTorch / Transformers models.

#### get\_total\_training\_memory\_transformers

```python
def get_total_training_memory_transformers(model: PreTrainedModel,
                                           batch_size: int,
                                           sequence_length: int) -> float
```

Estimate the total training memory (in MB) for a Transformers model.

Uses the formula from the EleutherAI Transformer Math reference.

**Arguments**:

- `model` - A Hugging Face ``PreTrainedModel`` with ``config.hidden_size`` /
  ``num_hidden_layers`` / ``num_attention_heads`` and ``torch_dtype``.
- ``1 - Training batch size.
- ``2 - Input sequence length per sample.
  

**Returns**:

  Estimated total training memory in MB, including a 20% buffer for
  fragmentation overhead.
  
  Reference:
  https://blog.eleuther.ai/transformer-math/

#### estimate\_activation\_memory\_non\_transformer

```python
def estimate_activation_memory_non_transformer(layer_output_dims: dict,
                                               batch_size: int,
                                               bytes_per_value: int) -> float
```

Estimate activation memory (MB) given captured per-layer output shapes.

**Arguments**:

- `layer_output_dims` - Mapping of ``nn.Module`` -&gt; tensor ``shape`` captured
  via a forward hook.
- `batch_size` - Training batch size.
- `bytes_per_value` - Bytes per value in the activation tensor.
  

**Returns**:

  Total activation memory in MB.

#### get\_total\_training\_memory\_nn\_module

```python
def get_total_training_memory_nn_module(model: torch.nn.Module,
                                        batch_size: int,
                                        input_size: int) -> float
```

Estimate the total training memory (in MB) for a generic ``nn.Module``.

Registers forward hooks on ``Linear`` / ``Conv*`` / ``Norm*`` / ``RNN*``
layers to capture activation shapes, then sums parameter + gradient +
optimizer + activation memory.

**Arguments**:

- ``0 - The model to size.
- ``1 - Training batch size.
- ``2 - Flat input size used to generate a sample input tensor.
  

**Returns**:

  Estimated total training memory in MB, including a 20% buffer for
  fragmentation overhead.

