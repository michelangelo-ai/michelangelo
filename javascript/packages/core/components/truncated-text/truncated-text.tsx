import { useEffect, useRef, useState } from 'react';
import { useStyletron } from 'baseui';
import { ACCESSIBILITY_TYPE, PLACEMENT, StatefulTooltip } from 'baseui/tooltip';

import { ELLIPSIS_STYLES } from './constants';

import type { Props } from './types';

/**
 * Displays text with automatic truncation and tooltip for overflow content.
 *
 * This component intelligently detects when text content overflows its container
 * and automatically shows a tooltip with the full text on hover. If the text fits
 * within the container, no tooltip is shown.
 *
 * Features:
 * - Automatic overflow detection with resize handling
 * - CSS ellipsis (...) for truncated text
 * - Tooltip with full text appears only when text is truncated
 * - Accessible with keyboard support
 * - Maximum tooltip width of 400px with word wrapping
 * - Responsive to window resize events
 *
 * @param props.children - Text content to display and potentially truncate
 * @param props.overrides - BaseUI overrides for the Tooltip component
 *
 * @example
 * ```tsx
 * // Short text - no truncation, no tooltip
 * <TruncatedText>
 *   Short pipeline name
 * </TruncatedText>
 *
 * // Long text in constrained container - shows ellipsis and tooltip
 * <div style={{ width: '200px' }}>
 *   <TruncatedText>
 *     This is a very long pipeline name that will be truncated
 *   </TruncatedText>
 * </div>
 *
 * // In table cells
 * <td style={{ maxWidth: '150px' }}>
 *   <TruncatedText>
 *     {row.description}
 *   </TruncatedText>
 * </td>
 *
 * // With custom tooltip styling
 * <TruncatedText
 *   overrides={{
 *     Tooltip: {
 *       Body: {
 *         style: { backgroundColor: '#333' }
 *       }
 *     }
 *   }}
 * >
 *   Long content here
 * </TruncatedText>
 * ```
 */
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
