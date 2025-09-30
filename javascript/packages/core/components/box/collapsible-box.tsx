import React, { useEffect, useState } from 'react';
import { getOverrides, mergeOverrides, Theme } from 'baseui';
import { Panel } from 'baseui/accordion';

import { DescriptionText } from '#core/components/description-text';
import { StyledBoxContainer, StyledBoxTitle } from './styled-components';

import type { CollapsibleBoxProps } from './types';

export const CollapsibleBox: React.FC<CollapsibleBoxProps> = ({
  children,
  title,
  description,
  expanded: controlledExpanded,
  defaultExpanded = false,
  onToggle,
  disabled = false,
  overrides = {},
}) => {
  const [internalExpanded, setInternalExpanded] = useState(defaultExpanded);

  const isControlled = controlledExpanded !== undefined;
  const expanded = isControlled ? controlledExpanded : internalExpanded;

  useEffect(() => {
    if (isControlled && !onToggle) {
      console.warn(
        'CollapsibleBox: `expanded` prop provided without `onToggle`. ' +
          'This will make the component unresponsive to user interaction. ' +
          'Either provide `onToggle` or use `defaultExpanded` instead.'
      );
    }
  }, [isControlled, onToggle]);

  const handleToggle = () => {
    if (disabled) return;

    const newExpanded = !expanded;

    if (!isControlled) {
      setInternalExpanded(newExpanded);
    }

    onToggle?.(newExpanded);
  };

  const [HeaderTitle, headerTitleProps] = getOverrides(overrides.HeaderTitle, StyledBoxTitle);

  const header = title ? (
    <div>
      <HeaderTitle {...headerTitleProps}>{title}</HeaderTitle>
      {description && <DescriptionText>{description}</DescriptionText>}
    </div>
  ) : undefined;

  return (
    <Panel
      expanded={expanded}
      onChange={handleToggle}
      title={header}
      overrides={mergeOverrides(
        {
          PanelContainer: {
            component: StyledBoxContainer,
            style: ({ $expanded, $theme }: { $expanded: boolean; $theme: Theme }) => ({
              ...(!$expanded && { gap: 0 }),
              transitionProperty: 'gap',
              transitionDuration: $theme.animation.timing500,
            }),
          },
          Header: {
            style: {
              padding: 0,
            },
          },
          Content: {
            style: {
              padding: 0,
              paddingBottom: 0,
            },
          },
          ToggleIcon: {
            style: {
              alignSelf: 'flex-start',
            },
          },
        },
        {
          ...(overrides.Container && { PanelContainer: overrides.Container }),
          ...(overrides.Header && { Header: overrides.Header }),
          ...(overrides.Content && { Content: overrides.Content }),
          ...(overrides.ToggleIcon && { ToggleIcon: overrides.ToggleIcon }),
        }
      )}
    >
      {children}
    </Panel>
  );
};
