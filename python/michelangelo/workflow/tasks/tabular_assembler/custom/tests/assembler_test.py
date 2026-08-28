"""Unit tests for ``...tabular_assembler.custom.assembler``."""

from __future__ import annotations

import os
import pickle
import tempfile
import unittest
from io import BytesIO
from typing import TYPE_CHECKING
from unittest.mock import patch

import numpy as np

from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.interface.custom_model import Model
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.schema.feature_schema import FeatureSchema
from michelangelo.lib.model_manager.schema.feature_schema_item import FeatureSchemaItem
from michelangelo.lib.shared.utils.model_fuser import fuse_model_schema
from michelangelo.workflow.schema.assembler import (
    CustomAssemblerConfig,
    TabularAssemblerConfig,
)
from michelangelo.workflow.tasks.tabular_assembler.custom.assembler import (
    custom_assembler,
)
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    FeaturePackageMetadata,
    ModelMetadata,
)
from michelangelo.workflow.variables.types import FeaturePackageArtifact, ModelArtifact

if TYPE_CHECKING:
    from numpy import ndarray

_ASSEMBLER_MODULE = "michelangelo.workflow.tasks.tabular_assembler.custom.assembler"


class _CustomModelFixture(Model):
    """Minimal concrete ``Model`` used only as an importable dotted path."""

    def save(self, path: str) -> None:
        pass

    @classmethod
    def load(cls, path: str) -> _CustomModelFixture:
        return cls()

    def predict(self, inputs: dict[str, ndarray]) -> dict[str, ndarray]:
        return inputs


CUSTOM_MODEL_CLASS_PATH = (
    "michelangelo.workflow.tasks.tabular_assembler.custom.tests."
    "assembler_test._CustomModelFixture"
)


def _make_schema() -> ModelSchema:
    return ModelSchema(
        input_schema=[
            ModelSchemaItem(name="input", data_type=DataType.FLOAT, shape=[2, 2]),
            ModelSchemaItem(name="label", data_type=DataType.STRING, shape=[1]),
        ],
        output_schema=[
            ModelSchemaItem(name="output", data_type=DataType.FLOAT, shape=[1])
        ],
    )


def _fake_create_package(dest_dir_name: str):
    """Return a packager-method side effect that materializes a real package dir.

    ``storage_backend.upload`` (unmocked, real ``LocalStorageBackend``) needs
    an actual file on disk to copy, so the packager stand-in must write
    something to ``dest_model_path`` rather than returning a bare string.
    """

    def _side_effect(model_path, *, dest_model_path=None, **kwargs):
        os.makedirs(dest_model_path, exist_ok=True)
        with open(os.path.join(dest_model_path, "artifact.bin"), "wb") as f:
            f.write(dest_dir_name.encode())
        return dest_model_path

    return _side_effect


