import React, { type ReactNode } from 'react';
import ColorModeToggle from '@theme-original/ColorModeToggle';
import type ColorModeToggleType from '@theme/ColorModeToggle';
import type { WrapperProps } from '@docusaurus/types';
import type { ColorMode } from '@docusaurus/theme-common';
import { flushSync } from 'react-dom';

type Props = WrapperProps<typeof ColorModeToggleType>;

// Mirror Docusaurus logic for 3-value cycle
function getNextColorMode(
  colorMode: ColorMode | null,
  respectPrefersColorScheme: boolean
): ColorMode | null {
  if (!respectPrefersColorScheme) {
    return colorMode === 'dark' ? 'light' : 'dark';
  }
  switch (colorMode) {
    case null:
      return 'light';
    case 'light':
      return 'dark';
    case 'dark':
      return null;
    default:
      return 'light';
  }
}

export default function ColorModeToggleWrapper(props: Props): ReactNode {
  const handleClick = async (e: React.MouseEvent) => {
    const nextMode = getNextColorMode(props.value, props.respectPrefersColorScheme ?? false);

    // Fallback for unsupported browsers or reduced motion
    if (
      !document.startViewTransition ||
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
    ) {
      props.onChange(nextMode);
      return;
    }

    const x = e.clientX;
    const y = e.clientY;
    const maxRadius = Math.hypot(
      Math.max(x, window.innerWidth - x),
      Math.max(y, window.innerHeight - y)
    );

    const transition = document.startViewTransition(() => {
      flushSync(() => props.onChange(nextMode));
    });

    await transition.ready;

    document.documentElement.animate(
      {
        clipPath: [
          `circle(0px at ${x}px ${y}px)`,
          `circle(${maxRadius}px at ${x}px ${y}px)`,
        ],
      },
      {
        duration: 500,
        easing: 'ease-out',
        pseudoElement: '::view-transition-new(root)',
      }
    );
  };

  return (
    <div onClick={handleClick}>
      <ColorModeToggle {...props} onChange={() => {}} />
    </div>
  );
}
