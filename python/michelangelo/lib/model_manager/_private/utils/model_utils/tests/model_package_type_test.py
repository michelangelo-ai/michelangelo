from unittest import TestCase
import os
import tempfile
import yaml
from michelangelo.lib.model_manager.constants import PackageType
from michelangelo.lib.model_manager._private.utils.model_utils import (
    infer_model_package_type,
    infer_raw_model_package_type,
)


class ModelPackageTypeTest(TestCase):
    def test_infer_model_package_type_spark(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            spark_pkg = os.path.join(temp_dir, "spark")

            os.makedirs(os.path.join(spark_pkg, "deploy_jar"))
            with open(os.path.join(spark_pkg, "deploy_jar", "model.jar.gz"), "wb") as f:
                f.write(b"content")

            pkg_type = infer_model_package_type(spark_pkg)
            self.assertEqual(pkg_type, PackageType.SPARK)

    def test_infer_model_package_type_triton(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            triton_pkg = os.path.join(temp_dir, "triton")
            model_1 = os.path.join(triton_pkg, "model_1")
            model_2 = os.path.join(triton_pkg, "model_2")
            os.makedirs(model_1)
            os.makedirs(model_2)

            with open(os.path.join(model_1, "config.pbtxt"), "w") as f:
                f.write("content")

            with open(os.path.join(model_2, "config.pbtxt"), "w") as f:
                f.write("content")

            pkg_type = infer_model_package_type(triton_pkg)
            self.assertEqual(pkg_type, PackageType.TRITON)

    def test_infer_model_package_type_raw(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            raw_pkg = os.path.join(temp_dir, "raw")
            os.makedirs(raw_pkg)
            raw_file = os.path.join(temp_dir, "raw.txt")
            with open(raw_file, "w") as f:
                f.write("content")

            pkg_type = infer_model_package_type(raw_pkg)
            self.assertEqual(pkg_type, PackageType.RAW)

            pkg_type = infer_model_package_type(raw_file)
            self.assertEqual(pkg_type, PackageType.RAW)

    def test_infer_model_package_type_file_not_found(self):
        with self.assertRaises(FileNotFoundError):
            infer_model_package_type("__the_most_unique_random_test_file__")

    def test_infer_raw_model_package_type(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "metadata"))
            with open(os.path.join(temp_dir, "metadata", "type.yaml"), "w") as f:
                yaml.dump({"type": "custom-python"}, f)
            model_type = infer_raw_model_package_type(temp_dir)
            self.assertEqual(model_type, "custom-python")

    def test_infer_raw_model_package_type_not_dir(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            with open(os.path.join(temp_dir, "type.yaml"), "w") as f:
                yaml.dump({"type": "custom-python"}, f)
            model_type = infer_raw_model_package_type(os.path.join(temp_dir, "type.yaml"))
            self.assertIsNone(model_type)

    def test_infer_raw_model_package_type_no_type_yaml(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "metadata"))
            model_type = infer_raw_model_package_type(temp_dir)
            self.assertIsNone(model_type)

    def test_infer_raw_model_package_type_file_not_found(self):
        self.assertIsNone(infer_raw_model_package_type("__the_most_unique_random_test_file__"))
