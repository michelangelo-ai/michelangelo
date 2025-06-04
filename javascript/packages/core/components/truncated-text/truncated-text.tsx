import { useEffect, useRef, useState } from 'react';
import { useStyletron } from 'baseui';
import { ACCESSIBILITY_TYPE, PLACEMENT, StatefulTooltip } from 'baseui/tooltip';

import { ELLIPSIS_STYLES } from './constants';

import type { Props } from './types';

export function TruncatedText({ children, overrides }: Props) {
  const [css] = useStyletron();
  const [isOverflowing, setIsOverflowing] = useState(false);
  const anchorRef = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    const handleResize = () => {
      if (anchorRef.current) {
        setIsOverflowing(anchorRef.current.scrollWidth > anchorRef.current.clientWidth);
      }
    };

    handleResize();
    window.addEventListener('resize', handleResize);

    return () => window.removeEventListener('resize', handleResize);
  }, [anchorRef]);

  const anchorContent = (
    <div className={css({ display: 'flex', maxWidth: '100%' })}>
      <span className={css(ELLIPSIS_STYLES)} ref={anchorRef}>
        {children}
      </span>
    </div>
  );

  if (!isOverflowing) return anchorContent;

  return (
    <StatefulTooltip
      overrides={overrides?.Tooltip}
      accessibilityType={ACCESSIBILITY_TYPE.tooltip}
      content={
        <div className={css({ maxWidth: '400px', wordBreak: 'break-word' })}>{children}</div>
      }
      placement={PLACEMENT.top}
      showArrow
      returnFocus
      autoFocus
    >
      {anchorContent}
    </StatefulTooltip>
  );
}
