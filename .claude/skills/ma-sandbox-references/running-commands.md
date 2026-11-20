# Running sandbox commands without approval prompts

Each `ma-sandbox-*` skill lists the scripts it runs in its `allowed-tools`
frontmatter. Those entries only apply once the skill is invoked, and only to
commands that match them.

- **Invoke the skill first.** Call the skill (`/ma-sandbox-deploy`, etc.)
  before running its scripts. A script run before then gets judged on its
  content and may prompt.
- **Use the literal form.** Write `bash .claude/skills/ma-sandbox-scripts/<script>.sh <args>`
  from the repo root. Wrappers such as `$(git rev-parse --show-toplevel)/...`,
  `cd ... &&`, `source ...` or `VAR=$(...)` don't match the allow rules.
- **One command per Bash call.** Chained calls (`&&`, `;`, pipes into other
  tools) are judged as a whole.
- **Pass checkouts with flags.** To target another checkout, use the script's
  `--repo <path>` flag instead of changing directory.
- **Denied means stop.** Don't reshape a denied command into another form.
  Record the command and the denial text, and report it.
