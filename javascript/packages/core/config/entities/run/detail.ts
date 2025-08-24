import { SHARED_RUN_CELL_CONFIG } from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

export const RUN_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: SHARED_RUN_CELL_CONFIG,
  pages: [
    {
      type: 'execution',
    },
  ],
};
