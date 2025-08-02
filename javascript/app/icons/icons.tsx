import AutorenewIcon from '@mui/icons-material/Autorenew';
import CancelIcon from '@mui/icons-material/Cancel';
import CheckCircle from '@mui/icons-material/CheckCircle';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import CropSquareIcon from '@mui/icons-material/CropSquare';
import Info from '@mui/icons-material/Info';
import Launch from '@mui/icons-material/Launch';
import SkipNextIcon from '@mui/icons-material/SkipNext';

import { createMuiIconAdapter } from './mui-icon-adapter';

export const ICONS = {
  arrowCircular: createMuiIconAdapter(AutorenewIcon),
  arrowLaunch: createMuiIconAdapter(Launch),
  chevronRight: createMuiIconAdapter(ChevronRightIcon),
  circleI: createMuiIconAdapter(Info),
  circleX: createMuiIconAdapter(CancelIcon),
  circleCheck: createMuiIconAdapter(CheckCircle),
  circleCheckFilled: createMuiIconAdapter(CheckCircle),
  diamondEmpty: createMuiIconAdapter(CropSquareIcon),
  playerNext: createMuiIconAdapter(SkipNextIcon),
} as const;
