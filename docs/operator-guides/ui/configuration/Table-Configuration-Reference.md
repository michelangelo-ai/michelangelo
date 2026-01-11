# overview

Table configuration controls table-level behavior (pagination, sorting, search, filters) and is shared across:

- **List views** (entity tables in list pages)
- **Detail view table pages** (tables embedded in detail views)

## TableConfig Interface

| Property            | Type               | Description                                                                    | Default                                                |
| ------------------- | ------------------ | ------------------------------------------------------------------------------ | ------------------------------------------------------ |
| `columns`           | `ColumnConfig[]`   | Array of column definitions (see [Cell Types Reference](./cell-type-reference)) | ✅ Required                                            |
| `emptyState`        | `EmptyState`       | Content to display when table has no data                                      | `{ title: 'No data', content: 'No data is present.' }` |
| `disablePagination` | `boolean`          | Show all data in single page without pagination controls                       | `false`                                                |
| `disableSorting`    | `boolean`          | Disable sorting functionality for all columns                                  | `false`                                                |
| `disableSearch`     | `boolean`          | Hide search bar in table action bar                                            | `false`                                                |
| `disableFilters`    | `boolean`          | Disable column filtering functionality                                         | `false`                                                |
| `pageSizes`         | `PageSizeOption[]` | Available page size options in dropdown                                        | `[{ id: 15, label: '15' }, { id: 25, label: '25' }, { id: 50, label: '50' }]`                                         |
| `enableStickySides` | `boolean`          | Keep first and last columns visible during horizontal scroll                   | `true`                                                 |

## Property Details

### `columns`

- Array of column configurations using cell types
- See [Cell Types Reference](./cell-type-reference) for complete column/cell configuration
- Order in array determines initial display order

### `emptyState`

Controls what displays when the table has no data.

**Properties:**
- `title` (required) - Main heading text
- `content` (optional) - Descriptive text below title
- `icon` (optional) - React component to display above title (e.g., `<Icon name="emptyBox" />`)

**When shown:**
- Table has zero rows (`data.length === 0`)
- Not in loading or error state
- All filters cleared

**Use cases:**
- Guide users to create their first entity
- Explain why the table is empty
- Provide contextual help or next steps

### `disablePagination`

- When `true`, displays all rows on single page
- No pagination controls shown
- Use for small datasets (< 50 rows) or embedded tables

### `disableSorting`

- Disables sorting for all columns
- Removes sort indicators from column headers
- Individual columns can also disable sorting via column config

### `disableSearch`

- Hides the search bar from table action bar
- Disables global filtering across all columns
- Useful for simple tables with few rows

### `disableFilters`

- Disables column-specific filtering
- Tooltips with filter actions won't work
- Search (global filter) still works if not disabled

### `pageSizes`

- Dropdown options for rows per page
- Format: `[{ id: 15, label: '15' }, { id: 25, label: '25' }]`
- First option is default page size

### `enableStickySides`

- First column and last column remain visible during horizontal scroll
- Improves UX for wide tables
- Set to `false` for narrow tables where sticky behavior isn't needed

## Source Files

**Type definitions:**
- [javascript/packages/core/components/views/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/types.ts) - `TableConfig` interface
- [javascript/packages/core/components/table/types/table-types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/table/types/table-types.ts) - `TableProps` with full defaults
- [javascript/packages/core/components/table/components/table-pagination/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/table/components/table-pagination/types.ts) - `PageSizeOption`

**Real examples:**
- [javascript/packages/core/config/entities/run/list.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/run/list.ts) - Minimal list view table
- [javascript/packages/core/config/entities/trigger/detail.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/trigger/detail.ts) - Table in detail view page

## Related Documentation

- [Cell Types Reference](./cell-type-reference) - Column configuration (the `columns` property)
- [Entity Configuration Reference](entity-configuration-reference) - How entities use list and detail views
- [Configuration API](./configuration-api) - overview of configuration system
