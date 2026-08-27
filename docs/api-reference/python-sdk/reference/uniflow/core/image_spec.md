---
sidebar_label: image_spec
title: michelangelo.uniflow.core.image_spec
---

Container image specifications for Uniflow tasks.

This module provides the ImageSpec dataclass for defining custom container images
and build recipes for task execution environments. ImageSpec allows tasks to specify
their runtime environment independently from the default workflow container.

**Example**:

Specifying a custom container image:

```python
from michelangelo.uniflow.core.image_spec import ImageSpec
from michelangelo.uniflow.core.decorator import task

@task(
    config=RayTask(head_cpu=4),
    image_spec=ImageSpec(
        container_image="docker.io/myorg/ml-tools:v1.2.3",
        recipe="bazel://path/to:build_target"
    )
)
def train_model(data):
    # Runs in custom container with specific ML libraries
    pass
```

## ImageSpec Objects

```python
@dataclass
class ImageSpec()
```

ImageSpec defines container image specifications for uniflow tasks.

Example usage:

```python
@uniflow.task(
    config=RayTask(head_cpu=1),
    image_spec=ImageSpec(
        container_image="docker.io/library/examples:latest",
        recipe="bazel://path/to:build_target"
    )
)
def my_task():
    pass
```

#### container\_image

The container image name/tag to use for task execution

#### recipe

Build recipe/target for reproducible image builds
