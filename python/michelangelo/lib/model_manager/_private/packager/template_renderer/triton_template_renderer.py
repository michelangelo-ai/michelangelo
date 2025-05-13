from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TemplateRenderer


class TritonTemplateRenderer(TemplateRenderer):
    def __init__(self):
        super().__init__("triton")
