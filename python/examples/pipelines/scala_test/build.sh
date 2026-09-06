#!/usr/bin/env bash
# Compiles src/ScalaTest.scala into target/ScalaTest.jar.
#
# Spark 3.5.x is built against Scala 2.12, whose stdlib is not binary
# compatible with Scala 3 (NoSuchMethodError: ScalaRunTime$.wrapRefArray) —
# this must be compiled with a Scala 2.12.x compiler, not scalac 3.x.
#
# Prerequisites (macOS):
#   brew install scala@2.12 openjdk@11
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON_DIR="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
SCALAC="${SCALAC:-/opt/homebrew/opt/scala@2.12/bin/scalac}"
JAVA_HOME="${JAVA_HOME:-/opt/homebrew/opt/openjdk@11}"
PYSPARK_PYTHON="${PYSPARK_PYTHON:-${PYTHON_DIR}/.venv/bin/python3}"

if [[ ! -x "${SCALAC}" ]]; then
  echo "ERROR: scalac 2.12 not found at ${SCALAC} (brew install scala@2.12)" >&2
  exit 1
fi

BUILD_DIR="${SCRIPT_DIR}/target/classes"
JAR_PATH="${SCRIPT_DIR}/target/ScalaTest.jar"

rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"

echo "Compiling ScalaTest.scala with $(${SCALAC} -version 2>&1)..."
JAVA_HOME="${JAVA_HOME}" PATH="${JAVA_HOME}/bin:${PATH}" \
  "${SCALAC}" -classpath "$("${PYSPARK_PYTHON}" -c 'import pyspark,os; print(os.path.join(os.path.dirname(pyspark.__file__), "jars", "*"))')" \
  -d "${BUILD_DIR}" "${SCRIPT_DIR}/src/ScalaTest.scala"

echo "Packaging ${JAR_PATH}..."
JAVA_HOME="${JAVA_HOME}" "${JAVA_HOME}/bin/jar" --create --file "${JAR_PATH}" -C "${BUILD_DIR}" .

echo "Built ${JAR_PATH}"
