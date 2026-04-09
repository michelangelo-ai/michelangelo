import type {
  ColumnFilter,
  ColumnVisibilityState,
  SortingState,
  TableState,
} from '#core/components/table/types/table-types';

export type UrlScope =
  | 'globalFilter'
  | 'columnFilters'
  | 'sorting'
  | 'columnVisibility'
  | 'columnOrder';

export type UrlFiltersConfig = {
  enabled?: boolean;
  scope?: UrlScope[];
  paramPrefix?: string;
};

export type RawUrlTableState = {
  gf?: string;
  cf?: string;
  so?: string;
  cv?: string;
  co?: string;
};

const SUPPORTED_OPERATORS = new Set([
  'eq',
  'in',
  'neq',
  'gt',
  'lt',
  'gte',
  'lte',
  'contains',
] as const);

const MAX_COLUMN_FILTERS = 20;
const MAX_PARAM_BYTES = 2048;

export const DEFAULT_URL_SCOPE: UrlScope[] = ['globalFilter', 'columnFilters', 'sorting'];
export const DEFAULT_PARAM_PREFIX = 'tb';

// --- Serialize ---

export function serializeGlobalFilter(value: string): string {
  return value;
}

export function serializeColumnFilters(filters: ColumnFilter[]): string {
  return filters
    .filter((f) => {
      // Skip complex object values (e.g. DatetimeFilterValue) — they can't be
      // round-tripped through the id:op:value format and would produce [object Object].
      if (f.value === null || f.value === undefined) return false;
      if (Array.isArray(f.value)) {
        return (f.value as unknown[]).every((v) => typeof v !== 'object' || v === null);
      }
      return typeof f.value !== 'object';
    })
    .map((f) => {
      const isArray = Array.isArray(f.value);
      const op = isArray ? 'in' : 'eq';
      const value = isArray ? (f.value as unknown[]).join('|') : String(f.value);
      return `${f.id}:${op}:${value}`;
    })
    .join(',');
}

export function serializeSorting(sorting: SortingState): string {
  return sorting.map((s) => `${s.id}:${s.desc ? 'desc' : 'asc'}`).join(',');
}

export function serializeColumnVisibility(visibility: ColumnVisibilityState): string {
  return Object.entries(visibility)
    .map(([id, visible]) => `${id}:${visible ? '1' : '0'}`)
    .join(',');
}

// --- Parse ---

export function parseGlobalFilter(raw: string | undefined): string | null {
  if (!raw) return null;
  if (raw.length > MAX_PARAM_BYTES) return null;
  return raw;
}

export function parseColumnFilters(
  raw: string | undefined,
  validColumnIds: string[]
): ColumnFilter[] | null {
  if (!raw) return null;
  if (raw.length > MAX_PARAM_BYTES) return null;

  const validIdSet = new Set(validColumnIds);
  const parts = raw.split(',').slice(0, MAX_COLUMN_FILTERS);
  const filters: ColumnFilter[] = [];

  for (const part of parts) {
    const firstColon = part.indexOf(':');
    if (firstColon === -1) continue;

    const id = part.slice(0, firstColon);
    const rest = part.slice(firstColon + 1);
    const secondColon = rest.indexOf(':');
    if (secondColon === -1) continue;

    const op = rest.slice(0, secondColon);
    const rawValue = rest.slice(secondColon + 1);

    if (!validIdSet.has(id)) continue;
    if (!SUPPORTED_OPERATORS.has(op as Parameters<typeof SUPPORTED_OPERATORS.has>[0])) continue;

    const value = op === 'in' ? rawValue.split('|') : rawValue;
    filters.push({ id, value });
  }

  return filters.length > 0 ? filters : null;
}

export function parseSorting(
  raw: string | undefined,
  validColumnIds: string[]
): SortingState | null {
  if (!raw) return null;
  if (raw.length > MAX_PARAM_BYTES) return null;

  const validIdSet = new Set(validColumnIds);
  const sorting: SortingState = [];

  for (const part of raw.split(',')) {
    const lastColon = part.lastIndexOf(':');
    if (lastColon === -1) continue;
    const id = part.slice(0, lastColon);
    const dir = part.slice(lastColon + 1);
    if (!validIdSet.has(id)) continue;
    if (dir !== 'asc' && dir !== 'desc') continue;
    sorting.push({ id, desc: dir === 'desc' });
  }

  return sorting.length > 0 ? sorting : null;
}

export function parseColumnVisibility(
  raw: string | undefined,
  validColumnIds: string[]
): ColumnVisibilityState | null {
  if (!raw) return null;
  if (raw.length > MAX_PARAM_BYTES) return null;

  const validIdSet = new Set(validColumnIds);
  const visibility: ColumnVisibilityState = {};

  for (const part of raw.split(',')) {
    const lastColon = part.lastIndexOf(':');
    if (lastColon === -1) continue;
    const id = part.slice(0, lastColon);
    const val = part.slice(lastColon + 1);
    if (!validIdSet.has(id)) continue;
    if (val !== '0' && val !== '1') continue;
    visibility[id] = val === '1';
  }

  return Object.keys(visibility).length > 0 ? visibility : null;
}

// --- Full URL payload build / extract ---

export function buildTableUrlParams(
  tableSettingsId: string,
  currentState: Partial<TableState>,
  scope: UrlScope[] = DEFAULT_URL_SCOPE,
  paramPrefix: string = DEFAULT_PARAM_PREFIX
): URLSearchParams {
  const params = new URLSearchParams();
  const prefix = `${paramPrefix}.${tableSettingsId}`;

  if (scope.includes('globalFilter') && currentState.globalFilter) {
    params.set(`${prefix}.gf`, serializeGlobalFilter(currentState.globalFilter));
  }

  if (scope.includes('columnFilters') && currentState.columnFilters?.length) {
    params.set(`${prefix}.cf`, serializeColumnFilters(currentState.columnFilters));
  }

  if (scope.includes('sorting') && currentState.sorting?.length) {
    params.set(`${prefix}.so`, serializeSorting(currentState.sorting));
  }

  if (scope.includes('columnVisibility') && currentState.columnVisibility) {
    const cv = serializeColumnVisibility(currentState.columnVisibility);
    if (cv) params.set(`${prefix}.cv`, cv);
  }

  if (scope.includes('columnOrder') && currentState.columnOrder?.length) {
    params.set(`${prefix}.co`, currentState.columnOrder.join(','));
  }

  return params;
}

export function extractTableUrlParams(
  search: string,
  tableSettingsId: string,
  paramPrefix: string = DEFAULT_PARAM_PREFIX
): RawUrlTableState {
  const params = new URLSearchParams(search);
  const prefix = `${paramPrefix}.${tableSettingsId}`;

  return {
    gf: params.get(`${prefix}.gf`) ?? undefined,
    cf: params.get(`${prefix}.cf`) ?? undefined,
    so: params.get(`${prefix}.so`) ?? undefined,
    cv: params.get(`${prefix}.cv`) ?? undefined,
    co: params.get(`${prefix}.co`) ?? undefined,
  };
}