class CustomAssemblerTest(unittest.TestCase):
    """Tests for ``custom_assembler``."""

    def setUp(self) -> None:
        """Create a fresh ``LocalStorageBackend`` rooted at a temp dir per test."""
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.storage_backend = LocalStorageBackend(self._tmp.name)

    def _upload_raw_model_source(self, contents: bytes = b"weights") -> str:
        """Create a local source dir and upload it, returning a backend URI."""
        src_dir = tempfile.mkdtemp(dir=self._tmp.name)
        with open(os.path.join(src_dir, "model.bin"), "wb") as f:
            f.write(contents)
        return self.storage_backend.upload(
            src_dir, f"sources/{os.path.basename(src_dir)}"
        )

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_custom_assembler_basic(self, mock_create_raw, mock_create_model):
        """Deployable and raw metadata, and upload, all reflect the source model."""
        mock_create_model.side_effect = _fake_create_package("deployable")
        mock_create_raw.side_effect = _fake_create_package("raw")

        config = TabularAssemblerConfig()
        raw_model = ModelArtifact(
            path=self._upload_raw_model_source(),
            metadata=ModelMetadata(
                model_class=CUSTOM_MODEL_CLASS_PATH,
                _schema=BytesIO(pickle.dumps(_make_schema())),
                _sample_data=BytesIO(
                    pickle.dumps(
                        [
                            {
                                "input": np.array([[1.0, 2.0], [3.0, 4.0]]),
                                "label": np.array([b"a"]),
                            }
                        ]
                    )
                ),
                is_incremental_training=True,
                baseline_model_identifier="baseline-model-v1",
            ),
        )

        assembled = custom_assembler(
            config, raw_model, storage_backend=self.storage_backend
        )

        self.assertEqual(assembled.deployable_model.metadata.deployable, True)
        self.assertEqual(assembled.deployable_model.metadata.assembled, True)
        self.assertEqual(
            assembled.deployable_model.metadata.schema, raw_model.metadata.schema
        )
        self.assertEqual(
            pickle.loads(assembled.deployable_model.metadata._schema.getvalue()),
            raw_model.metadata.schema,
        )
        unpickled_sample_data = pickle.loads(
            assembled.deployable_model.metadata._sample_data.getvalue()
        )
        np.testing.assert_array_equal(
            unpickled_sample_data[0]["input"],
            raw_model.metadata.sample_data[0]["input"],
        )

        self.assertEqual(assembled.raw_model.metadata.deployable, False)
        self.assertEqual(assembled.raw_model.metadata.assembled, True)
        self.assertEqual(assembled.raw_model.metadata.schema, raw_model.metadata.schema)
        self.assertEqual(
            pickle.loads(assembled.raw_model.metadata._schema.getvalue()),
            raw_model.metadata.schema,
        )
        unpickled_raw_sample_data = pickle.loads(
            assembled.raw_model.metadata._sample_data.getvalue()
        )
        np.testing.assert_array_equal(
            unpickled_raw_sample_data[0]["input"],
            raw_model.metadata.sample_data[0]["input"],
        )
        self.assertEqual(assembled.raw_model.metadata.is_incremental_training, True)
        self.assertEqual(
            assembled.raw_model.metadata.baseline_model_identifier, "baseline-model-v1"
        )
        self.assertEqual(
            assembled.raw_model.metadata.training_framework, TRAINING_FRAMEWORK_CUSTOM
        )
        self.assertEqual(
            assembled.raw_model.metadata.model_class, CUSTOM_MODEL_CLASS_PATH
        )

        # Both packaged artifacts were actually uploaded through the backend.
        self.assertTrue(os.path.exists(assembled.deployable_model.path))
        self.assertTrue(os.path.exists(assembled.raw_model.path))

        # Default: both deployable and raw are uploaded as loose files.
        self.assertTrue(os.path.isdir(assembled.deployable_model.path))
        self.assertTrue(os.path.isdir(assembled.raw_model.path))

        pkg_kwargs = mock_create_model.call_args.kwargs
        self.assertTrue(
            np.array_equal(
                pkg_kwargs["sample_data"][0]["input"],
                np.array([[1.0, 2.0], [3.0, 4.0]]),
            )
        )
        self.assertEqual(pkg_kwargs["model_path_source_type"], StorageType.LOCAL)
        self.assertIsNone(pkg_kwargs["additional_import_prefixes"])
        self.assertIsNone(pkg_kwargs["include_import_prefixes"])

        raw_kwargs = mock_create_raw.call_args.kwargs
        self.assertEqual(raw_kwargs["model_path_source_type"], StorageType.LOCAL)
        self.assertIsNone(raw_kwargs["additional_import_prefixes"])
        self.assertIsNone(raw_kwargs["include_import_prefixes"])

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_tar_deployable_package_uploads_single_tar(
        self, mock_create_raw, mock_create_model
    ):
        """tar_deployable_package=True uploads one tar, not loose files."""
        mock_create_model.side_effect = _fake_create_package("deployable")
        mock_create_raw.side_effect = _fake_create_package("raw")

        config = TabularAssemblerConfig(
            custom=CustomAssemblerConfig(tar_deployable_package=True)
        )
        raw_model = ModelArtifact(
            path=self._upload_raw_model_source(),
            metadata=ModelMetadata(
                model_class=CUSTOM_MODEL_CLASS_PATH,
                _schema=BytesIO(pickle.dumps(_make_schema())),
                _sample_data=BytesIO(pickle.dumps([{"input": np.array([[1.0, 2.0]])}])),
            ),
        )

        assembled = custom_assembler(
            config, raw_model, storage_backend=self.storage_backend
        )

        self.assertTrue(os.path.isfile(assembled.deployable_model.path))
        self.assertTrue(assembled.deployable_model.path.endswith("model.tar"))
        self.assertTrue(os.path.isdir(assembled.raw_model.path))

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_downloads_raw_model_locally_before_packaging(
        self, mock_create_raw, mock_create_model
    ):
        """The packager only understands local paths, so the source must be local.

        The download is materialized under the assembler's own
        ``TemporaryDirectory``, which is gone by the time ``custom_assembler``
        returns — so the on-disk state must be observed from inside the
        packager side effect, not after the call.
        """
        observed: dict[str, object] = {}

        def _observe_and_package(model_path, *, dest_model_path=None, **kwargs):
            observed["is_dir"] = os.path.isdir(model_path)
            with open(os.path.join(model_path, "model.bin"), "rb") as f:
                observed["contents"] = f.read()
            observed["model_path_source_type"] = kwargs["model_path_source_type"]
            os.makedirs(dest_model_path, exist_ok=True)
            return dest_model_path

        mock_create_model.side_effect = _observe_and_package
        mock_create_raw.side_effect = _fake_create_package("raw")

        config = TabularAssemblerConfig()
        raw_model = ModelArtifact(
            path=self._upload_raw_model_source(contents=b"weights-xyz"),
            metadata=ModelMetadata(
                model_class=CUSTOM_MODEL_CLASS_PATH,
                _schema=BytesIO(pickle.dumps(_make_schema())),
                _sample_data=BytesIO(pickle.dumps([{}])),
            ),
        )

        custom_assembler(config, raw_model, storage_backend=self.storage_backend)

        self.assertTrue(observed["is_dir"])
        self.assertEqual(observed["contents"], b"weights-xyz")
        self.assertEqual(observed["model_path_source_type"], StorageType.LOCAL)

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_custom_batch_processing_threaded_to_packager(
        self, mock_create_raw, mock_create_model
    ):
        """``custom_batch_processing`` reaches the packager constructor."""
        mock_create_model.side_effect = _fake_create_package("deployable")
        mock_create_raw.side_effect = _fake_create_package("raw")

        config = TabularAssemblerConfig(
            custom=CustomAssemblerConfig(custom_batch_processing=True)
        )
        raw_model = ModelArtifact(
            path=self._upload_raw_model_source(),
            metadata=ModelMetadata(
                model_class=CUSTOM_MODEL_CLASS_PATH,
                _schema=BytesIO(pickle.dumps(_make_schema())),
                _sample_data=BytesIO(pickle.dumps([{}])),
            ),
        )

        with patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager") as mock_packager_cls:
            mock_packager_cls.return_value.create_model_package.side_effect = (
                _fake_create_package("deployable")
            )
            mock_packager_cls.return_value.create_raw_model_package.side_effect = (
                _fake_create_package("raw")
            )
            custom_assembler(config, raw_model, storage_backend=self.storage_backend)
            mock_packager_cls.assert_called_once_with(custom_batch_processing=True)

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_passes_additional_import_prefixes(
        self, mock_create_raw, mock_create_model
    ):
        """``additional_import_prefixes`` reach both packager calls, empty or not."""
        for prefixes in (["mypkg.dynamic"], []):
            with self.subTest(prefixes=prefixes):
                mock_create_model.side_effect = _fake_create_package("deployable")
                mock_create_raw.side_effect = _fake_create_package("raw")

                config = TabularAssemblerConfig(
                    custom=CustomAssemblerConfig(additional_import_prefixes=prefixes)
                )
                raw_model = ModelArtifact(
                    path=self._upload_raw_model_source(),
                    metadata=ModelMetadata(
                        model_class=CUSTOM_MODEL_CLASS_PATH,
                        _schema=BytesIO(pickle.dumps(_make_schema())),
                        _sample_data=BytesIO(pickle.dumps([{}])),
                    ),
                )

                custom_assembler(
                    config, raw_model, storage_backend=self.storage_backend
                )

                self.assertEqual(
                    mock_create_model.call_args.kwargs["additional_import_prefixes"],
                    prefixes,
                )
                self.assertEqual(
                    mock_create_raw.call_args.kwargs["additional_import_prefixes"],
                    prefixes,
                )

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_passes_include_import_prefixes(self, mock_create_raw, mock_create_model):
        """``include_import_prefixes`` reach both packager calls, empty or not."""
        for prefixes in (["mypkg.models"], []):
            with self.subTest(prefixes=prefixes):
                mock_create_model.side_effect = _fake_create_package("deployable")
                mock_create_raw.side_effect = _fake_create_package("raw")

                config = TabularAssemblerConfig(
                    custom=CustomAssemblerConfig(include_import_prefixes=prefixes)
                )
                raw_model = ModelArtifact(
                    path=self._upload_raw_model_source(),
                    metadata=ModelMetadata(
                        model_class=CUSTOM_MODEL_CLASS_PATH,
                        _schema=BytesIO(pickle.dumps(_make_schema())),
                        _sample_data=BytesIO(pickle.dumps([{}])),
                    ),
                )

                custom_assembler(
                    config, raw_model, storage_backend=self.storage_backend
                )

                self.assertEqual(
                    mock_create_model.call_args.kwargs["include_import_prefixes"],
                    prefixes,
                )
                self.assertEqual(
                    mock_create_raw.call_args.kwargs["include_import_prefixes"],
                    prefixes,
                )

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_native_transform_combined_layout(self, mock_create_raw, mock_create_model):
        """Native-transform models fuse their schema/sample_data into one package.

        The predictor and native-transform sources are downloaded through the
        injected ``storage_backend`` (not bypassed via a raw fsspec helper),
        so both are uploaded through a real ``LocalStorageBackend`` here and
        their on-disk presence under ``combined_model/`` is observed from
        inside the packager side effect, before the assembler's temporary
        directory is cleaned up.
        """
        observed: dict[str, object] = {}

        def _observe_and_package(model_path, *, dest_model_path=None, **kwargs):
            observed["predictor_file"] = os.path.exists(
                os.path.join(model_path, "predictor", "predictor.bin")
            )
            observed["native_transform_file"] = os.path.exists(
                os.path.join(model_path, "native_transform", "tx.bin")
            )
            os.makedirs(dest_model_path, exist_ok=True)
            return dest_model_path

        mock_create_model.side_effect = _observe_and_package
        mock_create_raw.side_effect = _fake_create_package("raw")

        tx_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="a", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[
                ModelSchemaItem(name="a_tx", data_type=DataType.FLOAT, shape=[1])
            ],
        )
        pred_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="a", data_type=DataType.FLOAT, shape=[1]),
                ModelSchemaItem(name="a_tx", data_type=DataType.FLOAT, shape=[1]),
            ],
            output_schema=[
                ModelSchemaItem(name="out", data_type=DataType.FLOAT, shape=[1])
            ],
        )
        tx_src_dir = tempfile.mkdtemp(dir=self._tmp.name)
        with open(os.path.join(tx_src_dir, "tx.bin"), "wb") as f:
            f.write(b"tx-weights")
        native_tx = ModelArtifact(
            path=self.storage_backend.upload(
                tx_src_dir, f"sources/{os.path.basename(tx_src_dir)}"
            ),
            metadata=ModelMetadata(
                _schema=BytesIO(pickle.dumps(tx_schema)),
                _sample_data=BytesIO(
                    pickle.dumps([{"a": np.array([0.0], dtype=np.float32)}])
                ),
            ),
        )
        pred_src_dir = tempfile.mkdtemp(dir=self._tmp.name)
        with open(os.path.join(pred_src_dir, "predictor.bin"), "wb") as f:
            f.write(b"predictor-weights")
        config = TabularAssemblerConfig()
        raw_model = ModelArtifact(
            path=self.storage_backend.upload(
                pred_src_dir, f"sources/{os.path.basename(pred_src_dir)}"
            ),
            metadata=ModelMetadata(
                model_class=CUSTOM_MODEL_CLASS_PATH,
                _schema=BytesIO(pickle.dumps(pred_schema)),
                _sample_data=BytesIO(pickle.dumps([{}])),
            ),
        )

        custom_assembler(
            config,
            raw_model,
            native_transform_model=native_tx,
            storage_backend=self.storage_backend,
        )

        self.assertTrue(observed["predictor_file"])
        self.assertTrue(observed["native_transform_file"])
        pkg_kw = mock_create_model.call_args.kwargs
        raw_kw = mock_create_raw.call_args.kwargs
        model_path = mock_create_model.call_args.args[0]
        self.assertTrue(model_path.replace("\\", "/").endswith("combined_model"))
        self.assertEqual(model_path, mock_create_raw.call_args.args[0])
        self.assertEqual(pkg_kw["model_path_source_type"], StorageType.LOCAL)
        self.assertEqual(raw_kw["model_path_source_type"], StorageType.LOCAL)
        fused = fuse_model_schema(tx_schema, pred_schema)
        self.assertEqual(pkg_kw["model_schema"].input_schema, fused.input_schema)
        self.assertEqual(pkg_kw["model_schema"].output_schema, fused.output_schema)
        self.assertEqual(raw_kw["sample_data"][0]["a"].tolist(), [0.0])

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_prefers_config_model_class_when_importable(
        self, mock_create_raw, mock_create_model
    ):
        """An importable ``config.model_class`` overrides the metadata class."""
        mock_create_model.side_effect = _fake_create_package("deployable")
        mock_create_raw.side_effect = _fake_create_package("raw")

        config = TabularAssemblerConfig(model_class=CUSTOM_MODEL_CLASS_PATH)
        raw_model = ModelArtifact(
            path=self._upload_raw_model_source(),
            metadata=ModelMetadata(
                model_class="metadata.should.NotBeUsed",
                _schema=BytesIO(pickle.dumps(_make_schema())),
                _sample_data=BytesIO(pickle.dumps([{}])),
            ),
        )

        custom_assembler(config, raw_model, storage_backend=self.storage_backend)

        self.assertEqual(
            mock_create_model.call_args.kwargs["model_class"], CUSTOM_MODEL_CLASS_PATH
        )

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_falls_back_to_metadata_model_class_when_config_invalid(
        self, mock_create_raw, mock_create_model
    ):
        """An unimportable ``config.model_class`` falls back to the metadata class."""
        mock_create_model.side_effect = _fake_create_package("deployable")
        mock_create_raw.side_effect = _fake_create_package("raw")

        config = TabularAssemblerConfig(model_class="nonexistent_xyz.module.Class")
        raw_model = ModelArtifact(
            path=self._upload_raw_model_source(),
            metadata=ModelMetadata(
                model_class=CUSTOM_MODEL_CLASS_PATH,
                _schema=BytesIO(pickle.dumps(_make_schema())),
                _sample_data=BytesIO(pickle.dumps([{}])),
            ),
        )

        custom_assembler(config, raw_model, storage_backend=self.storage_backend)

        self.assertEqual(
            mock_create_model.call_args.kwargs["model_class"], CUSTOM_MODEL_CLASS_PATH
        )


