from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TemplateRenderer


class TemplateRendererTest(TestCase):
    def test_render_template(self):
        gen = TemplateRenderer("triton")
        result = gen.render("model.py.tmpl")
        self.assertIsNotNone(result)
