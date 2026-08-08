"""Tests for ``...loader_utils.class_importer``."""

from __future__ import annotations

import ast
import os
import tempfile
from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.lib.model_manager._private.utils.loader_utils.class_importer import (
    create_alternative_defs,
    create_import_rewriter,
    import_model_class,
)

_MODULE = "michelangelo.lib.model_manager._private.utils.loader_utils.class_importer"


class ImportModelClassTest(TestCase):
    """Tests for ``import_model_class``."""

    def test_invalid_model_class_no_module(self):
        """Test error when model_class_str has no module part."""
        with self.assertRaisesRegex(ValueError, "Invalid model class definition"):
            import_model_class("/some/path", "MyModel")

    def test_invalid_model_class_empty(self):
        """Test error when model_class_str is just a dot."""
        with self.assertRaisesRegex(ValueError, "Invalid model class definition"):
            import_model_class("/some/path", ".")

    @patch("importlib.import_module")
    def test_direct_import_success(self, mock_import_module):
        """Test successful direct import on first try."""
        mock_module = MagicMock()
        mock_class = MagicMock()
        mock_module.MyModel = mock_class
        mock_import_module.return_value = mock_module

        result = import_model_class("/some/path", "mypackage.MyModel")

        self.assertEqual(result, mock_class)
        mock_import_module.assert_called_once_with("mypackage")

    @patch("importlib.import_module")
    def test_class_not_found_in_module(self, mock_import_module):
        """Test error when class doesn't exist in the imported module."""
        mock_module = MagicMock(spec=[])
        mock_import_module.return_value = mock_module

        with self.assertRaisesRegex(AttributeError, "Class MyModel not found"):
            import_model_class("/some/path", "mypackage.MyModel")

    @patch("importlib.import_module")
    def test_fallback_to_defs_path(self, mock_import_module):
        """Test fallback to appending defs_path when direct import fails."""
        mock_module = MagicMock()
        mock_class = MagicMock()
        mock_module.MyModel = mock_class

        # First call fails, second succeeds
        mock_import_module.side_effect = [ImportError("not found"), mock_module]

        with tempfile.TemporaryDirectory() as temp_dir:
            result = import_model_class(temp_dir, "mypackage.MyModel")

        self.assertEqual(result, mock_class)
        self.assertEqual(mock_import_module.call_count, 2)

    @patch(f"{_MODULE}.create_alternative_defs")
    @patch("importlib.import_module")
    def test_fallback_to_alternative_defs(self, mock_import_module, mock_create_alt):
        """Test fallback to create_alternative_defs when both direct imports fail."""
        mock_module = MagicMock()
        mock_class = MagicMock()
        mock_module.MyModel = mock_class
        mock_create_alt.return_value = ("/tmp/alt", "package_abc123")

        # First two calls fail, third succeeds
        mock_import_module.side_effect = [
            ImportError("not found"),
            ImportError("still not found"),
            mock_module,
        ]

        with tempfile.TemporaryDirectory() as temp_dir:
            result = import_model_class(temp_dir, "mypackage.MyModel")

        self.assertEqual(result, mock_class)
        self.assertEqual(mock_import_module.call_count, 3)
        mock_create_alt.assert_called_once_with(temp_dir)


class CreateAlternativeDefsTest(TestCase):
    """Tests for ``create_alternative_defs``."""

    def test_creates_proper_directory_structure(self):
        """Test create_alternative_defs creates proper directory structure."""
        with tempfile.TemporaryDirectory() as temp_dir:
            defs_dir = os.path.join(temp_dir, "defs")
            os.makedirs(defs_dir)

            with open(os.path.join(defs_dir, "sample.py"), "w") as f:
                f.write("import torch\nclass SampleModel:\n    pass\n")

            tmpdir, wrapper_name = create_alternative_defs(defs_dir)

            self.assertTrue(os.path.exists(tmpdir))
            self.assertTrue(wrapper_name.startswith("package_"))
            wrapper_dir = os.path.join(tmpdir, wrapper_name, "defs")
            self.assertTrue(os.path.exists(wrapper_dir))
            self.assertTrue(os.path.exists(os.path.join(wrapper_dir, "sample.py")))

    def test_rewrites_imports_in_files(self):
        """Test create_alternative_defs actually rewrites imports in copied files."""
        with tempfile.TemporaryDirectory() as temp_dir:
            defs_dir = os.path.join(temp_dir, "defs")
            os.makedirs(defs_dir)

            mymodule_dir = os.path.join(defs_dir, "mymodule")
            os.makedirs(mymodule_dir)

            with open(os.path.join(defs_dir, "sample.py"), "w") as f:
                f.write("import mymodule\nfrom mymodule import something\n")

            tmpdir, wrapper_name = create_alternative_defs(defs_dir)

            rewritten_file = os.path.join(tmpdir, wrapper_name, "defs", "sample.py")
            with open(rewritten_file) as f:
                content = f.read()

            self.assertIn(f"{wrapper_name}.defs.mymodule", content)


class CreateImportRewriterTest(TestCase):
    """Tests for ``create_import_rewriter``."""

    def test_handles_import_statement(self):
        """Test create_import_rewriter handles import statements."""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "mymodule"))

            rewriter = create_import_rewriter(temp_dir, "prefix.")

            code = "import mymodule"
            tree = ast.parse(code)
            new_tree = rewriter.visit(tree)
            modified_code = ast.unparse(new_tree)

            self.assertIn("prefix.mymodule", modified_code)

    def test_handles_from_import_statement(self):
        """Test create_import_rewriter handles from...import statements."""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "mymodule"))

            rewriter = create_import_rewriter(temp_dir, "prefix.")

            code = "from mymodule import something"
            tree = ast.parse(code)
            new_tree = rewriter.visit(tree)
            modified_code = ast.unparse(new_tree)

            self.assertIn("prefix.mymodule", modified_code)

    def test_preserves_non_matching_imports(self):
        """Test create_import_rewriter doesn't modify non-matching imports."""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "mymodule"))

            rewriter = create_import_rewriter(temp_dir, "prefix.")

            code = "import torch"
            tree = ast.parse(code)
            new_tree = rewriter.visit(tree)
            modified_code = ast.unparse(new_tree)

            self.assertEqual(modified_code.strip(), "import torch")
            self.assertNotIn("prefix.torch", modified_code)

    def test_handles_multiple_imports(self):
        """Test create_import_rewriter handles multiple imports in same statement."""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "module1"))
            os.makedirs(os.path.join(temp_dir, "module2"))

            rewriter = create_import_rewriter(temp_dir, "prefix.")

            code = "import module1, torch, module2"
            tree = ast.parse(code)
            new_tree = rewriter.visit(tree)
            modified_code = ast.unparse(new_tree)

            self.assertIn("prefix.module1", modified_code)
            self.assertIn("prefix.module2", modified_code)
            self.assertIn("torch", modified_code)
