# File sync

## What you'll learn

* How file sync lets you test local changes on remote infrastructure without rebuilding images
* When to use (and not use) file sync
* What files get synced and the typical development flow

## Prerequisites

- **A running sandbox** — File sync pushes code to a remote cluster. Follow the [Sandbox Setup](../../getting-started/sandbox-setup.md) guide first.
- **A Docker image already built for your workflow** — File sync patches an existing image; it does not build one. See [Running Uniflow Pipelines](./running-uniflow.md) for image build steps.
- **A Git repository** — File sync uses Git metadata to determine which files changed since the image was built.
- **Cloud storage credentials** — S3/MinIO access is required to upload the sync tarball (see Troubleshooting below).

## What is file sync?

File Sync lets you test your local code changes on remote infrastructure **without rebuilding Docker images**. Instead of waiting 20+ minutes for image builds per task, you can sync your changes in 2-5 minutes.

## When to use file sync

**Use file sync when:**

* My laptop can't handle this dataset locally, I want to do remote cluster runs with ray/spark task code changes  
* I want to do development runs to increase velocity when developing models, setting up pipelines, or implementing evaluations  
* I want to test changes quickly without committing code

**Don't use file sync when:**

* Your changes affect Dockerfile, Dependencies or Python environment  
* You're ready to deploy to production  
* You're working with sensitive data that shouldn't leave your machine (not applicable to cloud storage)

## How to use file sync

remote run support: add `--file-sync` flag to your remote run command

```shell
python workflow.py remote-run \
    --storage-url s3://my-bucket/workflows \
    --image my-workflow:latest \
    --file-sync
```

`ma pipeline dev-run` support: add `--file-sync` flag to your ma pipeline dev-run command

```shell
ma pipeline dev-run --file-sync --file <path_to_pipeline.yaml>
```

### Requirements

* Your code must be in a Git repository
* You have authorization access to cloud storage (S3/MinIO)

## Important things to know

**File sync assumes your local Git changes relate to the Docker image**

**This works best when:**

* You built the Docker image yourself from your current branch  
* The image contains Git metadata showing which commit it was built from

**This can cause issues when:**

* Someone else built the image you're using  
* You're using an image built from a different branch/commit  
* Your local changes are outside the project root folder OR the volume and complexity of non-dependency changes is excessive to the fundamental system limitations

**What happens:**

* **With Git metadata:** Only sends files that actually changed since the image was built  
* **Without Git metadata:** Sends all your uncommitted changes (may include extra files)

## What gets synced

**Files included:**

* Modified files since the image was built (if image has Git metadata)  
* All your uncommitted changes (if no Git metadata)  
* New files you've created locally  
* Staged changes in Git

**Files excluded:**

* Files listed in `.gitignore`  
* Large binary files (should be in `.gitignore`)  
* Unchanged files (when Git metadata is available)

## Typical Development Flow

1. **Make code changes** (don't commit yet)  
2. **Run with file sync** \- your changes are tested remotely in 2-5 minutes  
3. **Iterate quickly** \- repeat steps 1-2 until satisfied  
4. **Commit and rebuild image** only when ready for production

## Troubleshooting

### Missing cloud storage credentials

If the sync fails with:
```
Failed to upload tarball: Unable to locate credentials
```

Set credentials before running:

```bash
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export AWS_ENDPOINT_URL=http://localhost:9091
```

### Unexpected files being synced

If more files are synced than expected, your Docker image may not have Git metadata. In that case, file sync sends all uncommitted changes rather than just the diff since the image was built. Ensure the image is built from your current branch with Git history included.

### File sync not picking up changes

File sync only includes files tracked by Git. If you added new files, make sure they are staged (`git add`) so Git is aware of them.

## Next Steps

- **Cache results between runs** — See [Uniflow caching and pipeline run resume](./cache-and-pipelinerun-resume-form.md) to skip unchanged steps and resume failed runs
- **Run on a schedule** — See [Set Up Triggers](../set-up-triggers.md) to automate pipeline execution with cron triggers
- **Build and register your model** — Once your code is validated, follow the [Model Registry Guide](../model-registry-guide.md) to package and version your model
