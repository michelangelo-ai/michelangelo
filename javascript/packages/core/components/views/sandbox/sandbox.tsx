import { useState } from 'react';
import { useStyletron } from 'baseui';
import { Block } from 'baseui/block';
import { HeadingXLarge, HeadingXXLarge } from 'baseui/typography';

import { TextEditor } from '#core/components/text-editor/text-editor';

const sampleJson = {
  name: 'Text Editor Demo',
  description: 'Testing the migrated TextEditor component',
  features: ['JSON syntax highlighting', 'Read-only mode', 'Editable mode'],
  config: {
    theme: 'light',
    fontSize: '14px',
    showLineNumbers: true,
  },
  data: [
    { id: 1, value: 'Sample data' },
    { id: 2, value: 'More test data' },
  ],
};

export function Sandbox() {
  const [css] = useStyletron();
  const [jsonValue, setJsonValue] = useState(JSON.stringify(sampleJson, null, 2));
  const [readOnlyValue] = useState(
    JSON.stringify({ message: 'This is read-only', timestamp: new Date().toISOString() }, null, 2)
  );

  return (
    <Block
      className={css({
        padding: '24px',
        maxWidth: '1200px',
        margin: '0 auto',
      })}
    >
      <HeadingXXLarge>Component Sandbox</HeadingXXLarge>
      <Block marginBottom="24px">This is a sandbox for testing WIP components and features.</Block>

      <HeadingXLarge>Text Editor Component</HeadingXLarge>

      <Block marginBottom="24px">
        <Block marginBottom="12px">
          <strong>Editable JSON Editor:</strong>
        </Block>
        <TextEditor
          value={jsonValue}
          onChange={(value) => setJsonValue(value || '')}
          language="json"
          height="300px"
        />
      </Block>

      <Block marginBottom="24px">
        <Block marginBottom="12px">
          <strong>Read-Only JSON Viewer:</strong>
        </Block>
        <TextEditor value={readOnlyValue} language="json" readOnly height="200px" />
      </Block>

      <Block marginBottom="24px">
        <Block marginBottom="12px">
          <strong>Plain Text Editor:</strong>
        </Block>
        <TextEditor value="This is a plain text editor without JSON highlighting." height="150px" />
      </Block>
    </Block>
  );
}
