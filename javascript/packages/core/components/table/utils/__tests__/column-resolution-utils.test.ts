import { resolveColumnForRow } from '../column-resolution-utils';

import type { ColumnConfig } from '#core/components/table/types/column-types';

describe('column resolution utils', () => {
  describe('resolveColumnForRow', () => {
    it('should return the original column when row has no typeMeta', () => {
      const column: ColumnConfig = {
        id: 'name',
        accessor: 'name',
        label: 'Name',
      };

      expect(resolveColumnForRow(column, { name: 'John Doe' })).toBe(column);
    });

    it('should return the original column when row has typeMeta but no kind', () => {
      const column: ColumnConfig = {
        id: 'name',
        accessor: 'name',
        label: 'Name',
      };

      expect(
        resolveColumnForRow(column, {
          name: 'John Doe',
          typeMeta: {},
        })
      ).toBe(column);
    });

    it('should return the original column when typeMeta.kind is not a string', () => {
      const column: ColumnConfig = {
        id: 'name',
        accessor: 'name',
        label: 'Name',
      };

      expect(
        resolveColumnForRow(column, {
          name: 'John Doe',
          typeMeta: { kind: 123 },
        })
      ).toBe(column);
    });

    it('should return the original column when typeMeta.kind does not exist in column', () => {
      const column: ColumnConfig = {
        id: 'name',
        accessor: 'name',
        label: 'Name',
      };

      expect(
        resolveColumnForRow(column, {
          name: 'John Doe',
          typeMeta: { kind: 'NonExistentType' },
        })
      ).toBe(column);
    });

    it('should handle null or undefined row gracefully', () => {
      const column: ColumnConfig = {
        id: 'name',
        accessor: 'name',
        label: 'Name',
      };

      expect(resolveColumnForRow(column, null)).toBe(column);
      expect(resolveColumnForRow(column, undefined)).toBe(column);
    });

    it('should handle row without object structure gracefully', () => {
      const column: ColumnConfig = {
        id: 'name',
        accessor: 'name',
        label: 'Name',
      };

      expect(resolveColumnForRow(column, 'not an object')).toBe(column);
    });

    describe('when typeMeta.kind matches column property', () => {
      let result: ColumnConfig;

      const column: ColumnConfig & { Draft: Partial<ColumnConfig> } = {
        id: 'name',
        accessor: 'name',
        label: 'Name',
        icon: 'default-icon',
        Draft: {
          id: 'name-draft',
          accessor: 'spec.content.name',
          label: 'Draft Name',
        },
      };

      beforeEach(() => {
        result = resolveColumnForRow(column, {
          name: 'John Doe',
          typeMeta: { kind: 'Draft' },
          spec: { content: { name: 'Draft John' } },
        });
      });

      it('should resolve with type-specific overrides preserving existing data', () => {
        expect(result).toEqual(
          expect.objectContaining({
            id: 'name-draft',
            accessor: 'spec.content.name',
            label: 'Draft Name',
          })
        );
      });

      it('should maintain base column properties that are not overridden by type-specific overrides', () => {
        expect(result).toEqual(
          expect.objectContaining({
            icon: 'default-icon',
          })
        );
      });

      it('should remove the typeMeta kind property from result to avoid recursion', () => {
        expect(result).not.toHaveProperty('Draft');
      });
    });
  });
});
