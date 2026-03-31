# Set Up Your Python IDE

Get your editor configured for building ML pipelines with the Michelangelo SDK. This takes about 5 minutes if you've already run `poetry install`.

## Prerequisites

Before configuring your IDE, make sure you've installed the Python dependencies:

```bash
cd <repo-root>/python
poetry install
```

This creates a `.venv` directory with all Michelangelo packages. Your IDE needs to use this environment for autocomplete and import resolution to work.

> **Tip**: Replace `<repo-root>` with the path where you cloned the Michelangelo repository (e.g., `~/michelangelo`).

---

## VS Code / Cursor

1. Install the [Python extension](https://marketplace.visualstudio.com/items?itemName=ms-python.python).
2. Open the `python/` directory from the repository root.
3. Select the Poetry virtual environment as your Python interpreter:
   - Press `Cmd+Shift+P` (macOS) or `Ctrl+Shift+P` (Linux/Windows)
   - Type **"Python: Select Interpreter"**
   - Choose the `.venv` environment created by `poetry install`
4. Verify autocomplete works by opening a Python file and typing `import michelangelo`.

---

## PyCharm

1. Open the `python/` directory as your project root.
2. Go to **Settings > Project > Python Interpreter**.
3. Click the gear icon and select **Add Interpreter > Existing**.
4. Point to the Poetry environment (usually at `python/.venv/bin/python`).
5. PyCharm should automatically detect imports and provide autocomplete for the Michelangelo SDK.

---

## Verifying Your Setup

After configuring your editor, confirm everything works by checking that you get autocomplete for these imports:

```python
import michelangelo.uniflow.core as uniflow

@uniflow.task()
def hello():
    print("IDE setup working!")
```

If your editor can't resolve the `michelangelo` import:

1. Make sure you've run `poetry install` in the `python/` directory
2. Confirm your IDE is using the `.venv` Python interpreter (not your system Python)
3. Try reloading the IDE window

---

## What's next?

- **Ready to build?** [Set up your local sandbox](./sandbox-setup.md) and follow [Getting Started with Pipelines](../user-guides/ml-pipelines/getting-started.md)
- **Contributing to Michelangelo's Go backend?** See [Go and Bazel Development Setup](../contributing/dev-environment.md)
