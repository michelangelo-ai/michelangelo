Add user config layer to mactl configuration

## Summary
Split the single `~/.ma/config.toml` into two files: `config.toml` for project defaults and `user_config.toml` for personal overrides. Previously there was no way to separate shared project configuration from individual user preferences -- every override lived in one file.

The new four-layer priority is: built-in defaults, project config, user config, environment variables. `_load_toml_config()` is generalized to `_load_toml_file(path)` so both files load through the same code path. The example config is renamed to `user_config.toml.example` and documents the new layering. A `[plugin]` section is added to the example for plugin directory and module configuration.

## Test plan
No new tests in the diff. This is a configuration layering change -- existing config loading tests should cover the merge behavior. Manual verification: copy the example to `~/.ma/user_config.toml`, set a value, and confirm it overrides `config.toml`.
