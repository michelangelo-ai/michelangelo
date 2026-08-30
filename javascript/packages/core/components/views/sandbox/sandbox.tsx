import { useState } from 'react';
import { Block } from 'baseui/block';
import { HeadingXXLarge, LabelMedium } from 'baseui/typography';

import { SubmitButton } from '#core/components/form/components/submit-button/submit-button';
import { Form } from '#core/components/form/form';
import { LayoutItemList } from '#core/components/form/layout/layout-item-list';
import { filterHiddenConditionFields } from '#core/components/form/utils/filter-hidden-condition-fields';
import { MainViewContainer } from '#core/components/views/main-view-container';
import { RepeatedConditionExample } from '#core/components/views/sandbox/repeated-condition-example';

import type { FormConfig } from '#core/components/form/types/config-types';

const conditionalFormConfig: FormConfig = {
  fields: {
    mode: {
      type: 'select',
      label: 'Mode',
      options: [
        { id: 'basic', label: 'Basic' },
        { id: 'advanced', label: 'Advanced' },
      ],
      clearable: false,
    },
    advancedSetting: { type: 'string', label: 'Advanced Setting' },
  },
  layout: ['mode', { type: 'condition', when: 'mode', is: 'advanced', items: ['advancedSetting'] }],
};

export function Sandbox() {
  const [submitted, setSubmitted] = useState<Record<string, unknown>>();
  const [repeatedSubmitted, setRepeatedSubmitted] = useState<Record<string, unknown>>();

  return (
    <MainViewContainer>
      <HeadingXXLarge>Component Sandbox</HeadingXXLarge>
      <Block marginBottom="24px">This is a sandbox for testing WIP components and features.</Block>
      <Block width="400px">
        <Form
          initialValues={{ mode: 'basic' }}
          onSubmit={(values) => {
            const transformed = filterHiddenConditionFields(values, conditionalFormConfig);
            setSubmitted(transformed);
          }}
        >
          <LayoutItemList
            items={conditionalFormConfig.layout}
            fields={conditionalFormConfig.fields}
          />
          <SubmitButton>Submit</SubmitButton>
        </Form>
      </Block>
      {submitted ? (
        <Block marginTop="24px">
          <LabelMedium marginBottom="scale300">Last submitted values</LabelMedium>
          <Block as="pre" backgroundColor="backgroundSecondary" padding="12px">
            {JSON.stringify(submitted, null, 2)}
          </Block>
        </Block>
      ) : null}
      <Block marginTop="48px" width="400px">
        <LabelMedium marginBottom="scale300">Condition inside a repeated layout</LabelMedium>
        <Form
          initialValues={{ items: [{ enabled: false }, { enabled: false }] }}
          onSubmit={(values) => setRepeatedSubmitted(values)}
        >
          <RepeatedConditionExample />
          <SubmitButton>Submit</SubmitButton>
        </Form>
      </Block>
      {repeatedSubmitted ? (
        <Block marginTop="24px">
          <LabelMedium marginBottom="scale300">Last submitted values</LabelMedium>
          <Block as="pre" backgroundColor="backgroundSecondary" padding="12px">
            {JSON.stringify(repeatedSubmitted, null, 2)}
          </Block>
        </Block>
      ) : null}
    </MainViewContainer>
  );
}
