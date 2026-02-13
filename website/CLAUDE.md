# CLAUDE.md - Website Guidelines

## Package Manager

**Use `bun`, not npm or yarn.**

| Task | Command |
|------|---------|
| Install | `bun install` |
| Dev server | `bun run start` |
| Build | `bun run build` |
| Type check | `bun run typecheck` |
| Clear cache | `bun run clear` |

## Docusaurus Structure

- Documentation source: `docs/` at repo root (not inside `website/`)
- Theme customizations: `src/theme/` (swizzled components)
- Static assets: `static/img/`
