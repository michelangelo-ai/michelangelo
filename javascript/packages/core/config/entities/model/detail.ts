import { CellType } from '#core/components/cell/constants';
import { TAG_COLOR } from '#core/components/tag/constants';
import { getCrdUpdatedSeconds } from '#core/utils/crd-utils';
import { dataQualityAccessor, MODEL_KIND_TEXT_MAP } from './constants';
import { ModelInfoPage } from './model-info-page';

import type { DetailViewConfig } from '#core/components/views/types';

export const MODEL_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    {
      id: 'spec.sourcePipelineRun.name',
      label: 'Source pipeline run',
      type: CellType.LINK,
      url: '/${studio.projectId}/${studio.phase}/runs/${page.spec.sourcePipelineRun.name}',
    },
    { id: 'spec.owner.name', label: 'Trained by', type: CellType.TEXT },
    { id: 'metadata.creationTimestamp.seconds', label: 'Creation time', type: CellType.DATE },
    {
      id: 'lastUpdated',
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
    {
      id: 'dataQuality',
      label: 'Data Quality',
      type: CellType.STATE,
      accessor: dataQualityAccessor,
      stateColorMap: { Passed: TAG_COLOR.green, Failed: TAG_COLOR.red },
    },
  ],
  pages: [
    {
      id: 'information',
      label: 'Information',
      type: 'custom',
      component: ModelInfoPage,
    },
  ],
};
