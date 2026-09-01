"""End-to-end example for the ScalaSparkTask uniflow plugin.

Runs a pre-compiled Scala Spark JAR (ScalaTest, see src/ScalaTest.scala)
as a single-task UniFlow workflow. Build the JAR first with build.sh, then
run this file directly for a local-run smoke test.
"""

from __future__ import annotations

import os

import michelangelo.uniflow.core as uniflow
from examples.pipelines.scala_test.scala_step import scala_test
from michelangelo.uniflow.plugins.scala import ScalaSparkTask

__all__ = ["scala_test_workflow"]


@uniflow.workflow()
def scala_test_workflow(
    main_file: str = "local:///app/examples/pipelines/scala_test/target/ScalaTest.jar",
    main_class: str = "ScalaTest",
):
    """Run the ScalaTest Spark JAR as a single ScalaSparkTask.

    Args:
        main_file: Path/URL to the compiled ScalaTest JAR (see build.sh).
            Any URL fsspec understands works on a local-run; on a cluster
            it must be reachable by the Spark driver/executor pods.
        main_class: Fully-qualified Spark main class in the JAR.
    """
    step = scala_test.with_overrides(
        alias="scala_test_overrides",
        config=ScalaSparkTask(main_file=main_file, main_class=main_class),
    )
    return step()


if __name__ == "__main__":
    # Default JAR built by build.sh, resolved here (outside the transpiled
    # workflow body, which only allows literal argument defaults) so
    # `python scala_test.py` works out of the box after `./build.sh`.
    default_jar = os.path.join(os.path.dirname(__file__), "target", "ScalaTest.jar")

    ctx = uniflow.create_context()
    ctx.run(scala_test_workflow, main_file=default_jar)
