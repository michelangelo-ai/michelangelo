import { Button, KIND, SHAPE } from 'baseui/button';
import { omit } from 'lodash';

import { DescriptionContainer, ErrorViewContainer, TitleContainer } from './styled-components';

import type { ErrorViewProps } from './types';

export function ErrorView(props: ErrorViewProps) {
  const { illustration, title, description, buttonConfig } = props;

  return (
    <ErrorViewContainer>
      {illustration}
      <TitleContainer>{title}</TitleContainer>
      {description && <DescriptionContainer>{description}</DescriptionContainer>}
      {buttonConfig && (
        <Button {...omit(buttonConfig, 'content')} kind={KIND.secondary} shape={SHAPE.pill}>
          {buttonConfig.content}
        </Button>
      )}
    </ErrorViewContainer>
  );
}
