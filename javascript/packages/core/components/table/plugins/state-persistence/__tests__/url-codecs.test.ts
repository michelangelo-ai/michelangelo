import { describe, expect, it } from 'vitest';

import {
  buildTableUrlParams,
  extractTableUrlParams,
  parseColumnFilters,
  parseColumnVisibility,
  parseGlobalFilter,
  parseSorting,
  serializeColumnFilters,
  serializeColumnVisibility,
  serializeGlobalFilter,
  serializeSorting,
} from '../url-codecs';

import type { ColumnFilter, SortingState } from '#core/components/table/types/table-types';

const VALID_IDS = ['name', 'status', 'department', 'updatedAt', 'createdAt'];

describe('serializeGlobalFilter / parseGlobalFilter', () => {
  it('round-trips a plain string', () => {
    const raw = serializeGlobalFilter('hello world');
    expect(parseGlobalFilter(raw)).toBe('hello world');
  });

  it('returns null for undefined input', () => {
    expect(parseGlobalFilter(undefined)).toBeNull();
  });

  it('returns null for empty string', () => {
    expect(parseGlobalFilter('')).toBeNull();
  });

  it('returns null when input exceeds max size', () => {
    expect(parseGlobalFilter('x'.repeat(2049))).toBeNull();
  });
});

describe('serializeColumnFilters / parseColumnFilters', () => {
  it('round-trips an eq filter', () => {
    const filters: ColumnFilter[] = [{ id: 'status', value: 'active' }];
    const raw = serializeColumnFilters(filters);
    expect(parseColumnFilters(raw, VALID_IDS)).toEqual(filters);
  });

  it('round-trips an in filter with multiple values', () => {
    const filters: ColumnFilter[] = [{ id: 'status', value: ['open', 'pending'] }];
    const raw = serializeColumnFilters(filters);
    expect(parseColumnFilters(raw, VALID_IDS)).toEqual(filters);
  });

  it('round-trips multiple filters', () => {
    const filters: ColumnFilter[] = [
      { id: 'status', value: ['active', 'inactive'] },
      { id: 'name', value: 'alice' },
    ];
    const raw = serializeColumnFilters(filters);
    expect(parseColumnFilters(raw, VALID_IDS)).toEqual(filters);
  });

  it('drops filters with unknown column IDs', () => {
    const raw = 'unknown:eq:x,status:eq:active';
    expect(parseColumnFilters(raw, VALID_IDS)).toEqual([{ id: 'status', value: 'active' }]);
  });

  it('skips filters with complex object values (e.g. DatetimeFilterValue)', () => {
    const filters: ColumnFilter[] = [
      { id: 'status', value: 'active' },
      {
        id: 'metadata.creationTimestamp.seconds',
        value: { operation: 'today', range: [new Date()], selection: [], description: 'Today', exclude: false },
      },
    ];
    const raw = serializeColumnFilters(filters);
    // Only the primitive filter is encoded; the datetime object is skipped
    expect(raw).toBe('status:eq:active');
    expect(raw).not.toContain('[object Object]');
  });

  it('skips null and undefined filter values', () => {
    const filters: ColumnFilter[] = [
      { id: 'status', value: null },
      { id: 'name', value: undefined },
      { id: 'department', value: 'Engineering' },
    ];
    expect(serializeColumnFilters(filters)).toBe('department:eq:Engineering');
  });

  it('drops filters with unknown operators', () => {
    const raw = 'status:like:active';
    expect(parseColumnFilters(raw, VALID_IDS)).toBeNull();
  });

  it('returns null for undefined input', () => {
    expect(parseColumnFilters(undefined, VALID_IDS)).toBeNull();
  });

  it('returns null for empty string', () => {
    expect(parseColumnFilters('', VALID_IDS)).toBeNull();
  });

  it('returns null when input exceeds max size', () => {
    expect(parseColumnFilters('x'.repeat(2049), VALID_IDS)).toBeNull();
  });

  it('returns null when all filters are invalid', () => {
    expect(parseColumnFilters('unknown:eq:val', VALID_IDS)).toBeNull();
  });
});

