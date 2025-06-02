import type { SvgIconProps } from '@mui/material/SvgIcon';
import type { IconProps } from 'baseui/icon';
import type { ComponentType } from 'react';

export const createMuiIconAdapter = (Icon: ComponentType<SvgIconProps>) => {
  return (props: IconProps) => (
    <Icon sx={{ ...props.style, color: props.color, fontSize: props.size }} />
  );
};
