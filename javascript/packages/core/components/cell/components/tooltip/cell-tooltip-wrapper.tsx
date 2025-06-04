import { ACCESSIBILITY_TYPE, PLACEMENT, StatefulTooltip } from 'baseui/tooltip';

import { Icon } from '#core/components/icon/icon';
import { ClickableContainer } from './styled-components';

import type { TooltipWrapperProps } from './types';

export function CellTooltipWrapper(props: TooltipWrapperProps) {
  const { actionHandler, children, content } = props;

  return (
    <StatefulTooltip
      showArrow
      autoFocus
      returnFocus
      placement={PLACEMENT.top}
      accessibilityType={ACCESSIBILITY_TYPE.tooltip}
      content={
        <ClickableContainer onClick={actionHandler}>
          {content}
          {!!actionHandler && <Icon name="chevronRight" size="18px" />}
        </ClickableContainer>
      }
    >
      <div data-testid="tooltip-hover-container">{children}</div>
    </StatefulTooltip>
  );
}
