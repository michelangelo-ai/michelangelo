from dataclasses import dataclass
from typing import Optional


@dataclass
class ImageSpec:
    """
    ImageSpec defines container image specifications for uniflow tasks.

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
