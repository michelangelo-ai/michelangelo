# Scala Test

Minimal end-to-end example for the `ScalaSparkTask` uniflow plugin: runs a
pre-compiled Scala Spark JAR as a single-task workflow.

Unlike `SparkTask`, the task's Python function body does no work itself —
`ScalaTest.scala`'s `main()` is a self-contained Spark program that
`ScalaSparkTask.pre_run()` (local-run) or the `SparkJob` CRD's driver pod
(remote-run) executes directly.

## Files

| File | Description |
|---|---|
| `src/ScalaTest.scala` | Spark job: sums 1..5 and asserts the result is 15 |
| `build.sh` | Compiles the JAR with Scala 2.12 (see Prerequisites) |
| `scala_step.py` | `scala_test` task, wraps the JAR in a `ScalaSparkTask` |
| `scala_test.py` | Workflow entry point |

## Prerequisites

- `brew install scala@2.12` — Spark 3.5.x is built against Scala 2.12; a
  Scala 3 compiler produces a stdlib-incompatible JAR
  (`NoSuchMethodError: ScalaRunTime$.wrapRefArray`).
- Java 11 on `PATH`/`JAVA_HOME` for both `scalac` and `spark-submit`
  (`brew install openjdk@11`) — Scala 3's compiler needs a Java 17+ JVM to
  *run*, but can target Java 17-21 bytecode only, so this example is built
  and run under Java 11 to keep both scalac and Spark's JVM version simple.
- PySpark installed in the project venv (already true for `python/.venv`;
  provides both the `pyspark` jars used at compile time and the
  `spark-submit` binary used at run time).

## Build

```bash
cd michelangelo-ai/michelangelo/python
examples/pipelines/scala_test/build.sh
```

Produces `target/ScalaTest.jar`.

## Local Run

```bash
cd michelangelo-ai/michelangelo/python
JAVA_HOME=/opt/homebrew/opt/openjdk@11 \
PATH="$(pwd)/.venv/bin:/opt/homebrew/opt/openjdk@11/bin:$PATH" \
PYTHONPATH=. .venv/bin/python3 examples/pipelines/scala_test/scala_test.py
```

`.venv/bin` must be on `PATH` so `ScalaSparkTask.pre_run()`'s `spark-submit`
subprocess call resolves to the venv's PySpark-provided binary.

## Expected Output

```
ScalaTest: sum = 15
ScalaTest: SUCCESS
```

## How It Works

`scala_step.py` decorates a no-op function with `ScalaSparkTask(main_file="", main_class="")` —
placeholders, since `main_file` is inherently a per-run value. `scala_test.py`'s
workflow overrides both via `.with_overrides(config=ScalaSparkTask(main_file=..., main_class=...))`
before calling the task, mirroring how `california_housing_xgb` overrides
`SparkTask` resource fields per call.

Workflow-function argument defaults must be literal constants (the
uniflow→Starlark transpiler statically resolves them and rejects references
to module-level globals), so the default JAR path is resolved in the
`if __name__ == "__main__":` block and passed explicitly to `ctx.run()`
rather than as a `def scala_test_workflow(main_file=SOME_GLOBAL)` default.
