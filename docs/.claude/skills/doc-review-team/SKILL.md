---
name: doc-review-team
description: Create a 3-agent team (engineer, tech-writer, product-manager) to review and improve docs for open-source quality. Use when reviewing any docs/ directory or specific doc files.
argument-hint: [docs-path]
user-invocable: true
---

Create a 3-agent team to review and improve documentation at `$ARGUMENTS` for open-source quality standards.

## Team Structure

Spawn three teammates in parallel:

### Engineer (agent: doc-engineer)
See [doc-engineer agent definition](../agents/doc-engineer.md).

### Product Manager (agent: product-manager)
See [product-manager agent definition](../agents/product-manager.md).

### Tech Writer (agent: tech-writer)
See [tech-writer agent definition](../agents/tech-writer.md).

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
