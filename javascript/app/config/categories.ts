import { PHASES } from './phases/phases';

import type { CategoryConfig } from '@michelangelo-ai/core';

export const CATEGORIES: CategoryConfig[] = [
  { id: 'core-ml', name: 'Core ML', phases: Object.values(PHASES) },
];
