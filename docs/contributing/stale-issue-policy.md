---
last_verified: 2026-11-04
---

# Stale Issue Policy

Michelangelo AI uses a bot to flag open issues that have gone quiet. A maintainer then decides whether to close, keep, or freeze each one. The bot itself only adds a label and a comment.

:::note
The bot never closes an issue and never touches pull requests. Closing an issue is always a person's decision.
:::

## How It Works

Once a day at 06:17 UTC, the [`stale-issues.yml`](https://github.com/michelangelo-ai/michelangelo/blob/main/.github/workflows/stale-issues.yml) workflow checks every open issue, oldest first. An issue with no activity for **180 days** gets the `stale` label and one comment explaining what happens next. The bot doesn't comment again while the label stays on.

GitHub sometimes starts scheduled workflows late or skips a run. The next run catches up.

## What Counts as Activity

Activity restarts an issue's 180-day clock. If the issue is already labeled `stale`, activity also removes the label at the next daily run.

| Action | Counts as activity? |
|--------|---------------------|
| A comment | Yes |
| Adding or removing a label | Yes |
| Adding an exempt label or a milestone | Yes, and the `stale` label comes off within seconds |
| Editing the issue description | No |
| A reaction (👍 and so on) | No |

## Exemptions

The bot never flags an issue that has any of these:

| Exemption | Why |
|-----------|-----|
| `frozen` | A maintainer decided the issue stays open without periodic review |
| `tech-debt` | Technical debt is long-lived by nature |
| `todo-tracking` | The issue tracks a `TODO(#123)` comment in the code, so it stays open while the TODO does |
| A milestone | The work is planned |

Assigned issues are not exempt. An assignee who has moved on is exactly the case the bot should surface, and the comment doubles as a reminder to the assignee.

Exemptions are permanent until a maintainer removes them. Nothing re-reviews exempt issues on a schedule.

## If Your Issue Is Labeled Stale

If the issue is still relevant, leave a comment with its current status: what's blocking it, or that you still need it. The label comes off at the next daily check. Reactions and description edits don't count.

If the issue is no longer needed, close it.

## Weekly Triage

A maintainer reviews [open `stale` issues](https://github.com/michelangelo-ai/michelangelo/issues?q=is%3Aissue%20is%3Aopen%20label%3Astale) every week and does one of three things with each:

- **Close** it with a one-line reason: fixed, out of scope, superseded, or no response.
- **Keep** it with a comment saying why it still matters. The comment clears the label and restarts the clock. If the work is scheduled, add a milestone instead.
- **Freeze** it with `frozen` when it should stay open and out of future sweeps.

Keep these in mind:

- Adding or removing a label restarts an issue's clock. A script that relabels old issues in bulk hides every issue it touches from the bot for 180 days and clears any `stale` label on them. Label new issues as they arrive instead of backfilling old ones.
- Each untriaged `stale` issue costs the bot about 2 operations per run, out of a budget of 400, and a run that hits the budget stops without resuming. Around 200 untriaged stale issues, the newest issues would stop being checked. Granting the workflow `actions: write` would let runs resume, at the cost of a broader token. Don't grant `actions: read` alone: the bot could then read its saved progress but never replace it.
- A failed label or comment call doesn't turn the run red. Check the run's annotations now and then.

## Running the Bot Manually

Go to **Actions → Stale issues → Run workflow**. Manual runs are dry runs by default: with the dry-run box checked, the bot logs what it would do and changes nothing. Uncheck the box to run live.

## Changing the Policy

The policy lives in three workflows and one script:

| File | What it holds |
|------|---------------|
| `.github/workflows/stale-issues.yml` | The timer, the comment, and the exempt labels |
| `.github/workflows/stale-issues-unstale-on-exempt.yml` | A second copy of the exempt labels, used to remove `stale` when an issue is exempted |
| `.github/workflows/stale-issues-config-check.yml` | Runs the config check |
| `.github/scripts/check_stale_config.py` | The config check |

The config check runs on every pull request that touches these files, and daily at 06:07 UTC. It fails if:

- The two workflows' exempt label lists differ.
- An exempt label doesn't exist in the repo, for example after a rename in repo settings.
- The two workflows refer to different `stale` labels.
- An option that takes names, such as `exempt-issue-milestones`, is given `true`.
- The comment's day count doesn't match `days-before-issue-stale`.
- A change would let the bot close issues, flag issues that have a milestone, touch pull requests, or post twice from overlapping runs.

To add an exempt label, create it in the repo first, then add it to both workflows and to the table above. To change the timer, also update the number in the bot's comment, on this page, and in `CONTRIBUTING.md`.

Renaming an exempt label in repo settings turns off its exemption until both workflows use the new name: the next daily run can flag every quiet issue that carries it, and the config check reports the problem but doesn't stop the bot. Rename the label, then merge the same rename in both workflows before the next 06:17 UTC run.

Both scheduled workflows run only in `michelangelo-ai/michelangelo`. If the repository is renamed or moves to another organization, update the `if:` line in `stale-issues.yml` and `stale-issues-config-check.yml`; otherwise both skip every run without an error.

To run the check locally (requires `pyyaml`):

```bash
gh label list --repo michelangelo-ai/michelangelo --limit 1000 --json name > /tmp/labels.json
python .github/scripts/check_stale_config.py --labels /tmp/labels.json
```
