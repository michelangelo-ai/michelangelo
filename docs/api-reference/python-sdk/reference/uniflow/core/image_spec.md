---
sidebar_label: image_spec
title: uniflow.core.image_spec
---

Container image specifications for Uniflow tasks.

This module provides the ImageSpec dataclass for defining custom container images
and build recipes for task execution environments. ImageSpec allows tasks to specify
their runtime environment independently from the default workflow container.

**Example**:

  Specifying a custom container image::
  
  from michelangelo.uniflow.core.image_spec import ImageSpec
  from michelangelo.uniflow.core.decorator import task
  
  @task(
  config=RayTask(head_cpu=4),
  image_spec=ImageSpec(
  container_image=&quot;docker.io/myorg/ml-tools:v1.2.3&quot;,
  recipe=&quot;bazel://path/to:build_target&quot;
  )
  )
  def train_model(data):
  # Runs in custom container with specific ML libraries
  pass

## ImageSpec Objects

```python
@dataclass
class ImageSpec()
```

ImageSpec defines container image specifications for uniflow tasks.

Example usage:
    @uniflow.task(
        config=RayTask(cpu=1),
        image_spec=ImageSpec(
            container_image=&quot;docker.io/library/examples:latest&quot;,
            recipe=&quot;bazel://uber/ai/michelangelo/sdk/workflow/tasks/llm_feature_prep:uniflow_default_task_image&quot;
        )
    )
    def my_task():
        pass

#### container\_image

The container image name/tag to use for task execution

#### recipe

Build recipe/target for reproducible image builds

