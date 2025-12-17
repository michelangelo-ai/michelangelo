"""Create deployable + raw model packages using CustomTritonPackager.

Run from the `python/` directory:

  PYTHONPATH="." poetry run python ./examples/model_manager/custom_triton_packager/package_model.py
"""

from __future__ import annotations

import argparse
import os
import tempfile

try:
    import numpy as np
except ModuleNotFoundError as e:  # pragma: no cover
    raise ModuleNotFoundError(
        "This example requires numpy. Run it from the `python/` directory with "
        "`poetry run ...` after installing the example deps, e.g. "
        "`poetry install -E example`."
    ) from e

from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.serde.model import load_raw_model

from examples.model_manager.custom_triton_packager.toy_model import ToyAddBiasModel


def _schema() -> ModelSchema:
    return ModelSchema(
        input_schema=[
            ModelSchemaItem(name="input", data_type=DataType.INT, shape=[1]),
        ],
        output_schema=[
            ModelSchemaItem(name="response", data_type=DataType.INT, shape=[1]),
        ],
    )


def _sample_data() -> list[dict[str, np.ndarray]]:
    return [{"input": np.array([1], dtype=np.int32)}]


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
        "--out",
        dest="out_dir",
        default=None,
        help="Output directory for packages. Defaults to a temp directory.",
    )
    parser.add_argument(
        "--mode",
        choices=["deployable", "raw", "both"],
        default="both",
        help="Which package(s) to create.",
    )
    args = parser.parse_args()

    model_class = "examples.model_manager.custom_triton_packager.toy_model.ToyAddBiasModel"
    include_prefixes = ["examples.model_manager.custom_triton_packager"]

    with tempfile.TemporaryDirectory() as tmp:
        out_dir = os.path.abspath(args.out_dir or os.path.join(tmp, "out"))
        os.makedirs(out_dir, exist_ok=True)

        # Create model artifacts (what Triton packaging will copy under model/).
        artifacts_dir = os.path.join(out_dir, "toy_model_artifacts")
        ToyAddBiasModel(bias=3).save(artifacts_dir)

        packager = CustomTritonPackager()
        schema = _schema()
        sample_data = _sample_data()

        if args.mode in ("deployable", "both"):
            deployable_path = os.path.join(out_dir, "deployable_toy_add_bias")
            created = packager.create_model_package(
                model_path=artifacts_dir,
                model_class=model_class,
                model_schema=schema,
                model_name="toy_add_bias",
                dest_model_path=deployable_path,
                include_import_prefixes=include_prefixes,
            )
            print(f"\nDeployable Triton package created at: {created}")
            _print_tree(created)

        if args.mode in ("raw", "both"):
            raw_path = os.path.join(out_dir, "raw_toy_add_bias")
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

            # Optional: prove the raw package can be loaded and predicts correctly.
            loaded = load_raw_model(created)
            pred = loaded.predict(sample_data[0])
            print(f"\nLoaded raw model predicts: {pred}")

    if args.out_dir is None:
        print(
            "\nNote: output was created in a temp directory and will be deleted at exit.\n"
            "Pass --out <dir> to keep the generated packages."
        )

    return 0


if __name__ == "__main__":
    raise SystemExit(main())


