import { Block } from 'baseui/block';
import { HeadingXXLarge } from 'baseui/typography';

import { MainViewContainer } from '#core/components/views/main-view-container';

export function Sandbox() {
  return (
    <MainViewContainer>
      <HeadingXXLarge>Component Sandbox</HeadingXXLarge>
      <Block marginBottom="24px">This is a sandbox for testing WIP components and features.</Block>
    </MainViewContainer>
  );
}
