import { Block } from 'baseui/block';
import { HeadingXXLarge } from 'baseui/typography';

import { ConfigDrivenForm } from '#core/components/form/config/config-driven-form';
import { MainViewContainer } from '#core/components/views/main-view-container';

import type { FormConfig } from '#core/components/form/config/types';

const SAMPLE_FORM_CONFIG: FormConfig = {
  entities: {
    'spec.title': { type: 'string', label: 'Title', required: true, placeholder: 'Enter a title' },
    'spec.description': {
      type: 'string',
      label: 'Description',
      placeholder: 'Enter a description',
    },
    'spec.tags': { type: 'string', label: 'Tags', multi: true, placeholder: 'Add a tag' },
  },
  layout: [
    {
      type: 'group',
      label: 'General',
      items: ['spec.title', 'spec.description'],
    },
    {
      type: 'group',
      label: 'Metadata',
      items: ['spec.tags'],
    },
  ],
};

export function Sandbox() {
  return (
    <MainViewContainer>
      <HeadingXXLarge>Component Sandbox</HeadingXXLarge>
      <Block marginBottom="24px">Config-driven form proof-of-life.</Block>
      <Block width="600px">
        <ConfigDrivenForm config={SAMPLE_FORM_CONFIG} onSubmit={() => undefined} />
      </Block>
    </MainViewContainer>
  );
}
