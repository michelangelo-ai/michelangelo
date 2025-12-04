"""Tests for template renderer."""

from unittest import TestCase

from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TemplateRenderer,
)


class TemplateRendererTest(TestCase):
    """Tests template renderer output."""

    def test_render_template(self):
        """It renders the requested template without raising."""
        gen = TemplateRenderer("triton")
        result = gen.render("model.py.tmpl")
        self.assertIsNotNone(result)
