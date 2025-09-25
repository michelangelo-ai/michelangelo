# Michelangelo Starlark

Michelangelo Starlark lets you write workflows in the Starlark scripting language and run them on Cadence or Temporal, without needing to redeploy the worker.

## Overview

Michelangelo Starlark is a Cadence/Temporal worker that runs a generic workflow. This workflow takes Starlark code as input and executes it using an embedded Starlark interpreter. The interpreter is integrated with the Cadence/Temporal SDK, so your Starlark code can call Cadence/Temporal APIs directly.

## Getting Started

### Running Starlark Files

Ensure that Sandbox is running:

If running on Cadence:
```bash
ma sandbox create
```

If running on Temporal:
```bash
ma sandbox create --workflow=temporal
```

More information to run sandbox can be found [here](https://github.com/michelangelo-ai/michelangelo/wiki/Getting-Started#running-michelangelos-api-sandbox-environment).

If running on Cadence, use the following command:
```bash
cd $WORKSPACE_ROOT
./tools/starlark.py run ./testdata/ping.star
```

If running on Temporal, use the following instead:
```bash
cd $WORKSPACE_ROOT
./tools/starlark.py run ./testdata/ping.star --workflow=temporal
```


### Running the Starlark Worker On Local Changes

To test local changes, such as when developing a new plugin, first follow the steps in the [Cadence Worker README](https://github.com/michelangelo-ai/michelangelo/blob/main/go/cmd/worker/README.md) to build and start the Cadence/Temporal Worker locally.

Then, run the Starlark Worker using the following command for Cadence:
```bash
cd $WORKSPACE_ROOT
./tools/starlark.py run ./testdata/ping.star
```

Or the following for Temporal:
```bash
cd $WORKSPACE_ROOT
./tools/starlark.py run ./testdata/ping.star --workflow=temporal
```