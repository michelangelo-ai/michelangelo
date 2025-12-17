"""Package a simple custom model using CustomTritonPackager.

Run from the `python/` directory:

  PYTHONPATH="." poetry run python ./examples/model_manager/simple_custom/package_model.py --out /tmp/mm-simple-custom
"""

from __future__ import annotations

import argparse
import os
import shutil

import numpy as np

from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.serde.model import load_raw_model

from examples.model_manager.simple_custom.model import DummyEchoModel


def _schema() -> ModelSchema:
    return ModelSchema(
        input_schema=[
            ModelSchemaItem(name="a", data_type=DataType.INT, shape=[1]),
            ModelSchemaItem(name="b", data_type=DataType.INT, shape=[1], optional=True),
        ],
        output_schema=[
            ModelSchemaItem(name="response", data_type=DataType.INT, shape=[1]),
            ModelSchemaItem(name="response2", data_type=DataType.INT, shape=[1]),
        ],
    )


def _sample_data() -> list[dict[str, np.ndarray]]:
    return [
        {"a": np.array([1], dtype=np.int32), "b": np.array([2], dtype=np.int32)},
        {"a": np.array([2], dtype=np.int32), "b": np.array([3], dtype=np.int32)},
    ]


def _print_tree(root: str) -> None:
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames.sort()
        filenames.sort()
        rel = os.path.relpath(dirpath, root)
        prefix = "" if rel == "." else f"{rel}/"
        for f in filenames:
            print(f"- {prefix}{f}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--out", required=True, help="Output directory to write packages"
    )
    parser.add_argument(
        "--mode",
        choices=["deployable", "raw", "both"],
        default="both",
        help="Which package(s) to create",
    )
    args = parser.parse_args()

    out_dir = os.path.abspath(args.out)
    os.makedirs(out_dir, exist_ok=True)

    model_class = "examples.model_manager.simple_custom.model.DummyEchoModel"
    include_prefixes = ["examples.model_manager.simple_custom"]

    # Create model artifacts (what packaging copies under model/).
    artifacts_dir = os.path.join(out_dir, "artifacts")
    DummyEchoModel().save(artifacts_dir)

    packager = CustomTritonPackager()
    schema = _schema()
    sample_data = _sample_data()

    if args.mode in ("deployable", "both"):
        deployable_path = os.path.join(out_dir, "deployable")
        shutil.rmtree(deployable_path, ignore_errors=True)
        created = packager.create_model_package(
            model_path=artifacts_dir,
            model_class=model_class,
            model_schema=schema,
            model_name="dummy_echo",
            dest_model_path=deployable_path,
            include_import_prefixes=include_prefixes,
        )
        print(f"\nDeployable Triton package created at: {created}")
        _print_tree(created)

    if args.mode in ("raw", "both"):
        raw_path = os.path.join(out_dir, "raw")
        shutil.rmtree(raw_path, ignore_errors=True)
        created = packager.create_raw_model_package(
            model_path=artifacts_dir,
            model_class=model_class,
            model_schema=schema,
            sample_data=sample_data,
            dest_model_path=raw_path,
            requirements=["numpy"],
            include_import_prefixes=include_prefixes,
        )
        print(f"\nRaw model package created at: {created}")
        _print_tree(created)

        loaded = load_raw_model(created)
        pred = loaded.predict(sample_data[0])
        print(f"\nLoaded raw model predicts: {pred}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