class FeaturePackageFusionTest(unittest.TestCase):
    """Tests for ``custom_assembler``'s optional ``feature_package`` fusion."""

    def setUp(self) -> None:
        """Create a fresh ``LocalStorageBackend`` rooted at a temp dir per test."""
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.storage_backend = LocalStorageBackend(self._tmp.name)

    def _upload_raw_model_source(self, contents: bytes = b"weights") -> str:
        """Create a local source dir and upload it, returning a backend URI."""
        src_dir = tempfile.mkdtemp(dir=self._tmp.name)
        with open(os.path.join(src_dir, "model.bin"), "wb") as f:
            f.write(contents)
        return self.storage_backend.upload(
            src_dir, f"sources/{os.path.basename(src_dir)}"
        )

    def _make_raw_model(self):
        return ModelArtifact(
            path=self._upload_raw_model_source(),
            metadata=ModelMetadata(
                model_class=CUSTOM_MODEL_CLASS_PATH,
                _schema=BytesIO(pickle.dumps(_make_schema())),
                _sample_data=BytesIO(
                    pickle.dumps(
                        [
                            {
                                "input": np.array([[1.0, 2.0], [3.0, 4.0]]),
                                "label": np.array([b"a"]),
                            }
                        ]
                    )
                ),
            ),
        )

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_none_feature_package_leaves_e2e_schema_matching_model_schema(
        self, mock_create_raw, mock_create_model
    ):
        """No ``feature_package`` -> e2e_schema falls back to the model's own schema."""
        mock_create_model.side_effect = _fake_create_package("deployable")
        mock_create_raw.side_effect = _fake_create_package("raw")
        raw_model = self._make_raw_model()

        assembled = custom_assembler(
            TabularAssemblerConfig(), raw_model, storage_backend=self.storage_backend
        )

        self.assertEqual(
            assembled.deployable_model.metadata.e2e_schema, raw_model.metadata.schema
        )
        self.assertIsNone(assembled.feature_package)

    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_model_package")
    @patch(f"{_ASSEMBLER_MODULE}.CustomTritonPackager.create_raw_model_package")
    def test_feature_package_is_fused_into_deployable_e2e_schema(
        self, mock_create_raw, mock_create_model
    ):
        """A supplied ``feature_package`` is fused into the deployable's e2e fields."""
        mock_create_model.side_effect = _fake_create_package("deployable")
        mock_create_raw.side_effect = _fake_create_package("raw")
        raw_model = self._make_raw_model()
        feature_package = FeaturePackageArtifact(
            path="features",
            metadata=FeaturePackageMetadata(
                _schema=BytesIO(
                    pickle.dumps(
                        FeatureSchema(
                            input_schema=[
                                FeatureSchemaItem(
                                    name="raw_feature",
                                    data_type=DataType.FLOAT,
                                    shape=[1],
                                ),
                            ],
                        )
                    )
                ),
                _sample_data=BytesIO(pickle.dumps([{"raw_feature": 1.0}])),
            ),
        )

        assembled = custom_assembler(
            TabularAssemblerConfig(),
            raw_model,
            feature_package=feature_package,
            storage_backend=self.storage_backend,
        )

        e2e_schema = assembled.deployable_model.metadata.e2e_schema
        self.assertEqual(
            sorted(item.name for item in e2e_schema.input_schema),
            ["input", "label", "raw_feature"],
        )
        e2e_sample_data = assembled.deployable_model.metadata.e2e_sample_data
        self.assertEqual(e2e_sample_data[0]["raw_feature"], 1.0)
        self.assertIs(assembled.feature_package, feature_package)
        # Raw model metadata is unaffected by feature-package fusion.
        self.assertIsNone(assembled.raw_model.metadata.e2e_schema)


if __name__ == "__main__":
    unittest.main()
