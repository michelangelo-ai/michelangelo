import { createContext } from 'react';

import type { InterpolationContextExtensions } from '#core/interpolation/types';

export const InterpolationContext = createContext<InterpolationContextExtensions | undefined>(
  undefined
);
