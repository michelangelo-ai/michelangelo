import { useState } from 'react';
import { useStyletron } from 'baseui';
import { Block } from 'baseui/block';
import { Tab, Tabs } from 'baseui/tabs';
import { HeadingXXLarge } from 'baseui/typography';

import { TextEditor } from '#core/components/text-editor/text-editor';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { Execution } from '#core/components/views/execution/execution';
import { failurePipelineRun, successfulPipelineRun } from './fixtures/execution-data';

import type { ExecutionDetailViewSchema } from '#core/components/views/execution/types';

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

const executionSchema: ExecutionDetailViewSchema = {
  type: 'execution',
  emptyState: {
    title: 'No execution data available',
    description: 'This component shows task execution status and details.',
  },
  tasks: {
    accessor: 'status.steps',
    subTasksAccessor: 'subSteps',
    header: {
      heading: 'displayName',
    },
    stateBuilder: (record: { state: string }) => {
      switch (record.state) {
        case 'PIPELINE_RUN_STEP_STATE_SUCCEEDED':
          return TASK_STATE.SUCCESS;
        case 'PIPELINE_RUN_STEP_STATE_RUNNING':
          return TASK_STATE.RUNNING;
        case 'PIPELINE_RUN_STEP_STATE_PENDING':
          return TASK_STATE.PENDING;
        case 'PIPELINE_RUN_STEP_STATE_FAILED':
          return TASK_STATE.ERROR;
        case 'PIPELINE_RUN_STEP_STATE_SKIPPED':
          return TASK_STATE.SKIPPED;
        default:
          return TASK_STATE.PENDING;
      }
    },
  },
};

export function Sandbox() {
  const [css] = useStyletron();
  const [activeKey, setActiveKey] = useState('0');
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

      <Tabs activeKey={activeKey} onChange={({ activeKey }) => setActiveKey(activeKey as string)}>
        <Tab title="Text Editor">
          <Block marginTop="24px">
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
              <TextEditor
                value="This is a plain text editor without JSON highlighting."
                height="150px"
              />
            </Block>
          </Block>
        </Tab>

        <Tab title="Execution - Success">
          <Block marginTop="24px">
            <Block marginBottom="24px">
              <Block marginBottom="12px">
                <strong>Successful Pipeline Run:</strong>
              </Block>
              <Execution schema={executionSchema} data={successfulPipelineRun} />
            </Block>
          </Block>
        </Tab>

        <Tab title="Execution - Failure">
          <Block marginTop="24px">
            <Block marginBottom="24px">
              <Block marginBottom="12px">
                <strong>Failed Pipeline Run:</strong>
              </Block>
              <Execution schema={executionSchema} data={failurePipelineRun} />
            </Block>
          </Block>
        </Tab>

        <Tab title="Execution - Empty">
          <Block marginTop="24px">
            <Block marginBottom="24px">
              <Block marginBottom="12px">
                <strong>Empty Execution:</strong>
              </Block>
              <Execution schema={executionSchema} data={{}} />
            </Block>
          </Block>
        </Tab>
      </Tabs>
    </Block>
  );
}
