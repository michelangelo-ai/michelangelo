import AddIcon from '@mui/icons-material/Add';
import ArrowDownwardIcon from '@mui/icons-material/ArrowDownward';
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import AutorenewIcon from '@mui/icons-material/Autorenew';
import CancelIcon from '@mui/icons-material/Cancel';
import CheckCircle from '@mui/icons-material/CheckCircle';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import CloseIcon from '@mui/icons-material/Close';
import CropSquareIcon from '@mui/icons-material/CropSquare';
import Delete from '@mui/icons-material/Delete';
import ErrorIcon from '@mui/icons-material/Error';
import Info from '@mui/icons-material/Info';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardBackspaceIcon from '@mui/icons-material/KeyboardBackspace';
import Launch from '@mui/icons-material/Launch';
import MenuIcon from '@mui/icons-material/Menu';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import SearchIcon from '@mui/icons-material/Search';
import SettingsIcon from '@mui/icons-material/Settings';
import ShowChartIcon from '@mui/icons-material/ShowChart';
import SkipNextIcon from '@mui/icons-material/SkipNext';

import { createMuiIconAdapter } from './mui-icon-adapter';

export const ICONS = {
  arrowCircular: createMuiIconAdapter(AutorenewIcon),
  arrowLaunch: createMuiIconAdapter(Launch),
  arrowLeft: createMuiIconAdapter(KeyboardBackspaceIcon),
  chartLine: createMuiIconAdapter(ShowChartIcon),
  chevronRight: createMuiIconAdapter(ChevronRightIcon),
  chevronDown: createMuiIconAdapter(KeyboardArrowDownIcon),
  circleI: createMuiIconAdapter(Info),
  circleX: createMuiIconAdapter(CancelIcon),
  circleCheck: createMuiIconAdapter(CheckCircle),
  circleCheckFilled: createMuiIconAdapter(CheckCircle),
  circleExclamation: createMuiIconAdapter(ErrorIcon),
  close: createMuiIconAdapter(CloseIcon),
  deleteAlt: createMuiIconAdapter(CancelIcon),
  diamondEmpty: createMuiIconAdapter(CropSquareIcon),
  menu: createMuiIconAdapter(MenuIcon),
  overflowMenu: createMuiIconAdapter(MoreVertIcon),
  playerNext: createMuiIconAdapter(SkipNextIcon),
  playerPlay: createMuiIconAdapter(PlayArrowIcon),
  plus: createMuiIconAdapter(AddIcon),
  search: createMuiIconAdapter(SearchIcon),
  settings: createMuiIconAdapter(SettingsIcon),
  sortAscending: createMuiIconAdapter(ArrowUpwardIcon),
  sortDescending: createMuiIconAdapter(ArrowDownwardIcon),
  stars: createMuiIconAdapter(AutoAwesomeIcon),
  trashCan: createMuiIconAdapter(Delete),
  x: createMuiIconAdapter(CloseIcon),
} as const;
