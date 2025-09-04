import type { SvgIconProps } from '@mui/material/SvgIcon';
import type { IconProps } from 'baseui/icon';
import type { ComponentType } from 'react';

export const createMuiIconAdapter = (Icon: ComponentType<SvgIconProps>) => {
  return (props: IconProps) => {
    const { size, style, color, shapeRendering, title, ...baseUIProps } = props;

    // Scale Material UI icons to match internal icon sizing (14px internal ≈ 12px Material UI)
    const scaledSize = size ? `calc(${size} * 1.125)` : size;

    // Remove BaseUI-specific props that would cause type errors
    const { overrides: _overrides, fontSize: _fontSize, ...compatibleProps } = baseUIProps;

    return (
      <Icon
        {...compatibleProps}
        htmlColor={color}
        shapeRendering={String(shapeRendering)}
        titleAccess={String(title)}
        sx={{ ...style, fontSize: scaledSize }}
      />
    );
  };
};
