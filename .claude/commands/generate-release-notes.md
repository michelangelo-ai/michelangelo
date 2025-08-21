---
allowed-tools: Bash(git log:*), Bash(git show:*), Bash(git diff:*), Bash(date:*), Read, Write, Glob, Grep
argument-hint: [date_range] (e.g., "last week", "since last monday", "2025-08-06 to 2025-08-20", "this month")
description: Generate release notes for date range and save to files
---

# Release Notes Generator

Generate concise, user-focused release notes for the Michelangelo ML Platform for {{date_range}}.

## Date Range Processing

First, convert the date range "{{date_range}}" to specific dates:

- If exact dates provided (YYYY-MM-DD format), use those
- If natural language like "last week", "since last monday", "this month", etc., calculate the actual dates
- Default to "this month" if no range specified

Use `date` command to help with calculations:

```bash
# Examples of date calculations:
date -d "last monday" +%Y-%m-%d     # Start of last week
date -d "1 week ago" +%Y-%m-%d      # One week ago
date -d "1 month ago" +%Y-%m-%d     # One month ago
date +%Y-%m-%d                      # Today
```

## Methodology

1. **Analyze git commit history** using the calculated date range:

   ```bash
   git log --since="<start_date>" --until="<end_date>" --pretty=format:"%h %ad %s" --date=short --no-merges
   ```

2. **Examine commit message and overview of changed files** for each commit using `git show --name-only`

2a. For more context, examine actual file changes using `git show`

3. **Group related commits** into cohesive features based on:
   - Related file paths (same component/subsystem)
   - Temporal proximity and author patterns
   - Semantic relationships between commits
   - Cross-component features spanning multiple directories

## Repository Structure (Michelangelo ML Platform)

- **Frontend**: `javascript/` - Michelangelo's UI, React/TypeScript UI components
- **Backend Services**: `go/` - Microservices, controllers, workers, APIs
- **Python SDK**: `python/` - CLI tools, SDK, uniflow pipeline system
- **Proto APIs**: `proto/` - Protobuf definitions for service communication
- **Infrastructure**: `.github/`, `docker/`, `bazel/` - Build, CI/CD, containerization

## Categorization Guidelines

- **Major Features**: User-facing functionality with significant impact
- **Infrastructure & Platform**: Backend services, APIs, deployment changes
- **Developer Experience**: Tooling, SDK improvements, development workflows
- **Bug Fixes**: Critical corrections to existing functionality (exclude fixes for functionality introduced within the same release period)

## Content Guidelines

**Include:**
- User-facing functionality changes
- New capabilities and features
- Infrastructure improvements that enable new functionality
- Developer tooling that improves workflow

**Exclude:**
- Code formatting and linting fixes
- Import organization and code style changes
- Internal refactoring without user impact
- Minor maintenance tasks
- Routine dependency updates
- Bug fixes for functionality introduced within the same release period

## Output Format

Structure the release notes as markdown with these sections:

```markdown
# Michelangelo Release Notes

## <start_date> to <end_date>

### 🚀 Major Features

#### **Feature Name** `[Component Tags]`
Concise description focusing on user capabilities. Group related functionality together (e.g., "Tables now include pagination, column controls, sorting"). _(commit-hash, commit-hash)_

### 🏗️ Infrastructure & Platform

#### **System Name** `[Component Tags]`
Brief description of backend improvements and new capabilities. _(commit-hash)_

### 🛠️ Developer Experience

#### **Tool/SDK Name** `[Component Tags]`
- Key improvement 1 _(commit-hash)_
- Key improvement 2 _(commit-hash)_

---

**Breaking Changes**: [None | List changes]
**Migration Notes**: [All changes maintain backward compatibility | Specific migration steps]
**Components Updated**: [List major components]
```

## File Output Strategy

After generating the release notes, save them to standardized locations:

### 1. Create/Update CHANGELOG.md

- **Format**: Follow [Keep a Changelog](https://keepachangelog.com/) standard
- **Location**: `/CHANGELOG.md` (repo root)
- **Content**: Concise version with date and major changes only

### 2. Create Detailed Release Notes

- **Location**: `/docs/releases/YYYY-MM.md` (e.g., `/docs/releases/2025-08.md`)
- **Content**: Full detailed release notes with all sections
- **Create directories**: `mkdir -p docs/releases` if needed

### 3. Suggested CHANGELOG.md Entry Format

```markdown
## [YYYY-MM-DD] - <start_date> to <end_date>

### Added

- Major feature highlights (2-3 key items)

### Changed

- Significant improvements (2-3 key items)

### Infrastructure

- Backend/platform changes (1-2 key items)

[Full release notes](./docs/releases/YYYY-MM.md)
```

## Best Practices

- **Be concise**: Group related features together (e.g., "API now supports authentication, rate limiting, and request validation" instead of separate bullets for each)
- Focus on **user impact**, not technical implementation details
- Use **present tense** ("Adds support for..." not "Added support for...")
- **Avoid persuasive language** - this is factual documentation
- **Group by functionality** rather than file structure
- **Exclude maintenance tasks**: Skip code formatting, linting fixes, import organization
- **Always highlight breaking changes** upfront
- **Include relevant commit hashes** for traceability
- **Provide migration guidance** when needed
