import { Block } from 'baseui/block';
import { HeadingXXLarge } from 'baseui/typography';

import { StringField } from '#core/components/form/fields/string/string-field';
import { Form } from '#core/components/form/form';
import { MainViewContainer } from '#core/components/views/main-view-container';

export function Sandbox() {
  return (
    <MainViewContainer>
      <HeadingXXLarge>Component Sandbox</HeadingXXLarge>
      <Block marginBottom="24px">This is a sandbox for testing WIP components and features.</Block>
      <Block width="400px">
        <Form onSubmit={() => undefined}>
          <StringField name="tags" label="Tags" multi placeholder="Add a tag" />
        </Form>
      </Block>
    </MainViewContainer>
  );
}
