# Go and Bazel Development Setup

This guide is for developers contributing to Michelangelo's Go backend services (API server, controller manager, worker). If you're building ML pipelines with the Python SDK, see [Python IDE Setup](../getting-started/setup-python-ide.md) instead.

---

## Go IDE Setup

### VS Code / Cursor

1. Install [Go](https://go.dev/doc/install) and verify with `go version`.
2. Install [gopls](https://github.com/golang/tools/blob/master/gopls/doc/index.md) (the Go language server).
3. Open the michelangelo root folder in VS Code / Cursor.
4. When you open a `.go` file, you should see "Setting up workspace: Loading packages..." in the status bar. Once complete, autocomplete, go-to-definition, and other language server features will work.

### GoLand / IntelliJ

1. Install the **Bazel for IntelliJ** plugin: GoLand > Settings > Plugins > search "Bazel for IntelliJ".
2. Open the **workspace root directory** (not the `/go` directory) as your project root.
3. The plugin indexes dependency libraries and proto-generated Go files automatically.
4. After changing Bazel files (e.g., after running Gazelle), click the green Bazel icon in the top-right corner to re-sync.

---

## Bazel Configuration

### macOS C++ Compiler

If Bazel fails with C++ build errors on macOS, add these lines to your `~/.zshrc`:

```bash
export CC=clang
export CXX=clang++
```

Then restart your terminal or run `source ~/.zshrc`.

### GoLand Bazel Wrapper

GoLand doesn't automatically load `.envrc` files. To make it use the command-line tools in the `tools/` directory, create a wrapper script:

```bash
#!/usr/bin/env bash
# GoLand always calls bazel from the project root directory.
export PATH=${PWD}/tools:${PATH}
bazel "$@"
```

Save this script (e.g., as `~/bin/bazel-goland.sh`), make it executable with `chmod +x`, and set it as the Bazel binary in GoLand > Settings > Other Settings > Bazel Settings.
