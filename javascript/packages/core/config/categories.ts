import { PHASES } from '#core/config/phases/phases';

import type { CategoryConfig } from '#core/types/common/studio-types';

export const CATEGORIES: CategoryConfig[] = [
  { id: 'core-ml', name: 'Core ML', phases: Object.values(PHASES) },
];
