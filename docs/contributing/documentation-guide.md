# Documentation Guide

This guide explains how to contribute to Michelangelo's documentation site.

## Why We Built This

GitHub wikis are convenient for quick notes, but they don't scale well for comprehensive platform documentation:

- **No code review** - Wiki edits bypass PR review, making it easy for errors to slip in
- **No local development** - Can't run docs on your machine or test in full context
- **Limited search** - Finding information becomes harder as docs grow
- **No versioning** - Can't maintain docs for different releases
- **Siloed from code** - Docs live separately from the codebase they describe

This Docusaurus site solves these problems by treating documentation as code:

| Feature | Wiki | Docusaurus Site |
|---------|------|-----------------|
| Search | Basic | Full-text search with highlights |
| Versioning | None | Git-based, can tag releases |
| Local development | No | Yes (`bun run start`) |
| CI/CD | No | Auto-deploy on merge to main |
| Code review | No | Full PR workflow |
| Broken link detection | No | Build fails on broken links |
| Offline access | No | Static files, works offline |

The site is built with **Docusaurus v3** and **Bun**, deployed automatically to GitHub Pages on every push to `main`.

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

# Build for production
bun run build

# Preview production build
bun run serve
```

The dev server runs at `http://localhost:3003/michelangelo/`

## Updating Documentation

### File Structure

```
docs/
├── intro.md                    # Landing page (/)
├── about/                      # Platform overview
├── user-guides/                # End-user tutorials
│   └── ml-pipelines/           # Nested section
├── operator-guides/            # Platform operators
├── setup-guide/                # Installation guides
└── contributing/               # Developer guides
```

### Adding a New Page

1. Create a markdown file in the appropriate folder:
   ```bash
   docs/user-guides/my-new-guide.md
   ```

2. Add a title as the first heading:
   ```markdown
   # My New Guide

   Content goes here...
   ```

3. The page is automatically added to the sidebar

### File Naming

- Use **lowercase-kebab-case**: `my-new-guide.md`
- This creates clean URLs: `/user-guides/my-new-guide`

### Frontmatter (Optional)

Control page behavior with YAML frontmatter:

```markdown
---
sidebar_position: 2
title: Custom Sidebar Title
---

# My Page Title

Content...
```

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

## Using AI to Update Docs

AI assistants like Claude can help maintain documentation:

### What AI Can Help With

- **Writing new docs** - Describe what you need, AI generates the content
- **Fixing formatting** - Standardize headings, fix markdown issues
- **Updating code examples** - Keep code snippets current
- **Improving clarity** - Rewrite confusing sections
- **Bulk operations** - Rename files, update links, restructure sections

### Example Prompts

```
"Add a new guide for deploying models to production in docs/user-guides/"

"Update all code examples in docs/setup-guide/ to use the new CLI syntax"

"Review docs/operator-guides/ and fix any broken internal links"

"Restructure docs/user-guides/ml-pipelines/ to have a clearer hierarchy"
```

### Best Practices

1. **Review AI output** - Always verify generated content for accuracy
2. **Test locally** - Run `bun run build` to catch broken links
3. **Use PR workflow** - AI changes go through normal code review
4. **Provide context** - Share existing docs so AI matches the style

## Deployment

Documentation deploys automatically when changes to `docs/` or `website/` are pushed to `main`.

### Automatic Deployment

The GitHub Actions workflow (`.github/workflows/deploy-docs.yml`):
1. Installs Bun and dependencies
2. Builds the static site
3. Deploys to GitHub Pages

### Manual Deployment

Trigger a deploy manually from GitHub Actions → "Deploy Docs (Bun)" → "Run workflow"

### Checking Status

- **Build status**: Check the Actions tab in GitHub
- **Live site**: https://michelangelo-ai.github.io/michelangelo/
