import tempfile
from unittest import TestCase
from michelangelo.lib.model_manager._private.packager.python_triton import generate_requirements_txt


class RequirementsTxtTest(TestCase):
    def test_generate_requirements_txt_with_list(self):
        requirements = ["numpy==1.18.5", "pandas==2.0.0", "scikit-learn"]
        expected_requirements_txt = "numpy==1.18.5\npandas==2.0.0\nscikit-learn"
        self.assertEqual(generate_requirements_txt(requirements), expected_requirements_txt)

    def test_generate_requirements_txt_with_file_path(self):
        with tempfile.NamedTemporaryFile(mode="w", delete=False) as f:
            f.write("numpy==1.18.5\npandas==2.0.0\nscikit-learn")
            f.flush()
            expected_requirements_txt = "numpy==1.18.5\npandas==2.0.0\nscikit-learn"
            self.assertEqual(generate_requirements_txt(f.name), expected_requirements_txt)

    def test_generate_requirements_txt_with_invalid_input(self):
        with self.assertRaises(ValueError):
            generate_requirements_txt(123)

        with self.assertRaises(ValueError):
            generate_requirements_txt(None)
