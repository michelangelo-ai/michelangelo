# Set Up Go Server for VS Code / Cursor

1. Make sure [golang](https://go.dev/doc/install) is installed and setup correctly. 
2. Install [gopls](https://github.com/golang/tools/blob/master/gopls/doc/index.md).
3. Open the michelangelo root folder in VSCode / Cursor.
4. When you open a go file, you should see the notification - `Setting up workspace: Loading packages...`. If there are no errors, your language server's capabilities should work correctly. 

## GoLand / IntelliJ

The vendor directory method also works for GoLand. But the IntelliJ bazel plugin is more convenient.

In GoLand -> Settings ... -> Plugins, install "Bazel for IntelliJ"

With this plugin, GoLand will be able to index dependency libraries and proto-generated go files automatically.

Use the workspace root directory (not /go directory) as the GoLand project root.

Remember to click the green Bazel logo at the right top corner of GoLand to sync bazel, when bazel files are changed (e.g. changed by gazelle).


## Set C++ compiler for bazel in MacOS

If bazel command fails on your mac with C++ build errors, you may have to set CC and CXX environment variables for bazel.

Add following lines in your .zshrc file:
```bash
export CC=clang
export CXX=clang++
```

## Use commands in tools directory
 
GoLand doesn't run .envrc. To make it use the command line tools in tools directory, we have to create a bazel wrapper script:
```bash
#!/usr/bin/env bash

# GoLand always call bazel in project root directory.
export PATH=${PWD}/tools:${PATH}

bazel "$@"
```

Give the script execution permission ('chmod +x')

In GoLand->Settings...->Other Settings->Bazel Settings, set bazel binary location to your wrapper script.
