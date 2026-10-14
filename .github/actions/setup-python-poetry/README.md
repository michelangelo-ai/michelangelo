# Setup Python and Poetry Action

A composite GitHub Action that handles the complete Python and Poetry setup workflow including:

- Python environment setup
- Poetry installation and configuration
- Virtual environment caching
- Dependency installation

**Note**: This action requires the repository to be checked out first using `actions/checkout@v4`.

## Usage

### Basic Usage

```yaml
- name: Checkout repository
  uses: actions/checkout@v4

- name: Setup Python and Poetry
  uses: ./.github/actions/setup-python-poetry
```

### With Custom Options

```yaml
- name: Setup Python and Poetry
  uses: ./.github/actions/setup-python-poetry
  with:
    python-version: '3.10'
    working-directory: './python'
    install-root-project: 'true'
    poetry-version: '2.2.1'
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `python-version` | Python version to setup | No | `3.9` |
| `working-directory` | Working directory for Poetry operations | No | `./python` |
| `install-root-project` | Whether to install the root project | No | `true` |
| `poetry-version` | Poetry version to install | No | `2.2.1` |

## What it does

1. Sets up Python with `actions/setup-python`.
2. Installs Poetry (with the `poetry-dynamic-versioning` plugin) using `snok/install-poetry`, configured to create the virtualenv inside the project (`.venv`).
3. Restores the `.venv` directory under `working-directory` from `actions/cache`. The cache key combines the runner OS, the resolved Python version, and a hash of every `poetry.lock` under `working-directory`.
4. On a cache miss only, runs `poetry install --no-interaction --no-root -E dev` in `working-directory`.
5. If `install-root-project` is `'true'`, runs `poetry install --no-interaction -E dev` to install the root project (this runs regardless of the cache result).
