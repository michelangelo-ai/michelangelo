"""Container image specifications for Uniflow tasks.

This module provides the ImageSpec dataclass for defining custom container images
and build recipes for task execution environments. ImageSpec allows tasks to specify
their runtime environment independently from the default workflow container.

Example:
    Specifying a custom container image::

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
"""

from dataclasses import dataclass
from typing import Optional


@dataclass
class ImageSpec:
    """ImageSpec defines container image specifications for uniflow tasks.

    Example usage:
        @uniflow.task(
            config=RayTask(cpu=1),
            image_spec=ImageSpec(
                container_image="docker.io/library/examples:latest",
                recipe="bazel://uber/ai/michelangelo/sdk/workflow/tasks/llm_feature_prep:uniflow_default_task_image"
            )
        )
        def my_task():
            pass
    """

    container_image: Optional[str] = None
    """The container image name/tag to use for task execution"""

    recipe: Optional[str] = None
    """Build recipe/target for reproducible image builds"""
