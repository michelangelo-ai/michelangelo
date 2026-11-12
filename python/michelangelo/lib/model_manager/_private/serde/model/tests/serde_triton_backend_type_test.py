"""Tests for reading the backend from a deployable package's config.pbtxt."""

import os
import tempfile
from unittest import TestCase

from michelangelo.lib.model_manager._private.serde.model import (
    get_deployable_backend_type,
)


class TritonBackendTypeTest(TestCase):
    """Parsing the ``backend`` line."""

    def setUp(self) -> None:
        """Create a scratch package directory."""
        self._tmp = tempfile.TemporaryDirectory()
        self.root = self._tmp.name

    def tearDown(self) -> None:
        """Remove the scratch directory."""
        self._tmp.cleanup()

    def backend_of(self, content: str) -> str:
        """Write ``content`` as config.pbtxt and read its backend."""
        with open(os.path.join(self.root, "config.pbtxt"), "w") as f:
            f.write(content)
        return get_deployable_backend_type(self.root)

    def test_each_supported_backend(self):
        """Every supported backend name is returned as written."""
        for backend in ("pytorch", "python", "onnxruntime"):
            self.assertEqual(self.backend_of(f'backend: "{backend}"\n'), backend)

    def test_whitespace_around_the_colon_and_before_the_key(self):
        """Spaces, tabs and indentation do not matter."""
        self.assertEqual(self.backend_of('backend  :   "python"'), "python")
        self.assertEqual(self.backend_of('backend\t:\t"python"'), "python")
        self.assertEqual(self.backend_of('  backend: "python"'), "python")

    def test_backend_among_other_fields(self):
        """The backend line is found in a full multiline config."""
        content = (
            'name: "m"\n'
            "max_batch_size: 8\n"
            'backend: "onnxruntime"\n'
            "input [\n"
            '  { name: "x", data_type: TYPE_FP32, dims: [ 4 ] }\n'
            "]\n"
        )
        self.assertEqual(self.backend_of(content), "onnxruntime")

    def test_backend_text_inside_another_field_is_ignored(self):
        """Only a line that starts with ``backend`` names the backend."""
        content = 'name: "m"\nplatform: "my_backend: \\"python\\""\n'
        with self.assertRaisesRegex(ValueError, "Could not find backend"):
            self.backend_of(content)

    def test_missing_backend_line(self):
        """A config without a backend is a ValueError."""
        with self.assertRaisesRegex(ValueError, "Could not find backend"):
            self.backend_of('name: "m"\nmax_batch_size: 8\n')

    def test_unquoted_backend_is_not_found(self):
        """A backend that is not a quoted string is not recognised."""
        with self.assertRaisesRegex(ValueError, "Could not find backend"):
            self.backend_of("backend: python\n")

    def test_unsupported_backend_lists_the_supported_ones(self):
        """The error names the bad backend and the supported set, sorted."""
        with self.assertRaisesRegex(
            ValueError,
            "Invalid backend type 'foo'.*onnxruntime, python, pytorch",
        ):
            self.backend_of('backend: "foo"\n')

    def test_missing_config(self):
        """A package without config.pbtxt is a FileNotFoundError."""
        with self.assertRaisesRegex(FileNotFoundError, "config.pbtxt not found"):
            get_deployable_backend_type(self.root)
