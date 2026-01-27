# Documentation Guide

This guide explains how to contribute to Michelangelo's documentation site.

## Quick Start

```bash
cd website
bun install
bun run start    # Dev server at http://localhost:3003/
```

Create a new page:
1. Add `docs/user-guides/my-guide.md` (lowercase-kebab-case)
2. Start with `# Descriptive Title`
3. Submit a PR

## Running Locally

### Prerequisites

- [Bun](https://bun.sh/) - Install with `curl -fsSL https://bun.sh/install | bash`

### Commands

```bash
cd website

# Install dependencies
bun install

# Start dev server (hot reload)
bun run start

# Build for production (catches broken links)
bun run build

# Preview production build
bun run serve
```

## File Structure

```
docs/
├── intro.md                    # Landing page (/)
├── images/                     # Shared images
├── about/                      # Platform overview
├── contributing/               # Developer guides
├── dev/                        # Development docs
│   └── go/                     # Go development
├── operator-guides/            # Platform operator docs
│   ├── jobs/                   # Job system docs
│   └── ui/                     # UI docs
│       └── configuration/      # UI config reference
├── setup-guide/                # Installation guides
└── user-guides/                # End-user tutorials
    └── ml-pipelines/           # ML pipeline guides
```

## Adding Content

### File Naming

Use **lowercase-kebab-case** for all filenames:
- `my-new-guide.md` (creates URL `/user-guides/my-new-guide`)
- Never use spaces, underscores, or capital letters

### Page Titles

Every page must start with a descriptive `# Title`:

```markdown
# Deploying Models to Production

Content starts here...
```

**Avoid generic titles:**
- `# Introduction` - too vague
- `# Overview` - too vague
- `# 1. Introduction` - save numbers for sub-sections

**Numbered sub-sections are fine** for tutorials:
```markdown
# Deploying Models

Introduction paragraph...

## 1. Prepare Your Model
## 2. Configure the Pipeline
## 3. Deploy and Monitor
```

### Frontmatter

Control sidebar order and display with YAML frontmatter:

```markdown
---
sidebar_position: 2
sidebar_label: "Short Label"
---

# Full Page Title

Content...
```

| Field | Purpose |
|-------|---------|
| `sidebar_position` | Order in sidebar (1, 2, 3...) |
| `sidebar_label` | Shorter name for sidebar |
| `slug` | Custom URL path |

### Adding a New Section

1. Create a folder: `docs/new-section/`
2. Add a `_category_.json` file:
   ```json
   {
     "label": "New Section",
     "position": 5,
     "collapsed": false
   }
   ```
3. Add markdown files to the folder

### Images

Place images in `docs/images/` and reference them with relative paths:

```markdown
![Alt text](../images/my-image.png)
```

For section-specific images, you can also co-locate them:

```
docs/
├── images/                     # Shared images
│   └── architecture.png
├── user-guides/
│   └── images/                 # Section-specific images
│       └── workflow-diagram.png
```

Reference co-located images:
```markdown
![Workflow](./images/workflow-diagram.png)
```

### Links

Use relative paths with `.md` extension for internal links:

```markdown
[See the CLI guide](./cli.md)
[Back to setup](../setup-guide/installation.md)
```

- Always include the `.md` extension (Docusaurus converts them)
- Relative paths work in both GitHub and the built site
- Avoid absolute paths like `/docs/page`

### Admonitions

Use callout blocks to highlight important information:

```markdown
:::note
Helpful background information.
:::

:::tip
Suggestions to help the reader succeed.
:::

:::info
Additional context or details.
:::

:::warning
Potential issues or gotchas to watch out for.
:::

:::danger
Critical warnings about destructive actions.
:::
```

## Style Guidelines

- Use **bold** for UI elements and emphasis
- Use `backticks` for code, commands, filenames, and paths
- Use tables for feature comparisons
- Keep paragraphs short (3-5 sentences max)
- Use bullet lists for features, numbered lists for sequential steps
- Use admonitions for notes, tips, and warnings

## Using AI

AI tools can help write and maintain documentation. If using [Claude Code](https://github.com/anthropics/claude-code), run `/update-docs` to load the project's documentation guidelines.

### Example Prompts

```
"Add a new guide for model deployment in docs/user-guides/"

"Fix any broken internal links in docs/operator-guides/"

"Add frontmatter to pages missing sidebar_position"
```

### Tips

- **Review output** - Verify generated content for technical accuracy
- **Test locally** - Run `bun run build` before committing
- **Use the PR workflow** - AI changes go through normal code review

## Before Submitting

Before opening a PR, verify your changes:

```bash
cd website && bun run build
```

This catches:
- Broken internal links
- Invalid markdown syntax
- Missing referenced files

**Checklist:**
- [ ] File uses lowercase-kebab-case naming
- [ ] Page has a descriptive `# Title` (not generic)
- [ ] Internal links use relative paths with `.md` extension
- [ ] Images are in `docs/images/` or co-located
- [ ] `bun run build` passes without errors

## Submitting Changes

1. Fork the repository (external contributors) or create a branch (maintainers)
2. Make your documentation changes
3. Run `bun run build` to verify
4. Open a pull request against `main`
5. Documentation deploys automatically after merge

## Deployment

Documentation deploys automatically when changes to `docs/` or `website/` are pushed to `main`.

### Automatic Deployment

The GitHub Actions workflow (`.github/workflows/deploy-docs.yml`):
1. Installs Bun and dependencies
2. Builds the static site
3. Deploys to GitHub Pages

### Manual Deployment

Trigger a deploy manually: GitHub Actions → "Deploy Docs" → "Run workflow"

### Checking Status

- **Build status**: Check the Actions tab in GitHub
- **Live site**: https://michelangelo-ai.github.io/
