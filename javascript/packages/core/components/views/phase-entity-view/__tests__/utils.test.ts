import { describe, expect, test } from 'vitest';

import { CellType } from '#core/components/cell/constants';
import { isListableEntity } from '../utils';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

describe('isListableEntity', () => {
  const testCases: Array<{
    name: string;
    entity: Pick<PhaseEntityConfig, 'state' | 'views'>;
    expected: boolean;
  }> = [
    {
      name: 'active entity with list view',
      entity: {
        state: 'active',
        views: [
          {
            type: 'list',
            columns: [{ id: 'name', label: 'Name', type: CellType.TEXT }],
          },
        ],
      },
      expected: true,
    },
    {
      name: 'disabled entity with list view',
      entity: {
        state: 'disabled',
        views: [
          {
            type: 'list',
            columns: [{ id: 'name', label: 'Name', type: CellType.TEXT }],
          },
        ],
      },
      expected: false,
    },
    {
      name: 'active entity with no views',
      entity: {
        state: 'active',
        views: [],
      },
      expected: false,
    },
    {
      name: 'active entity with non-list view',
      entity: {
        state: 'active',
        views: [
          {
            // @ts-expect-error - detail is not a valid view type
            type: 'detail',
            columns: [],
          },
        ],
      },
      expected: false,
    },
    {
      name: 'active entity with list view but empty columns',
      entity: {
        state: 'active',
        views: [
          {
            type: 'list',
            columns: [],
          },
        ],
      },
      expected: true,
    },
    {
      name: 'active entity with multiple views where first is list',
      entity: {
        state: 'active',
        views: [
          {
            type: 'list',
            columns: [{ id: 'name', label: 'Name', type: CellType.TEXT }],
          },
          {
            // @ts-expect-error - detail is not a valid view type
            type: 'detail',
            columns: [],
          },
        ],
      },
      expected: true,
    },
    {
      name: 'active entity with multiple views where first is not list',
      entity: {
        state: 'active',
        views: [
          {
            // @ts-expect-error - detail is not a valid view type
            type: 'detail',
            columns: [],
          },
          {
            type: 'list',
            columns: [{ id: 'name', label: 'Name', type: CellType.TEXT }],
          },
        ],
      },
      expected: true,
    },
  ];

  test.each(testCases)('$name', ({ entity, expected }) => {
    expect(isListableEntity(entity)).toBe(expected);
  });
});
