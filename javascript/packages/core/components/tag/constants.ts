import { HIERARCHY, SIZE as BASE_SIZE } from 'baseui/tag';

export const TAG_SIZE = {
  xSmall: 'xSmall',
  ...BASE_SIZE,
} as const;

export const TAG_BEHAVIOR = {
  display: 'display',
  selection: 'selection',
} as const;

export const TAG_COLOR = {
  gray: 'gray',
  red: 'red',
  orange: 'orange',
  yellow: 'yellow',
  green: 'green',
  blue: 'blue',
  purple: 'purple',
  magenta: 'magenta',
  teal: 'teal',
  lime: 'lime',
} as const;

export { HIERARCHY as TAG_HIERARCHY };
