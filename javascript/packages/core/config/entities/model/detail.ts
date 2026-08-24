import { CellType } from '#core/components/cell/constants';
import { getCrdUpdatedSeconds } from '#core/utils/crd-utils';
import { MODEL_KIND_TEXT_MAP } from './constants';
import { ModelInfoPage } from './model-info-page';
import { ModelPerformancePage } from './model-performance-page';

import type { DetailViewConfig } from '#core/components/views/types';

export const MODEL_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    {
      id: 'spec.sourcePipelineRun.name',
      label: 'Source pipeline run',
      url: '/${studio.projectId}/${studio.phase}/runs/${data.spec.sourcePipelineRun.name}',
    },
    { id: 'spec.owner.name', label: 'Trained by', type: CellType.TEXT },
    { id: 'metadata.creationTimestamp.seconds', label: 'Creation time', type: CellType.DATE },
    {
      id: 'metadata',
      label: 'Last updated',
      type: CellType.DATE,
      accessor: (data: unknown) => {
        // cast: accessor receives unknown data; narrowing to expected proto shape for property
        // access; see #1425
        const row = data as {
          metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
        };
        return getCrdUpdatedSeconds(row);
      },
    },
    { id: 'spec.kind', label: 'Type', type: CellType.TYPE, typeTextMap: MODEL_KIND_TEXT_MAP },
  ],
  pages: [
    { id: 'information', label: 'Information', type: 'custom', component: ModelInfoPage },
    { id: 'performance', label: 'Performance', type: 'custom', component: ModelPerformancePage },
  ],
};
