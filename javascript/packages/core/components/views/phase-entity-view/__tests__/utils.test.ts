import { describe, expect, test } from 'vitest';

import { CellType } from '#core/components/cell/constants';
import { buildTableConfigFactory } from '#core/components/views/__fixtures__/table-config-factory';
import { isListableEntity } from '../utils';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

describe('isListableEntity', () => {
  const buildTableConfig = buildTableConfigFactory({
    columns: [{ id: 'name', label: 'Name', type: CellType.TEXT }],
  });

  test.each<{
    name: string;
    entity: Pick<PhaseEntityConfig, 'state' | 'views'>;
    expected: boolean;
  }>([
    {
      name: 'active entity with list view',
      entity: {
        state: 'active',
        views: [
          {
            type: 'list',
            tableConfig: buildTableConfig(),
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
            tableConfig: buildTableConfig(),
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
            type: 'detail',
            metadata: [],
            pages: [],
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
            tableConfig: buildTableConfig({ columns: [] }),
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
            tableConfig: buildTableConfig(),
          },
          {
            type: 'detail',
            metadata: [],
            pages: [],
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
            type: 'detail',
            metadata: [],
            pages: [],
          },
          {
            type: 'list',
            tableConfig: buildTableConfig(),
          },
        ],
      },
      expected: true,
    },
  ])('$name', ({ entity, expected }) => {
    expect(isListableEntity(entity)).toBe(expected);
  });
});
