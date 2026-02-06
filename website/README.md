# Michelangelo Documentation

This is the documentation website for Michelangelo, built with [Docusaurus](https://docusaurus.io/).

## Prerequisites

- Node.js >= 20.0
- [Bun](https://bun.sh/) (recommended) or npm/yarn

## Getting Started

First, navigate to the website directory:

```bash
cd website
```

### Install dependencies

```bash
bun install
```

### Start the development server

```bash
bun run start
```

This starts a local development server at http://localhost:3003. Most changes are reflected live without needing to restart the server.

### Build for production

```bash
bun run build
```

This generates static content in the `build` directory that can be deployed to any static hosting service.

### Preview the production build

```bash
bun run serve
```

## Writing Documentation

### Where to add docs

All documentation lives in the `docs/` folder at the repository root (not inside `website/`).

```
michelangelo/
├── docs/           # Documentation files go here
│   ├── intro.md
│   ├── getting-started/
│   │   ├── installation.md
│   │   └── quickstart.md
│   └── guides/
│       └── ...
└── website/        # Docusaurus site config (you are here)
```

### Creating a new doc

1. Create a Markdown file in `docs/` (e.g., `docs/my-new-doc.md`)
2. Add frontmatter at the top:

```md
---
title: My New Doc
sidebar_position: 1
---

# My New Doc

Your content here...
```

### Frontmatter options

| Field | Description |
|-------|-------------|
| `title` | Page title (shown in sidebar and browser tab) |
| `sidebar_position` | Order in the sidebar (lower = higher up) |
| `sidebar_label` | Custom label for sidebar (defaults to `title`) |
| `description` | SEO description for the page |
| `slug` | Custom URL path (e.g., `/custom-path`) |

### Organizing docs

The sidebar is **auto-generated** from the folder structure. To organize:

- Create folders for categories (e.g., `docs/guides/`)
- Add a `_category_.json` file to customize the category:

```json
{
  "label": "Guides",
  "position": 2
}
```

### Linking between docs

Use relative paths to link to other docs:

```md
See the [installation guide](./getting-started/installation.md) for more details.
```

### Adding images

Place images in `website/static/img/` and reference them with absolute paths:

```md
![Architecture diagram](/img/architecture.png)
```

### Code blocks

Syntax highlighting is available for: `go`, `python`, `bash`, `yaml`, `json`, `javascript`, `typescript`, and more.

````md
```python
def hello():
    print("Hello, Michelangelo!")
```
````

## Troubleshooting

If the site isn't reflecting your changes, clear the cache and restart:

```bash
bun run clear
bun run start
```

## Other Commands

| Command | Description |
|---------|-------------|
| `bun run clear` | Clear the Docusaurus cache |
| `bun run typecheck` | Run TypeScript type checking |

## Learn More

- [Docusaurus Documentation](https://docusaurus.io/docs)
- [Markdown Features](https://docusaurus.io/docs/markdown-features)
