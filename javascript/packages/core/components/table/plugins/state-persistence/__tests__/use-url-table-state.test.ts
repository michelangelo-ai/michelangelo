import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { useUrlTableState } from '../use-url-table-state';

const VALID_IDS = ['name', 'status', 'updatedAt'];

function renderUrlTableState(search: string, options?: Parameters<typeof useUrlTableState>[0]) {
  return renderHook(
    () =>
      useUrlTableState(
        options ?? {
          tableSettingsId: 'users',
          validColumnIds: VALID_IDS,
        }
      ),
    buildWrapper([getRouterWrapper({ location: `/${search}` })])
  );
}

describe('useUrlTableState', () => {
  describe('urlState', () => {
    it('returns null when no matching params are in the URL', () => {
      const { result } = renderUrlTableState('');
      expect(result.current.urlState).toBeNull();
      expect(result.current.hasUrlState).toBe(false);
    });

    it('parses globalFilter from URL', () => {
      const { result } = renderUrlTableState('?tb.users.gf=hello');
      expect(result.current.urlState?.globalFilter).toBe('hello');
      expect(result.current.hasUrlState).toBe(true);
    });

    it('parses columnFilters from URL', () => {
      const { result } = renderUrlTableState('?tb.users.cf=status:in:open|pending');
      expect(result.current.urlState?.columnFilters).toEqual([
        { id: 'status', value: ['open', 'pending'] },
      ]);
    });

    it('parses sorting from URL', () => {
      const { result } = renderUrlTableState('?tb.users.so=updatedAt:desc');
      expect(result.current.urlState?.sorting).toEqual([{ id: 'updatedAt', desc: true }]);
    });

    it('parses columnVisibility when included in scope', () => {
      const { result } = renderUrlTableState('?tb.users.cv=name:0,status:1', {
        tableSettingsId: 'users',
        validColumnIds: VALID_IDS,
        scope: ['columnVisibility'],
      });
      expect(result.current.urlState?.columnVisibility).toEqual({ name: false, status: true });
    });

    it('parses columnOrder when included in scope', () => {
      const { result } = renderUrlTableState('?tb.users.co=status,name', {
        tableSettingsId: 'users',
        validColumnIds: VALID_IDS,
        scope: ['columnOrder'],
      });
      expect(result.current.urlState?.columnOrder).toEqual(['status', 'name']);
    });

    it('ignores params for a different tableSettingsId', () => {
      const { result } = renderUrlTableState('?tb.other.gf=hello');
      expect(result.current.urlState).toBeNull();
    });

    it('silently drops invalid column IDs in filters', () => {
      const { result } = renderUrlTableState('?tb.users.cf=ghost:eq:x,status:eq:active');
      expect(result.current.urlState?.columnFilters).toEqual([
        { id: 'status', value: 'active' },
      ]);
    });

    it('silently drops invalid column IDs in columnOrder', () => {
      const { result } = renderUrlTableState('?tb.users.co=status,ghost,name', {
        tableSettingsId: 'users',
        validColumnIds: VALID_IDS,
        scope: ['columnOrder'],
      });
      expect(result.current.urlState?.columnOrder).toEqual(['status', 'name']);
    });

    it('only parses pieces included in scope', () => {
      const { result } = renderUrlTableState(
        '?tb.users.gf=hello&tb.users.so=name:asc',
        {
          tableSettingsId: 'users',
          validColumnIds: VALID_IDS,
          scope: ['globalFilter'],
        }
      );
      expect(result.current.urlState?.globalFilter).toBe('hello');
      expect(result.current.urlState?.sorting).toBeUndefined();
    });
  });

  describe('buildShareUrl', () => {
    it('returns a string URL', () => {
      const { result } = renderUrlTableState('');
      const url = result.current.buildShareUrl({ globalFilter: 'test' });
      expect(typeof url).toBe('string');
    });

    it('encodes globalFilter into the URL', () => {
      const { result } = renderUrlTableState('');
      const url = result.current.buildShareUrl({ globalFilter: 'engineering' });
      expect(url).toContain('tb.users.gf=engineering');
    });

    it('encodes sorting into the URL', () => {
      const { result } = renderUrlTableState('');
      const url = result.current.buildShareUrl({
        sorting: [{ id: 'updatedAt', desc: true }],
      });
      expect(url).toContain('tb.users.so=updatedAt%3Adesc');
    });

    it('replaces existing tb.* params for this tableSettingsId', () => {
      const { result } = renderUrlTableState('?tb.users.gf=old');
      const url = result.current.buildShareUrl({ globalFilter: 'new' });
      expect(url).toContain('tb.users.gf=new');
      expect(url).not.toContain('old');
    });

    it('preserves unrelated query params', () => {
      const { result } = renderUrlTableState('?other=keep');
      const url = result.current.buildShareUrl({ globalFilter: 'x' });
      expect(url).toContain('other=keep');
    });

    it('uses a custom paramPrefix when specified', () => {
      const { result } = renderUrlTableState('', {
        tableSettingsId: 'users',
        validColumnIds: VALID_IDS,
        paramPrefix: 'app',
      });
      const url = result.current.buildShareUrl({ globalFilter: 'x' });
      expect(url).toContain('app.users.gf=x');
    });
  });
});
