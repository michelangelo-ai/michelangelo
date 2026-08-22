import { CellType } from '#core/components/cell/constants';
import { ModelInfoPage } from './model-info-page';
import { ModelPerformancePage } from './model-performance-page';

import type { DetailViewConfig } from '#core/components/views/types';

export const MODEL_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [{ id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE }],
  pages: [
    { id: 'information', label: 'Information', type: 'custom', component: ModelInfoPage },
    { id: 'performance', label: 'Performance', type: 'custom', component: ModelPerformancePage },
  ],
};
