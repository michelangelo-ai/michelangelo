# Setup Python and Poetry Action

A composite GitHub Action that handles the complete Python and Poetry setup workflow including:

- Repository checkout
- Python environment setup
- Poetry installation and configuration
- Virtual environment caching
- Dependency installation

## Usage

### Basic Usage

```yaml
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
    poetry-version: 'latest'
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `python-version` | Python version to setup | No | `3.9` |
| `working-directory` | Working directory for Poetry operations | No | `./python` |
| `install-root-project` | Whether to install the root project | No | `true` |
| `poetry-version` | Poetry version to install | No | `latest` |