describe('serializeSorting / parseSorting', () => {
  it('round-trips ascending sort', () => {
    const sorting: SortingState = [{ id: 'name', desc: false }];
    expect(parseSorting(serializeSorting(sorting), VALID_IDS)).toEqual(sorting);
  });

  it('round-trips descending sort', () => {
    const sorting: SortingState = [{ id: 'updatedAt', desc: true }];
    expect(parseSorting(serializeSorting(sorting), VALID_IDS)).toEqual(sorting);
  });

  it('round-trips multi-column sort', () => {
    const sorting: SortingState = [
      { id: 'name', desc: false },
      { id: 'updatedAt', desc: true },
    ];
    expect(parseSorting(serializeSorting(sorting), VALID_IDS)).toEqual(sorting);
  });

  it('drops entries with unknown column IDs', () => {
    const raw = 'name:asc,unknown:desc';
    expect(parseSorting(raw, VALID_IDS)).toEqual([{ id: 'name', desc: false }]);
  });

  it('drops entries with invalid direction', () => {
    const raw = 'name:sideways';
    expect(parseSorting(raw, VALID_IDS)).toBeNull();
  });

  it('returns null for undefined input', () => {
    expect(parseSorting(undefined, VALID_IDS)).toBeNull();
  });

  it('returns null when all entries are invalid', () => {
    expect(parseSorting('unknown:asc', VALID_IDS)).toBeNull();
  });
});

describe('serializeColumnVisibility / parseColumnVisibility', () => {
  it('round-trips visible and hidden columns', () => {
    const visibility = { name: true, status: false };
    const raw = serializeColumnVisibility(visibility);
    expect(parseColumnVisibility(raw, VALID_IDS)).toEqual(visibility);
  });

  it('drops unknown column IDs', () => {
    const raw = 'name:1,ghost:0';
    expect(parseColumnVisibility(raw, VALID_IDS)).toEqual({ name: true });
  });

  it('drops entries with invalid values', () => {
    const raw = 'name:yes';
    expect(parseColumnVisibility(raw, VALID_IDS)).toBeNull();
  });

  it('returns null for undefined input', () => {
    expect(parseColumnVisibility(undefined, VALID_IDS)).toBeNull();
  });
});

describe('buildTableUrlParams', () => {
  it('encodes globalFilter into the correct param key', () => {
    const params = buildTableUrlParams('users', { globalFilter: 'error' });
    expect(params.get('tb.users.gf')).toBe('error');
  });

  it('encodes columnFilters', () => {
    const params = buildTableUrlParams('users', {
      columnFilters: [{ id: 'status', value: ['open', 'pending'] }],
    });
    expect(params.get('tb.users.cf')).toBe('status:in:open|pending');
  });

  it('encodes sorting', () => {
    const params = buildTableUrlParams('users', {
      sorting: [{ id: 'updatedAt', desc: true }],
    });
    expect(params.get('tb.users.so')).toBe('updatedAt:desc');
  });

  it('omits empty/falsy state pieces', () => {
    const params = buildTableUrlParams('users', { globalFilter: '', columnFilters: [] });
    expect([...params.keys()]).toHaveLength(0);
  });

  it('respects custom paramPrefix', () => {
    const params = buildTableUrlParams('users', { globalFilter: 'x' }, ['globalFilter'], 'myapp');
    expect(params.get('myapp.users.gf')).toBe('x');
  });

  it('respects scope to exclude unspecified state pieces', () => {
    const params = buildTableUrlParams(
      'users',
      { globalFilter: 'x', sorting: [{ id: 'name', desc: false }] },
      ['globalFilter']
    );
    expect(params.get('tb.users.gf')).toBe('x');
    expect(params.get('tb.users.so')).toBeNull();
  });

  it('encoding is deterministic — same state produces same output', () => {
    const state = {
      globalFilter: 'test',
      sorting: [
        { id: 'name', desc: false },
        { id: 'updatedAt', desc: true },
      ],
    };
    const a = buildTableUrlParams('t', state).toString();
    const b = buildTableUrlParams('t', state).toString();
    expect(a).toBe(b);
  });
});

describe('extractTableUrlParams', () => {
  it('extracts params for the given tableSettingsId', () => {
    const search = '?tb.users.gf=hello&tb.users.so=name:asc';
    expect(extractTableUrlParams(search, 'users')).toEqual({
      gf: 'hello',
      so: 'name:asc',
      cf: undefined,
      cv: undefined,
      co: undefined,
    });
  });

  it('does not extract params for a different tableSettingsId', () => {
    const search = '?tb.other.gf=hello';
    expect(extractTableUrlParams(search, 'users')).toEqual({
      gf: undefined,
      cf: undefined,
      so: undefined,
      cv: undefined,
      co: undefined,
    });
  });

  it('returns all undefined when search is empty', () => {
    expect(extractTableUrlParams('', 'users')).toEqual({
      gf: undefined,
      cf: undefined,
      so: undefined,
      cv: undefined,
      co: undefined,
    });
  });
});
