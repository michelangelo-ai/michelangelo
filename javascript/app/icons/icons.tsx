import CheckCircle from '@mui/icons-material/CheckCircle';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import Info from '@mui/icons-material/Info';
import Launch from '@mui/icons-material/Launch';

import { createMuiIconAdapter } from './mui-icon-adapter';

export const ICONS = {
  arrowLaunch: createMuiIconAdapter(Launch),
  chevronRight: createMuiIconAdapter(ChevronRightIcon),
  circleI: createMuiIconAdapter(Info),
  circleCheckFilled: createMuiIconAdapter(CheckCircle),
} as const;
