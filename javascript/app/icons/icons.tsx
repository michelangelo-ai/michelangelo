import CheckCircle from '@mui/icons-material/CheckCircle';
import Launch from '@mui/icons-material/Launch';

import { createMuiIconAdapter } from './mui-icon-adapter';

export const ICONS = {
  arrowLaunch: createMuiIconAdapter(Launch),
  circleCheckFilled: createMuiIconAdapter(CheckCircle),
} as const;
