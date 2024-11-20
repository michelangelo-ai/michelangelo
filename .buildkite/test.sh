#!/bin/bash

set -euo pipefail # strict mode

# ------------------------------------------------------------------------------
# Fixing permissions for non-root user
# ------------------------------------------------------------------------------
#
# Initially, mounted working directory is owned by the special buildkite agent
# user - 2000, which causes problems for non-root users. Hence, we change
# owner of the working directory to the current user. Also, we allow others to
# write, as buildkite's user may write log files.
#
# Details:
# https://github.com/buildkite-plugins/docker-compose-buildkite-plugin/issues/213

IAM=$(whoami)
sudo chown -R "$IAM:$IAM" .
sudo chmod -R ugo+rw .

# ------------------------------------------------------------------------------
# Test
# ------------------------------------------------------------------------------

ls -al
bazel test ... --deleted_packages=tests
# fossa analyze
