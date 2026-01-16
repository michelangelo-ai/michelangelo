---
name: update-docs
description: Update Michelangelo documentation. Use when adding, modifying, or fixing docs in the docs/ folder. Ensures consistent formatting, proper titles, and valid links.
---

# Michelangelo Documentation Updater

You are updating the Michelangelo documentation site (Docusaurus v3 + Bun).

## Documentation Structure

```
docs/
├── intro.md                    # Landing page (/)
├── images/                     # Shared images
├── about/                      # Platform overview
├── user-guides/                # End-user tutorials
│   └── ml-pipelines/           # ML pipeline guides
├── operator-guides/            # Platform operator docs
│   ├── jobs/                   # Job system docs
│   └── ui/                     # UI docs
│       └── configuration/      # UI config reference
├── setup-guide/                # Installation guides
└── contributing/               # Developer guides
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
- `# 1. Introduction` (numbered - only use for first heading)
- `# overview / Introduction` (template artifact)

**Numbered sections are OK** for tutorial steps after the title:
```markdown
# My Tutorial

Introduction paragraph...

## 1. First Step
## 2. Second Step
```

## Adding New Pages

1. Create file in appropriate folder with lowercase-kebab name
2. Add `# Title` as first line
3. Page auto-appears in sidebar

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

## Style Guidelines

- Use **bold** for UI elements and emphasis
- Use `backticks` for code, commands, filenames
- Use tables for feature comparisons
- Keep paragraphs short (3-5 sentences max)
- Use bullet lists for features, numbered lists for steps

## Reviewing Documentation

When reviewing docs someone else wrote, check for:

1. **Title** - Is it descriptive? Not generic like "Introduction" or "Overview"?
2. **Filename** - Is it lowercase-kebab-case?
3. **Structure** - Does it have a logical flow? Intro → Details → Examples?
4. **Code examples** - Are they complete and runnable?
5. **Links** - Do internal links use correct relative paths?
6. **Images** - Are they in `docs/images/` with relative paths?

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
