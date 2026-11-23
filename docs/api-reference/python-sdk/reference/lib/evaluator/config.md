---
sidebar_label: config
title: lib.evaluator.config
---

Load an `EvaluatorConfig` from a plain dict or a YAML file.

Pipelines carry evaluator configuration as a proto ``Struct``, which arrives
here as ordinary Python dicts. These loaders turn that into the typed schema
objects the rest of the evaluator works with.

#### load\_config\_from\_dict

```python
def load_config_from_dict(config_dict: dict[str, Any]) -> EvaluatorConfig
```

Build an `EvaluatorConfig` from a nested dict.

**Arguments**:

- `config_dict` - Mapping with a ``metrics`` list. Each entry needs ``name``
  and ``metric``; ``params``, ``columns``, and ``filter_expr`` are
  optional.
  

**Returns**:

  The parsed configuration.

#### load\_config\_from\_yaml

```python
def load_config_from_yaml(yaml_path: str) -> EvaluatorConfig
```

Build an `EvaluatorConfig` from a YAML file.

**Arguments**:

- `yaml_path` - Path to a YAML document with the same shape
  `load_config_from_dict` expects.
  

**Returns**:

  The parsed configuration.

