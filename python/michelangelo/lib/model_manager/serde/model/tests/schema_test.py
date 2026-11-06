"""Tests for loading model schemas from raw and deployable packages."""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase

import yaml

from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.config_pbtxt import (
    generate_config_pbtxt_content,
)
from michelangelo.lib.model_manager._private.schema.common.serde import (
    schema_to_dict,
    schema_to_yaml,
)
from michelangelo.lib.model_manager._private.serde.model import (
    get_supported_deployable_backend_types,
)
from michelangelo.lib.model_manager._private.serde.model.triton_backend_type import (
    get_deployable_backend_type,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.serde.model import (
    load_deployable_model_schema,
    load_model_schema,
)

_RAW = ModelSchema(
    input_schema=[ModelSchemaItem("a", DataType.FLOAT, [1])],
    output_schema=[ModelSchemaItem("raw_out", DataType.FLOAT, [1])],
)
_DEPLOYABLE = ModelSchema(
    input_schema=[
        ModelSchemaItem("ids", DataType.LONG, [3]),
        ModelSchemaItem("grid", DataType.DOUBLE, [2, 4]),
        ModelSchemaItem("text", DataType.STRING, [1], optional=True),
    ],
    output_schema=[ModelSchemaItem("score", DataType.FLOAT, [1])],
)


class _PackageTest(TestCase):
    """Gives each test an empty package directory."""

    def setUp(self) -> None:
        """Create a scratch package directory."""
        self._tmp = tempfile.TemporaryDirectory()
        self.root = self._tmp.name

    def tearDown(self) -> None:
        """Remove the scratch directory."""
        self._tmp.cleanup()

    def write(self, relative: str, content: str) -> None:
        """Write ``content`` to ``relative`` inside the package."""
        path = os.path.join(self.root, relative)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w") as f:
            f.write(content)

    def write_raw(self) -> None:
        """Write a raw package's schema.yaml."""
        self.write("metadata/schema.yaml", schema_to_yaml(_RAW))

    def write_deployable(self) -> None:
        """Write the config.pbtxt the torch packager produces."""
        self.write(
            "config.pbtxt",
            generate_config_pbtxt_content(
                TritonTemplateRenderer(), "m", "1", _DEPLOYABLE
            ),
        )


class LoadModelSchemaTest(_PackageTest):
    """Public ``load_model_schema``."""

    def test_raw_package(self):
        """A raw package's schema.yaml is read."""
        self.write_raw()
        self.assertEqual(load_model_schema(self.root), _RAW)

    def test_deployable_package(self):
        """A deployable package's config.pbtxt is read."""
        self.write_deployable()
        names = [i.name for i in load_model_schema(self.root).input_schema]
        self.assertEqual(names, ["ids", "grid", "text"])

    def test_a_package_with_both_is_read_as_raw(self):
        """schema.yaml wins over config.pbtxt."""
        self.write_raw()
        self.write_deployable()
        self.assertEqual(load_model_schema(self.root), _RAW)

    def test_submodule_reads_submodel_schemas(self):
        """A submodule's schema comes from submodel_schemas.yaml."""
        self.write_raw()
        sub = ModelSchema(
            input_schema=[ModelSchemaItem("h", DataType.FLOAT, [8])],
            output_schema=[ModelSchemaItem("o", DataType.FLOAT, [2])],
        )
        self.write(
            "metadata/submodel_schemas.yaml",
            yaml.dump({"transform_module.layers.0": schema_to_dict(sub)}),
        )

        self.assertEqual(
            load_model_schema(self.root, submodule="transform_module.layers.0"), sub
        )
        with self.assertRaisesRegex(KeyError, "missing.layer"):
            load_model_schema(self.root, submodule="missing.layer")

    def test_submodule_of_a_deployable_package_is_rejected(self):
        """Only raw packages have submodel schemas."""
        self.write_deployable()
        with self.assertRaisesRegex(ValueError, "only available for raw packages"):
            load_model_schema(self.root, submodule="a.b")

    def test_missing_path_is_reported(self):
        """A path that does not exist is a FileNotFoundError."""
        with self.assertRaisesRegex(FileNotFoundError, "Model path not found"):
            load_model_schema(os.path.join(self.root, "nope"))

    def test_unknown_package_format_is_rejected(self):
        """A directory with neither file is a ValueError."""
        with self.assertRaisesRegex(ValueError, "Unknown model package format"):
            load_model_schema(self.root)


class LoadDeployableModelSchemaTest(_PackageTest):
    """Parsing ``config.pbtxt``."""

    def test_round_trips_names_types_and_shapes(self):
        """What the packager writes is read back, without the optional flag."""
        self.write_deployable()

        schema = load_deployable_model_schema(self.root)

        self.assertEqual(
            [(i.name, i.data_type, i.shape, i.optional) for i in schema.input_schema],
            [
                ("ids", DataType.LONG, [3], None),
                ("grid", DataType.DOUBLE, [2, 4], None),
                ("text", DataType.STRING, [1], None),
            ],
        )
        self.assertEqual(
            [(i.name, i.data_type, i.shape) for i in schema.output_schema],
            [("score", DataType.FLOAT, [1])],
        )

    def test_unknown_types_items_without_dims_and_missing_sections(self):
        """An unknown type reads as FLOAT, an item without dims is skipped."""
        self.write(
            "config.pbtxt",
            'backend: "python"\n'
            "input: [\n"
            '  { name: "a", data_type: TYPE_BF16, dims: [ ] },\n'
            '  { name: "no_dims", data_type: TYPE_FP32 }\n'
            "]\n",
        )

        schema = load_deployable_model_schema(self.root)

        self.assertEqual(
            [(i.name, i.data_type, i.shape) for i in schema.input_schema],
            [("a", DataType.FLOAT, [])],
        )
        self.assertEqual(schema.output_schema, [])

    def test_needs_config_pbtxt(self):
        """A directory without config.pbtxt is a FileNotFoundError."""
        with self.assertRaisesRegex(FileNotFoundError, "config.pbtxt not found"):
            load_deployable_model_schema(self.root)


class DeployableBackendTypeTest(_PackageTest):
    """Reading the backend from ``config.pbtxt``."""

    def test_reads_the_backend(self):
        """The backend line is returned."""
        self.write("config.pbtxt", 'name: "m"\nbackend: "onnxruntime"\n')
        self.assertEqual(get_deployable_backend_type(self.root), "onnxruntime")

    def test_errors(self):
        """A missing file, a missing line and an unknown backend are reported."""
        with self.assertRaisesRegex(FileNotFoundError, "config.pbtxt not found"):
            get_deployable_backend_type(self.root)
        self.write("config.pbtxt", "max_batch_size: 8\n")
        with self.assertRaisesRegex(ValueError, "Could not find backend"):
            get_deployable_backend_type(self.root)
        self.write("config.pbtxt", 'backend: "tensorrt"\n')
        with self.assertRaisesRegex(ValueError, "Invalid backend type 'tensorrt'"):
            get_deployable_backend_type(self.root)

    def test_supported_backends(self):
        """The supported set is the three packaging backends."""
        self.assertEqual(
            get_supported_deployable_backend_types(),
            {"pytorch", "onnxruntime", "python"},
        )
