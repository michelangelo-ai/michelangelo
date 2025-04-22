Sandbox is a lightweight version of the Michelangelo cluster, designed specifically for development and testing.
It also serves as an excellent tool for users to quickly explore the platform and familiarize themselves with its
interface.

> Note: The Sandbox deployment is intended for development and testing purposes only and is not suitable for production
> environments.
> For guidance on creating a production-ready Michelangelo deployment, please refer to the Deployment Guide.

## User Guide

### Prerequisites

**Required Software**

Please install the following software before proceeding:

- [Docker](https://docs.docker.com/get-started/get-docker)
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
- [k3d](https://k3d.io)

**GitHub Personal Access Token**

Michelangelo is not publicly available yet, so we keep Michelangelo's Docker containers in the private GitHub Container
Registry, which requires a GitHub personal access token (classic) for authentication.

To enable authentication for the sandbox, please create a GitHub personal access token (classic) with the
"read:packages" scope and save it to the `CR_PAT` environment variable. For example, you can add the following line to
your shell configuration file (such as `.bashrc` or `.zshrc`, depending on the shell you use):

```
export CR_PAT=your_token_...
```

For a more detailed guide, please refer
to https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry#authenticating-with-a-personal-access-token-classic.

> Be aware that `CR_PAT` environment variable is required while Michelangelo is NOT publicly accessible. Once we become
> public, the token will no longer be necessary, and this section will be removed.

TODO: andrii: remove this section after the public release of Michelangelo

### Install Michelangelo CLI

```bash
pip install michelangelo
ma sandbox --help
```

### Setting up Temporal Development Server

To quickly set up a Temporal development server for local development and testing, follow these brief steps:

```bash
brew install temporal
temporal server start-dev
```

For detailed instructions and additional setup options, please follow the [Temporal Development Environment Guide](https://learn.temporal.io/getting_started/typescript/dev_environment/).

