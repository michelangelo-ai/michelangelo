import type { SHAPE } from 'baseui/button';

export type AddButtonProps = {
  label?: string;
  shape?: keyof typeof SHAPE;
  onClick: () => void;
};
