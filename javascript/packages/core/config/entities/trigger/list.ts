import { CellType } from '#core/components/cell/constants';
import { readEnvironmentLabel } from '#core/utils/environment-utils';
import { TRIGGER_PIPELINE_CELL_CONFIG, TRIGGER_STATE_CELL_CONFIG } from './shared';

import type { ListViewConfig } from '#core/components/views/types';
import type { ManifestTrigger } from './types';

export const TRIGGER_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: [
      {
        id: 'metadata.name',
        label: 'Name',
        url: '/${studio.projectId}/${studio.phase}/triggers/${data.metadata.name}',
      },
      TRIGGER_PIPELINE_CELL_CONFIG,
      { id: 'metadata.creationTimestamp.seconds', label: 'Creation time', type: CellType.DATE },
      {
        id: 'spec.trigger.triggerType.value.cron',
        label: 'Cron',
        type: CellType.TEXT,
      },
      {
        id: 'spec.trigger.triggerType.value.interval.seconds',
        label: 'Interval seconds',
        type: CellType.TEXT,
        accessor: (data: unknown) => {
          // cast: accessor receives unknown data; narrowing to expected proto shape for
          // property access
          const triggerType = (data as { spec?: { trigger?: ManifestTrigger } })?.spec?.trigger
            ?.triggerType;
          if (triggerType?.case !== 'intervalSchedule') return undefined;
          const seconds = triggerType.value?.interval?.seconds;
          return seconds === undefined ? undefined : Number(seconds);
        },
      },
      { id: 'spec.actor.name', label: 'Owner', type: CellType.TEXT },
      TRIGGER_STATE_CELL_CONFIG,
      {
        id: 'metadata.labels',
        label: 'Environment',
        type: CellType.TEXT,
        accessor: (data: unknown) => {
          // cast: accessor receives unknown data; narrowing to expected proto shape for
          // property access
          const labels = (data as { metadata?: { labels?: Record<string, string> } })?.metadata
            ?.labels;
          return readEnvironmentLabel(labels) || null;
        },
      },
      { id: 'spec.autoFlip', label: 'Auto switch to latest main', type: CellType.BOOLEAN },
    ],
  },
};
