#!/usr/bin/env bash

set -eou pipefail

# Chronon configuration for Amazon Books Qwen pipeline
CHRONON_JAR=${CHRONON_JAR:=https://repo1.maven.org/maven2/ai/chronon/spark_uber_2.12/0.0.23/spark_uber_2.12-0.0.23-assembly.jar}

if [ -z "${SPARK_HOME:+x}" ]; then
  if ! command -v spark-submit &>/dev/null; then
    echo "[ error ] SPARK_HOME env var must be set, or spark-submit script must be in the PATH."
    exit 1
  fi
  SPARK_SUBMIT=$(which spark-submit)
else
  SPARK_SUBMIT=$SPARK_HOME/bin/spark-submit
fi

cat <<INFO
Amazon Books Qwen - Chronon Feature Engineering
===============================================
SPARK_HOME    :: ${SPARK_HOME-[unset]}
SPARK_SUBMIT  :: $SPARK_SUBMIT
CHRONON_JAR   :: $CHRONON_JAR

INFO

$SPARK_SUBMIT --class ai.chronon.spark.Driver $CHRONON_JAR "${@}"