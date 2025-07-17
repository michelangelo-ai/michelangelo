import { useContext } from 'react';

import { InterpolationContext } from './interpolation-context';

export const useInterpolationContext = () => useContext(InterpolationContext) ?? {};
