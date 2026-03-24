---
name: update-docs
description: Update Michelangelo documentation. Use when adding, modifying, or fixing docs in the docs/ folder. Ensures consistent formatting, proper titles, and valid links.
user-invocable: true
---

# Michelangelo Documentation Updater

You are updating the Michelangelo documentation site (Docusaurus v3 + Bun).

## Documentation Structure

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

## File Naming

- **Always use lowercase-kebab-case**: `my-new-guide.md`
- This creates clean URLs: `/user-guides/my-new-guide`
- Never use spaces, underscores, or capital letters in filenames

## Page Titles

Every page MUST start with a proper `# Title`:

```markdown
# Descriptive Page Title

Content starts here...
```

**Bad titles to avoid:**
- `# Introduction` (too generic)
- `# Overview` (too generic)
- `# 1. Introduction` (numbered page titles are bad - save numbers for sub-sections)
- `# overview / Introduction` (template artifact)

**Numbered sub-sections are OK** for tutorial steps after the title:
```markdown
# My Tutorial

Introduction paragraph...

## 1. First Step
## 2. Second Step
```

## Page Frontmatter

Use YAML frontmatter at the top of pages to control sidebar order and display:

```markdown
---
sidebar_position: 1
sidebar_label: "Short Label"
slug: /custom-url
---

# Full Page Title

Content...
```

| Field | Purpose |
|-------|---------|
| `sidebar_position` | Order in sidebar (1, 2, 3...) |
| `sidebar_label` | Shorter name for sidebar |
| `slug` | Custom URL path |
| `title` | SEO/browser tab title (defaults to `# heading`) |

## Adding New Pages

1. Create file in appropriate folder with lowercase-kebab name
2. Add frontmatter if you need specific ordering
3. Add `# Title` as first line
4. Page auto-appears in sidebar

## Adding New Sections

Create a folder with `_category_.json`:

```json
{
  "label": "Section Name",
  "position": 5,
  "collapsed": false
}
```

## Images

Place in `docs/images/` or co-locate with docs:

```markdown
![Alt text](../images/my-image.png)
```

## Validation

After making changes, always run:

```bash
cd website && bun run build
```

This catches:
- Broken internal links
- Invalid markdown
- Missing files

## Common Fixes

### Fix missing title
```markdown
# Proper Title Here

First paragraph of content...
```

### Fix template headings
Change `# 1. overview / Introduction` to `# Actual Title`

### Fix broken links
Update paths after file renames:
```markdown
[Link text](./correct-path.md)
```

## Links

**Use relative paths** for internal links:
```markdown
[Link text](./sibling-page.md)
[Link text](../other-section/page.md)
```

- Always use `.md` extension in links (Docusaurus converts them)
- Relative paths ensure links work in both GitHub and the built site
- Avoid absolute paths like `/docs/page` unless linking from non-docs content

## Admonitions

Use Docusaurus admonitions for callouts:

```markdown
:::note
Helpful information the reader should know.
:::

:::tip
Suggestions to help the reader be more successful.
:::

:::info
Additional context or background information.
:::

:::warning
Potential issues or gotchas.
:::

:::danger
Critical information about destructive actions.
:::
```

## Style Guidelines

- Use **bold** for UI elements and emphasis
- Use `backticks` for code, commands, filenames
- Use admonitions for notes, tips, and warnings
- Use tables for feature comparisons
- Keep paragraphs short (3-5 sentences max)
- Use bullet lists for features, numbered lists for steps

## Reviewing Documentation

When reviewing docs someone else wrote, check for:

1. **Title** - Is it descriptive? Not generic like "Introduction" or "Overview"?
2. **Filename** - Is it lowercase-kebab-case?
3. **Frontmatter** - Does it have `sidebar_position` if ordering matters?
4. **Structure** - Does it have a logical flow? Intro → Details → Examples?
5. **Code examples** - Are they complete and runnable?
6. **Links** - Do internal links use correct relative paths?
7. **Images** - Are they in `docs/images/` with relative paths?

Run `cd website && bun run build` to catch broken links.

## Generating Docs from Code

When asked to document code or verify docs match code:

1. **Read the source code** first - understand what it actually does
2. **Check existing docs** - see what's already documented
3. **Compare** - identify gaps or inaccuracies
4. **Update docs** to match the code, not the other way around

### Key code locations

| Component | Code Location | Docs Location |
|-----------|---------------|---------------|
| Python SDK | `python/` | `docs/user-guides/` |
| CLI tools | `python/` | `docs/user-guides/cli.md` |
| Go API server | `go/` | `docs/operator-guides/` |
| UI components | `javascript/` | `docs/operator-guides/ui/` |

### Documentation patterns for code

**For Python functions/classes:**
```markdown
## FunctionName

Description of what it does.

**Parameters:**
- `param1` (type): Description
- `param2` (type, optional): Description. Default: `value`

**Returns:**
- (type): Description

**Example:**
\`\`\`python
result = function_name(param1, param2)
\`\`\`
```

**For CLI commands:**
```markdown
## command-name

Description of what the command does.

\`\`\`bash
ma command-name [options] <required-arg>
\`\`\`

**Options:**
| Flag | Description |
|------|-------------|
| `--flag` | What it does |

**Examples:**
\`\`\`bash
ma command-name --flag value
\`\`\`
```

**For API endpoints:**
```markdown
## EndpointName

**Request:**
\`\`\`protobuf
message RequestType {
  string field = 1;
}
\`\`\`

**Response:**
\`\`\`protobuf
message ResponseType {
  string result = 1;
}
\`\`\`
```
