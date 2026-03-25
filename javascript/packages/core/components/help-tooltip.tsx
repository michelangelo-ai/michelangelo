import { useStyletron } from 'baseui';
import { ACCESSIBILITY_TYPE, PLACEMENT, StatefulTooltip } from 'baseui/tooltip';

import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';
import { Markdown } from '#core/components/markdown/markdown';

import type { ReactNode } from 'react';

/**
 * Displays a help icon with a tooltip containing markdown-formatted help text.
 *
 * This component provides contextual help to users through an accessible tooltip
 * that appears when hovering over or focusing the info icon. The tooltip content
 * supports markdown formatting for rich text, code snippets, and links.
 *
 * Features:
 * - Accessible tooltip with keyboard support (auto-focus and return focus)
 * - Markdown rendering in tooltip content
 * - Positioned above the icon by default
 * - Maximum width of 400px for readability
 * - Info icon (circleI) with tertiary styling
 *
 * @param props.text - Help text to display. Supports markdown formatting if a string,
 *   or any React node for custom content.
 *
 * @example
 * ```tsx
 * // Basic help text
 * <HelpTooltip text="This field is required" />
 *
 * // With markdown formatting
 * <HelpTooltip text="Use **bold** for emphasis and `code` for values" />
 *
 * // With code block
 * <HelpTooltip text="Example:\n```json\n{\"key\": \"value\"}\n```" />
 *
 * // With link
 * <HelpTooltip text="Learn more in [our docs](https://docs.example.com)" />
 *
 * // Next to form field label
 * <label>
 *   Pipeline Name <HelpTooltip text="Unique identifier for the pipeline" />
 * </label>
 * ```
 */
export function HelpTooltip({ text }: { text: string | ReactNode }) {
  const [css] = useStyletron();

  return (
    <StatefulTooltip
      showArrow
      returnFocus
      autoFocus
      placement={PLACEMENT.top}
      accessibilityType={ACCESSIBILITY_TYPE.tooltip}
      content={() => (
        <div className={css({ maxWidth: '400px' })}>
          <Markdown>{text}</Markdown>
        </div>
      )}
    >
      <span className={css({ cursor: 'help', display: 'flex' })}>
        <Icon kind={IconKind.TERTIARY} name="circleI" title="help" />
      </span>
    </StatefulTooltip>
  );
}
