#!/usr/bin/env python3
"""Check the stale-issue workflows against each other, repo labels, and guarantees."""

import argparse
import json
import re
import sys

import yaml

# Allowlist options take user/milestone names; `true` there silently matches nothing.
ALLOWLIST_TO_BOOLEAN = {
    f"exempt-{scope}{kind}": f"exempt-all-{scope}{kind}"
    for scope in ("", "issue-", "pr-")
    for kind in ("assignees", "milestones")
}


def label_set(value):
    """Split a comma-separated label list, lowercased like actions/stale does."""
    parts = str(value or "").split(",")
    return {part.strip().lower() for part in parts if part.strip()}


def check(stale_wf, unstale_wf, unstale_text, repo_labels):
    """Return a list of problems with the two workflows; empty means OK."""
    errors = []
    step = next(
        s
        for s in stale_wf["jobs"]["stale"]["steps"]
        if s.get("uses", "").startswith("actions/stale@")
    )
    cfg = step["with"]

    stale_exempt = label_set(cfg.get("exempt-issue-labels"))
    match = re.search(r"fromJSON\('(\[.*?\])'\)", unstale_wf["jobs"]["unstale"]["if"])
    unstale_exempt = set()
    if match:
        unstale_exempt = {name.lower() for name in json.loads(match.group(1))}
    if stale_exempt != unstale_exempt:
        errors.append(
            "exempt label lists differ between workflows: "
            f"only in stale workflow {sorted(stale_exempt - unstale_exempt)}, "
            f"only in unstale workflow {sorted(unstale_exempt - stale_exempt)}"
        )
    for name in sorted((stale_exempt | unstale_exempt) - repo_labels):
        errors.append(f"exempt label '{name}' does not exist in this repo")

    stale_label = str(cfg.get("stale-issue-label", "Stale"))
    if f"'{stale_label}'".lower() not in unstale_text.lower():
        errors.append(
            f"unstale workflow does not reference the stale label '{stale_label}'"
        )

    for key, boolean_key in ALLOWLIST_TO_BOOLEAN.items():
        if str(cfg.get(key, "")).strip().lower() in ("true", "false"):
            errors.append(
                f"{key} takes a list of names, not a boolean; use {boolean_key}"
            )

    days = cfg.get("days-before-issue-stale", cfg.get("days-before-stale"))
    said = re.search(r"(\d+)\s+days", str(cfg.get("stale-issue-message", "")))
    if said and days is not None and int(said.group(1)) != int(days):
        errors.append(
            f"stale comment says {said.group(1)} days "
            f"but days-before-issue-stale is {days}"
        )

    # Missing values matter: days-before-close defaults to 7, which closes issues.
    if cfg.get("days-before-issue-close", cfg.get("days-before-close")) != -1:
        errors.append(
            "guarantee 'never closes an issue' broken: "
            "days-before-issue-close must be -1"
        )
    if cfg.get("days-before-pr-stale") != -1 or cfg.get("days-before-pr-close") != -1:
        errors.append(
            "guarantee 'never touches a PR' broken: "
            "days-before-pr-stale and days-before-pr-close must be -1"
        )
    if stale_wf.get("permissions", {}).get("pull-requests") != "none":
        errors.append(
            "guarantee 'never touches a PR' broken: "
            "permissions.pull-requests must be none"
        )
    if not (stale_wf.get("concurrency") or {}).get("group"):
        errors.append(
            "guarantee 'comments at most once' broken: "
            "stale workflow needs a concurrency group"
        )
    return errors


def main():
    """Run the check and exit non-zero on any problem."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stale", default=".github/workflows/stale-issues.yml")
    parser.add_argument(
        "--unstale", default=".github/workflows/stale-issues-unstale-on-exempt.yml"
    )
    parser.add_argument(
        "--labels", required=True, help="JSON from `gh label list --json name`"
    )
    args = parser.parse_args()

    with open(args.stale) as f:
        stale_wf = yaml.safe_load(f)
    with open(args.unstale) as f:
        unstale_text = f.read()
    with open(args.labels) as f:
        repo_labels = {entry["name"].lower() for entry in json.load(f)}

    errors = check(stale_wf, yaml.safe_load(unstale_text), unstale_text, repo_labels)
    for error in errors:
        print(f"::error::{error}")
    if not errors:
        print("stale-issue config OK")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
