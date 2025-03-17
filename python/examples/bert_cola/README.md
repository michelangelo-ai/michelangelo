Example project showing various capabilities of the Michelangelo workflows.

## Project Structure

- Dockerfile for task code distribution. Used for running workflows in remote mode.
- BUILD.bazel defines project-level metadata and tasks.
- commons directory contains reusable utility code.
- Other directories represent concrete workflow use cases.

## Local Run

**Prerequisite**


**Run workflows locally**

Workflows run locally as an ordinary Python program. Just use relevant `py_binary` Bazel target to run a workflow in the
local mode. Ex:

    bazel run //uber/ai/michelangelo/sandbox/boston_housing_xgb
    bazel run //uber/ai/michelangelo/sandbox/torch_example
    bazel run //uber/ai/michelangelo/sandbox/bert_cola
    bazel run //uber/ai/michelangelo/sandbox/nlp_two_tower

For IDE users to access ray dashboard,
- Command + Cmd Shift + P
- SimpleBrower
- http://127.0.0.1:8265

## Remote Run

**Prerequisite**

setup sandbox, see /python/michelangelo/sandbox/README.md

    ma sandbox create

The create setup dependencies for Uniflow including

- Michelangelo-ai API
- Michelangelo-ai controller manager
- Cadence
- Starlark-worker
- S3 storage

<hr/>

**Run workflow**

Running workflows in the remote mode requires a docker container that contains code of the workflow tasks. Build
a new revision of the project's container, or use an existing revision if you didn't change task code.

    docker build -t bert-cola-nv:latest .

Copy the build's `Revision ID`, we use it later.

In order for Kubernetes to pull the image, push it to a registry that the cluster has access to. For example, push it to

    k3d image import bert-cola-nv:latest -c michelangelo-sandbox

Before running the remote, we need to have a default storage bucket. If you don't have one, create it.
In your browser, open http://localhost:9090/buckets, click "Create Bucket" and create a bucket with the name `default`.
<hr/>

**Run workflows in the remote cluster**

Use `.remote_run` Bazel target to run a workflow in the remote mode. Ex:

    poetry run python ./examples/bert_cola/bert_cola.py  remote-run --image docker.io/library/bert-cola-nv:latest --storage-url s3://default --yes

<hr/>

***Check your workflow in cadence***

fetch workflow id from output of remote run:

Example:

    Started Workflow Id: examples.bert_cola.bert_cola.train_workflow.0v5ja, run Id: e2632073-d0c0-4f09-a9c4-b1c923f9064c

****Check the workflow in Cadence****

Cadence workflow Id: `examples.bert_cola.bert_cola.train_workflow.0v5ja`

<hr/>

****Ray Cluster provisioning****

Waiting for cluster started and open the cluster dashboard URL

Example:
The cluster dashboard URL is `http://10.6.4.1:8265`

<hr/>

****Monitor load data job in dashboard****

Bert-Cola example contains 2 steps, first step is load_data to download data from Huggingface

After cluster becomes ready state, you can now see the ray job submitted to the cluster.
Now open browser to see the ray job running log

<hr/>

****Clean up load data step cluster****

When the job finishes, the cluster will be terminated by Uniflow

<hr/>

****Monitor train data job in dashboard****

Second step is training. Now open browser to see the ray job running log for the second cluster.

<hr/>

****Clean up training step cluster****

When the job finishes, the cluster will be terminated by Uniflow

<hr/>

**Kill workflows in the remote run**

For the remote run from sandbox, terminate the workflow in Cadence.

    visit http://localhost:8088/ 
    And terminate target workflow


