"""Run the canvasflex_ray_spark example end-to-end, in-process.

Usage (from python/):
    poetry run python -m examples.canvasflex_ray_spark.run_example

The task bodies run in this process: SparkTask.pre_run starts a local Spark
session (spark.master below) and RayTask.pre_run starts a local Ray runtime.
"""

import os
from pathlib import Path

from michelangelo.canvas.pipeline.run import run_pipeline

if __name__ == "__main__":
    # SparkTask.pre_run reads _SPARK_PROPERTIES; without a master the local
    # in-process Spark session cannot start. Remote runs get the master from
    # the sandbox's Spark job submission instead.
    os.environ.setdefault("_SPARK_PROPERTIES", "spark.master=local[2]")

    pipeline_conf_path = Path(__file__).parent / "pipeline_conf.yaml"
    result = run_pipeline(pipeline_conf_path)
    print(f"result: {result}")
