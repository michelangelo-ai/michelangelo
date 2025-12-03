from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TemplateRenderer,
)


class TritonTemplateRenderer(TemplateRenderer):
    """TritonTemplateRenderer handles Jinja2 template rendering for Triton models."""

    def __init__(self):
        """Initialize the TritonTemplateRenderer with the triton template path.

        Returns:
            None
        """
        super().__init__("triton")
