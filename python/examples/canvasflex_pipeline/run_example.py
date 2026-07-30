"""Run the canvasflex_pipeline example end-to-end.

Usage (from python/):
    poetry run python -m examples.canvasflex_pipeline.run_example
"""

from pathlib import Path

from michelangelo.canvas.pipeline.run import run_pipeline

if __name__ == "__main__":
    pipeline_conf_path = Path(__file__).parent / "pipeline_conf.yaml"
    result = run_pipeline(pipeline_conf_path)
    print(f"result: {result}")
