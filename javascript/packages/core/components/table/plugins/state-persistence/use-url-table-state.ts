import { useMemo } from 'react';
import { useLocation } from 'react-router-dom-v5-compat';

import {
  DEFAULT_PARAM_PREFIX,
  DEFAULT_URL_SCOPE,
  buildTableUrlParams,
  extractTableUrlParams,
  parseColumnFilters,
  parseColumnVisibility,
  parseGlobalFilter,
  parseSorting,
} from './url-codecs';

import type { UrlScope } from './url-codecs';
import type { TableState } from '#core/components/table/types/table-types';

type UseUrlTableStateOptions = {
  tableSettingsId: string;
  validColumnIds: string[];
  scope?: UrlScope[];
  paramPrefix?: string;
};

type UseUrlTableStateResult = {
  urlState: Partial<TableState> | null;
  hasUrlState: boolean;
  buildShareUrl: (currentState: Partial<TableState>) => string;
};

export function useUrlTableState({
  tableSettingsId,
  validColumnIds,
  scope = DEFAULT_URL_SCOPE,
  paramPrefix = DEFAULT_PARAM_PREFIX,
}: UseUrlTableStateOptions): UseUrlTableStateResult {
  const location = useLocation();

  const urlState = useMemo(
    () => {
      const raw = extractTableUrlParams(location.search, tableSettingsId, paramPrefix);
      const state: Partial<TableState> = {};

      if (scope.includes('globalFilter')) {
        const gf = parseGlobalFilter(raw.gf);
        if (gf !== null) state.globalFilter = gf;
      }

      if (scope.includes('columnFilters')) {
        const cf = parseColumnFilters(raw.cf, validColumnIds);
        if (cf !== null) state.columnFilters = cf;
      }

      if (scope.includes('sorting')) {
        const so = parseSorting(raw.so, validColumnIds);
        if (so !== null) state.sorting = so;
      }

      if (scope.includes('columnVisibility')) {
        const cv = parseColumnVisibility(raw.cv, validColumnIds);
        if (cv !== null) state.columnVisibility = cv;
      }

      if (scope.includes('columnOrder') && raw.co) {
        const co = raw.co.split(',').filter((id) => validColumnIds.includes(id));
        if (co.length > 0) state.columnOrder = co;
      }

      return Object.keys(state).length > 0 ? state : null;
    },
    // validColumnIds and scope are intentionally excluded — callers should memoize
    // these arrays or pass stable references to avoid unnecessary re-runs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [location.search, tableSettingsId, paramPrefix]
  );

  const hasUrlState = urlState !== null;

  const buildShareUrl = (currentState: Partial<TableState>): string => {
    // Use window.location.origin + router location as the base so that both
    // production (BrowserRouter) and test (MemoryRouter) environments work correctly.
    const base =
      typeof window !== 'undefined' ? window.location.origin : 'http://localhost:3000';
    const url = new URL(location.pathname, base);

    // Preserve existing search params from the router location
    const existingParams = new URLSearchParams(location.search);
    for (const [key, value] of existingParams.entries()) {
      url.searchParams.set(key, value);
    }

    const prefix = `${paramPrefix}.${tableSettingsId}`;

    // Remove existing tb.tableSettingsId.* params
    for (const key of [...url.searchParams.keys()]) {
      if (key.startsWith(`${prefix}.`)) {
        url.searchParams.delete(key);
      }
    }

    // Append new state params
    const newParams = buildTableUrlParams(tableSettingsId, currentState, scope, paramPrefix);
    for (const [key, value] of newParams.entries()) {
      url.searchParams.set(key, value);
    }

    return url.toString();
  };

  return { urlState, hasUrlState, buildShareUrl };
}
