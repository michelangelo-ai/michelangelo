---
name: doc-review-team
description: Create a 3-agent team (engineer, tech-writer, product-manager) to review and improve docs for open-source quality. Use when reviewing any docs/ directory or specific doc files.
argument-hint: [docs-path]
user-invocable: true
---

Create a 3-agent team to review and improve documentation at `$ARGUMENTS` for open-source quality standards.

## Team Structure

Spawn three teammates in parallel:

### Engineer (subagent_type: Explore)
Audits the codebase against every claim in the docs. Reports CORRECT / OUTDATED / MISSING per section. Covers:
- CLI commands and flags (verify against actual CLI code)
- Code examples (verify imports, class names, API methods)
- Architecture descriptions (verify against Go/Python implementation)
- File paths referenced in docs (verify they exist)
- Enum values, config fields, YAML schemas (verify against proto/source)

### Product Manager (subagent_type: Explore)
Defines product positioning and audience. Covers:
- Target audience: who reads this, what do they already know?
- Ideal user journey through the docs
- Three-audience model: getting-started (end users) / operator-guides (platform operators) / contributing (code contributors)
- Open-source gaps: what feels internal/corporate vs welcoming?
- Scope concerns: does any content belong in a different section?
- Missing content: what would a new user need that isn't here?

### Tech Writer (subagent_type: general-purpose)
Starts quality assessment immediately without waiting for teammates. Then writes improvements to disk. Assesses:
1. Audience clarity — clear who this is for and what they'll achieve?
2. Tone — warm, friendly, open-source welcoming? Or internal/corporate?
3. Structure — logical flow for a first-time reader?
4. Completeness — prerequisites stated? Next steps linked? Troubleshooting?
5. Code examples — present, complete, accurate-looking?
6. OSS quality bar — would this pass review on Kubernetes, Airflow, or Ray?

Key quality rules:
- Warm, friendly, approachable tone (never internal/corporate)
- No internal file paths, usernames, or company-specific references
- Prerequisites clearly stated at top of each guide
- Each guide ends with clear "Next steps" links
- Code examples verified against actual source code

## Workflow

1. All three start simultaneously
2. Engineer and PM send findings to tech-writer
3. Team lead forwards key findings to tech-writer with actionable summaries
4. Tech-writer writes improvements to disk
5. Run broken link check after all files are written

## Broken Link Check

After files are written, run an Explore agent to check all internal links:
- Extract all relative links (./foo.md, ../bar/baz.md) from every .md file
- Check each target file exists on disk
- Ignore links inside code blocks
- Report: file, line, broken link, suggested fix
