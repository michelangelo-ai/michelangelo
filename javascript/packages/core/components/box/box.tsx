import { getOverrides } from 'baseui';

import { DescriptionText } from '#core/components/description-text';
import { StyledBoxContainer, StyledBoxHeader, StyledBoxTitle } from './styled-components';

import type { Props } from './types';

export function Box(props: Props) {
  const { children, description, overrides = {}, title } = props;

  const [BoxContainer, boxContainerProps] = getOverrides(
    overrides.BoxContainer,
    StyledBoxContainer
  );
  const [BoxHeader, boxHeaderProps] = getOverrides(overrides.BoxHeader, StyledBoxHeader);
  const [BoxTitle, boxTitleProps] = getOverrides(overrides.BoxTitle, StyledBoxTitle);

  return (
    <BoxContainer {...boxContainerProps}>
      {title && (
        <BoxHeader {...boxHeaderProps}>
          <BoxTitle {...boxTitleProps}>{title}</BoxTitle>
          <DescriptionText>{description}</DescriptionText>
        </BoxHeader>
      )}
      {children}
    </BoxContainer>
  );
}
