import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE } from 'baseui/button';
import { HeadingXSmall, ParagraphMedium } from 'baseui/typography';
import { omit } from 'lodash';

import { SignpostContainer } from './styled-components';

import type { SignpostProps } from './types';
export function Signpost(props: SignpostProps) {
  const [css] = useStyletron();
  const { illustration, title, description, buttonConfig } = props;

  return (
    <SignpostContainer>
      {illustration}
      <HeadingXSmall className={css({ margin: 0 })}>{title}</HeadingXSmall>
      {description && (
        <ParagraphMedium className={css({ margin: 0 })}>{description}</ParagraphMedium>
      )}
      {buttonConfig && (
        <Button {...omit(buttonConfig, 'content')} kind={KIND.secondary} shape={SHAPE.pill}>
          {buttonConfig.content}
        </Button>
      )}
    </SignpostContainer>
  );
}
