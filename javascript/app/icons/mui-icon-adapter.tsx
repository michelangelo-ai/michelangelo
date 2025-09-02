import type { SvgIconProps } from '@mui/material/SvgIcon';
import type { IconProps } from 'baseui/icon';
import type { ComponentType } from 'react';

export const createMuiIconAdapter = (Icon: ComponentType<SvgIconProps>) => {
  return (props: IconProps) => {
    // Scale Material UI icons to match internal icon sizing (14px internal ≈ 12px Material UI)
    const scaledSize = props.size ? `calc(${props.size} * 1.125)` : props.size;
    return <Icon sx={{ ...props.style, color: props.color, fontSize: scaledSize }} />;
  };
};
