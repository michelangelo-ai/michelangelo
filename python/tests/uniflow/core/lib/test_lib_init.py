"""Tests for michelangelo.uniflow.core.lib package initialization."""

import subprocess
import sys
import textwrap
import unittest


class LibInitTest(unittest.TestCase):
    """Tests that importing one plugin does not load unrelated plugins."""

    def test_sibling_imports_do_not_load_deployment(self):
        """Lightweight plugins do not eagerly import deployment dependencies."""
        script = textwrap.dedent(
            """
            import sys

            import michelangelo.uniflow.core.lib.concurrent
            import michelangelo.uniflow.core.lib.time

            assert "michelangelo.uniflow.core.lib.deployment" not in sys.modules
            """
        )

        subprocess.run([sys.executable, "-c", script], check=True)
