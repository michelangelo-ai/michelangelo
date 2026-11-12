"""Tests for reading schemas from raw and deployable model packages."""

import os
import tempfile
from unittest import TestCase

import yaml

from michelangelo.lib.model_manager._private.schema.common.serde import (
    schema_to_dict,
)
from michelangelo.lib.model_manager._private.serde.model import (
    load_deployable_model_schema,
    load_raw_model_schema,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem


def _items(items):
    return [(i.name, i.data_type, i.shape) for i in items]


class _PackageTest(TestCase):
    """Gives each test a scratch package directory."""

    def setUp(self) -> None:
        """Create the directory."""
        self._tmp = tempfile.TemporaryDirectory()
        self.root = self._tmp.name

    def tearDown(self) -> None:
        """Remove the directory."""
        self._tmp.cleanup()

    def write(self, relative: str, content: str) -> None:
        """Write ``content`` to ``relative`` inside the package."""
        path = os.path.join(self.root, relative)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w") as f:
            f.write(content)


class LoadDeployableModelSchemaTest(_PackageTest):
    """Parsing ``config.pbtxt``."""

    def schema_of(self, content: str) -> ModelSchema:
        """Write ``content`` as config.pbtxt and parse it."""
        self.write("config.pbtxt", content)
        return load_deployable_model_schema(self.root)

    def test_single_input_and_output(self):
        """One input and one output are read with their types and shapes."""
        schema = self.schema_of(
            'backend: "pytorch"\n'
            "input: [\n"
            '  { name: "x", data_type: TYPE_FP32, dims: [ 4 ] }\n'
            "]\n"
            "output: [\n"
            '  { name: "y", data_type: TYPE_INT64, dims: [ 2 ] }\n'
            "]\n"
        )
        self.assertEqual(_items(schema.input_schema), [("x", DataType.FLOAT, [4])])
        self.assertEqual(_items(schema.output_schema), [("y", DataType.LONG, [2])])

    def test_spaces_and_line_breaks_inside_an_item(self):
        """Items may spread over lines and use any spacing around ``:``."""
        schema = self.schema_of(
            "input: [\n"
            "  {\n"
            '    name  :  "x"\n'
            "    data_type:TYPE_FP32\n"
            "    dims :[\n"
            "      3 ,\n"
            "      5\n"
            "    ]\n"
            "  }\n"
            "]\n"
        )
        self.assertEqual(_items(schema.input_schema), [("x", DataType.FLOAT, [3, 5])])

    def test_multiple_inputs_and_outputs_keep_their_order(self):
        """Every item is read, in file order."""
        schema = self.schema_of(
            "input: [\n"
            '  { name: "a", data_type: TYPE_FP32, dims: [ 1 ] },\n'
            '  { name: "b", data_type: TYPE_INT32, dims: [ 2 ] },\n'
            '  { name: "c", data_type: TYPE_STRING, dims: [ 1 ] }\n'
            "]\n"
            "output: [\n"
            '  { name: "p", data_type: TYPE_BOOL, dims: [ 1 ] },\n'
            '  { name: "q", data_type: TYPE_FP32, dims: [ 1 ] }\n'
            "]\n"
        )
        self.assertEqual(
            _items(schema.input_schema),
            [
                ("a", DataType.FLOAT, [1]),
                ("b", DataType.INT, [2]),
                ("c", DataType.STRING, [1]),
            ],
        )
        self.assertEqual(
            _items(schema.output_schema),
            [("p", DataType.BOOLEAN, [1]), ("q", DataType.FLOAT, [1])],
        )

    def test_every_schema_data_type_round_trips(self):
        """Each Triton type the packagers write reads back as its DataType."""
        types = {
            "BOOL": DataType.BOOLEAN,
            "UINT8": DataType.BYTE,
            "INT8": DataType.CHAR,
            "INT16": DataType.SHORT,
            "INT32": DataType.INT,
            "INT64": DataType.LONG,
            "FP32": DataType.FLOAT,
            "FP64": DataType.DOUBLE,
            "STRING": DataType.STRING,
        }
        body = ",\n".join(
            f'  {{ name: "n_{t}", data_type: TYPE_{t}, dims: [ 1 ] }}' for t in types
        )
        schema = self.schema_of(f"input: [\n{body}\n]\n")
        self.assertEqual(
            {i.name: i.data_type for i in schema.input_schema},
            {f"n_{t}": d for t, d in types.items()},
        )

    def test_fp64_is_read_as_double(self):
        """TYPE_FP64 maps to DOUBLE: the schema has no separate numeric type."""
        schema = self.schema_of(
            'input: [ { name: "v", data_type: TYPE_FP64, dims: [ 1 ] } ]\n'
        )
        self.assertEqual(schema.input_schema[0].data_type, DataType.DOUBLE)

    def test_dynamic_dimension(self):
        """A dimension of -1 is kept as -1."""
        schema = self.schema_of(
            'input: [ { name: "x", data_type: TYPE_FP32, dims: [ -1, 8 ] } ]\n'
        )
        self.assertEqual(schema.input_schema[0].shape, [-1, 8])

    def test_multi_dimensional_shape(self):
        """Every dimension is read."""
        schema = self.schema_of(
            'input: [ { name: "x", data_type: TYPE_FP32, dims: [ 3, 224, 224 ] } ]\n'
        )
        self.assertEqual(schema.input_schema[0].shape, [3, 224, 224])

    def test_empty_dims_is_an_empty_shape(self):
        """``dims: [ ]`` is an item with no dimensions, not a missing item."""
        schema = self.schema_of(
            'input: [ { name: "x", data_type: TYPE_FP32, dims: [] } ]\n'
        )
        self.assertEqual(_items(schema.input_schema), [("x", DataType.FLOAT, [])])

    def test_unknown_data_type_is_read_as_float(self):
        """A type with no schema equivalent falls back to FLOAT."""
        schema = self.schema_of(
            'input: [ { name: "x", data_type: TYPE_BF16, dims: [ 2 ] } ]\n'
        )
        self.assertEqual(schema.input_schema[0].data_type, DataType.FLOAT)

    def test_item_missing_a_field_is_skipped(self):
        """An item without a name, data type or dims is not read."""
        schema = self.schema_of(
            "input: [\n"
            "  { data_type: TYPE_FP32, dims: [ 1 ] },\n"
            '  { name: "no_type", dims: [ 1 ] },\n'
            '  { name: "no_dims", data_type: TYPE_FP32 },\n'
            '  { name: "ok", data_type: TYPE_FP32, dims: [ 1 ] }\n'
            "]\n"
        )
        self.assertEqual([i.name for i in schema.input_schema], ["ok"])

    def test_no_inputs(self):
        """A config with only outputs has an empty input schema."""
        schema = self.schema_of(
            'output: [ { name: "y", data_type: TYPE_FP32, dims: [ 1 ] } ]\n'
        )
        self.assertEqual(schema.input_schema, [])
        self.assertEqual([i.name for i in schema.output_schema], ["y"])

    def test_no_outputs(self):
        """A config with only inputs has an empty output schema."""
        schema = self.schema_of(
            'input: [ { name: "x", data_type: TYPE_FP32, dims: [ 1 ] } ]\n'
        )
        self.assertEqual([i.name for i in schema.input_schema], ["x"])
        self.assertEqual(schema.output_schema, [])

    def test_empty_sections(self):
        """Empty brackets give empty schemas."""
        schema = self.schema_of("input: [ ]\noutput: [ ]\n")
        self.assertEqual(schema.input_schema, [])
        self.assertEqual(schema.output_schema, [])

    def test_missing_config_pbtxt(self):
        """A package without config.pbtxt is a FileNotFoundError."""
        with self.assertRaisesRegex(FileNotFoundError, "config.pbtxt not found"):
            load_deployable_model_schema(self.root)


class LoadRawModelSchemaTest(_PackageTest):
    """Reading ``metadata/*.yaml`` of a raw package."""

    def test_top_level_schema(self):
        """Without a submodule, schema.yaml is read."""
        schema = ModelSchema(
            input_schema=[ModelSchemaItem("a", DataType.FLOAT, [1])],
            output_schema=[ModelSchemaItem("b", DataType.INT, [2])],
        )
        self.write("metadata/schema.yaml", yaml.dump(schema_to_dict(schema)))
        self.assertEqual(load_raw_model_schema(self.root), schema)

    def test_submodule_schema(self):
        """A submodule's schema is read from submodel_schemas.yaml."""
        sub = ModelSchema(
            input_schema=[ModelSchemaItem("h", DataType.FLOAT, [8])],
            output_schema=[ModelSchemaItem("o", DataType.FLOAT, [2])],
        )
        self.write(
            "metadata/submodel_schemas.yaml", yaml.dump({"enc.0": schema_to_dict(sub)})
        )
        self.assertEqual(load_raw_model_schema(self.root, submodule="enc.0"), sub)

    def test_unknown_submodule_is_a_key_error(self):
        """A submodule missing from the file is a KeyError naming it."""
        self.write("metadata/submodel_schemas.yaml", yaml.dump({"enc.0": {}}))
        with self.assertRaisesRegex(KeyError, "enc.1"):
            load_raw_model_schema(self.root, submodule="enc.1")

    def test_empty_submodel_schemas_file_is_a_key_error(self):
        """An empty submodel_schemas.yaml has no submodules."""
        self.write("metadata/submodel_schemas.yaml", "")
        with self.assertRaisesRegex(KeyError, "enc.0"):
            load_raw_model_schema(self.root, submodule="enc.0")

    def test_missing_files_are_file_not_found(self):
        """Each schema file must exist."""
        with self.assertRaises(FileNotFoundError):
            load_raw_model_schema(self.root)
        with self.assertRaises(FileNotFoundError):
            load_raw_model_schema(self.root, submodule="enc.0")
