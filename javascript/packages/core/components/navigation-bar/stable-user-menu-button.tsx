import React from 'react';
import { Button } from 'baseui/button';

// BaseUI's user-menu applies paddingLeft on the chevron SVG via overrides.Svg.style,
// but theme icon adapters (e.g. MUI) may strip those overrides. This component
// normalizes the layout: the wrapper clips any inherited padding so the button's
// gap property alone controls avatar–chevron spacing across icon libraries.
export const StableUserMenuButton = React.forwardRef<
  HTMLButtonElement,
  React.ComponentPropsWithRef<typeof Button>
>(function StableUserMenuButton(props, ref) {
  const { children, ...rest } = props;
  return (
    <Button {...rest} ref={ref}>
      {React.Children.map(children, (child: React.ReactNode, i) => {
        if (i === 1 && React.isValidElement<Record<string, unknown>>(child)) {
          // cast: cloneElement props are loosely typed; overrides is a valid BaseUI icon prop
          return React.cloneElement(child, {
            overrides: { Svg: { style: { paddingLeft: 0 } } },
          });
        }
        return child;
      })}
    </Button>
  );
});
