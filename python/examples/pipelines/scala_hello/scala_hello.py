"""End-to-end example for the ScalaTask uniflow plugin.

Runs a pre-compiled Scala Spark JAR (HelloScala, see src/HelloScala.scala)
as a single-task UniFlow workflow. Build the JAR first with build.sh, then
run this file directly for a local-run smoke test.
"""

from __future__ import annotations

import os

import michelangelo.uniflow.core as uniflow
from examples.pipelines.scala_hello.scala_step import hello_scala
from michelangelo.uniflow.plugins.scala import ScalaTask

__all__ = ["scala_hello_workflow"]


@uniflow.workflow()
def scala_hello_workflow(main_file: str = "local:///app/examples/pipelines/scala_hello/target/HelloScala.jar", main_class: str = "HelloScala"):
    """Run the HelloScala Spark JAR as a single ScalaTask.

    Args:
        main_file: Path/URL to the compiled HelloScala JAR (see build.sh).
            Any URL fsspec understands works on a local-run; on a cluster
            it must be reachable by the Spark driver/executor pods.
        main_class: Fully-qualified Spark main class in the JAR.
    """
    step = hello_scala.with_overrides(
        alias="hello_scala_overrides",
        config=ScalaTask(main_file=main_file, main_class=main_class),
    )
    return step()


if __name__ == "__main__":
    # Default JAR built by build.sh, resolved here (outside the transpiled
    # workflow body, which only allows literal argument defaults) so
    # `python scala_hello.py` works out of the box after `./build.sh`.
    default_jar = os.path.join(os.path.dirname(__file__), "target", "HelloScala.jar")

    ctx = uniflow.create_context()
    ctx.run(scala_hello_workflow, main_file=default_jar)
