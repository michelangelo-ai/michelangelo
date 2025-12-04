"""Template renderer module."""

from typing import Optional

from jinja2 import (
    Environment,
    PackageLoader,
)

from michelangelo.lib.model_manager._private.packager.templates import (
    __package__ as template_package,
)


class TemplateRenderer:
    """TemplateRenderer is a wrapper around Jinja2 templating engine."""

    def __init__(self, template_path: str):
        """Initialize the TemplateRenderer with the given template path.

        Args:
            template_path (str): The path to the template directory.

        Returns:
            None
        """
        self.env = Environment(
            loader=PackageLoader(
                template_package,
                template_path,
            ),
        )
        self.env.keep_trailing_newline = True

    def render(self, template: str, options: Optional[dict] = None) -> str:
        """Render the given template with the given options.

        Args:
            template (str): The name of the template file.
            options (dict): The options to render the template with.

        Returns:
            str: The rendered template.
        """
        if options is None:
            options = {}

        tmpl = self.env.get_template(template)
        return tmpl.render(options)
