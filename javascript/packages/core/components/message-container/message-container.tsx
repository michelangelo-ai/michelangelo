import { getOverrides } from 'baseui';

import { Markdown } from '#core/components/markdown/markdown';
import { StyledContent, StyledRoot } from './styled-components';
import { MessageLevel } from './types';

import type { MessageContainerProps } from './types';

/**
 * Displays a status or diagnostic message with level-based background coloring
 * (error, warning, or informational). Message content is rendered as markdown.
 */
export function MessageContainer(props: MessageContainerProps) {
  const { level = MessageLevel.INFO, message, overrides = {} } = props;

  const [Root, rootProps] = getOverrides(overrides.Root, StyledRoot);
  const [Content, contentProps] = getOverrides(overrides.Content, StyledContent);

  return (
    <Root {...rootProps} $level={level}>
      <Content {...contentProps}>
        <Markdown>{message}</Markdown>
      </Content>
    </Root>
  );
}
