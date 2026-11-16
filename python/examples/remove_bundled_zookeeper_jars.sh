#!/bin/bash
# Remove the ZooKeeper client jars that Spark and pyspark bundle.
#
# Spark ships zookeeper-<version>.jar and zookeeper-jute-<version>.jar for
# standalone-master HA recovery (spark.deploy.recoveryMode=ZOOKEEPER) and for
# Hive's ZooKeeper lock manager / service discovery. Michelangelo runs Spark in
# local and Kubernetes mode and uses none of these, but the jars are flagged by
# vulnerability scanners and no Spark release bundles a fixed version yet.
# Run this in the same step (or Docker RUN) that installs Spark/pyspark, so the
# jars never land in an image layer or CI virtualenv.
#
# Usage:
#   remove_bundled_zookeeper_jars.sh [JARS_DIR ...]
#
# With no arguments, the jars directory of the pyspark package importable by
# ${PYTHON:-python3} is used. Only the two ZooKeeper jars are removed; the
# Curator jars stay (they are inert without ZooKeeper).
#
# To restore the jars, reinstall pyspark / re-extract the Spark distribution
# without running this script.
set -euo pipefail

if [[ $# -eq 0 ]]; then
  pyspark_dir="$("${PYTHON:-python3}" -c 'import os, pyspark; print(os.path.dirname(pyspark.__file__))')"
  set -- "${pyspark_dir}/jars"
fi

shopt -s nullglob
for dir in "$@"; do
  if [[ ! -d "${dir}" ]]; then
    echo "error: ${dir} is not a directory" >&2
    exit 1
  fi
  jars=("${dir}"/zookeeper-[0-9]*.jar "${dir}"/zookeeper-jute-[0-9]*.jar)
  if [[ ${#jars[@]} -eq 0 ]]; then
    echo "no bundled ZooKeeper jars in ${dir}"
    continue
  fi
  for jar in "${jars[@]}"; do
    rm -f "${jar}"
    echo "removed ${jar}"
  done
done
