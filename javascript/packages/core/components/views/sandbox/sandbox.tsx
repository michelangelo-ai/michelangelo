import { useState } from 'react';
import { FormSpy } from 'react-final-form';
import { Block } from 'baseui/block';
import { Button } from 'baseui/button';
import { Notification } from 'baseui/notification';
import { Tab, Tabs } from 'baseui/tabs';
import { HeadingXXLarge, LabelLarge, ParagraphSmall } from 'baseui/typography';

import { CellType } from '#core/components/cell/constants';
import { FormErrorBanner } from '#core/components/form/components/form-error-banner/form-error-banner';
import { CheckboxField } from '#core/components/form/fields/checkbox/checkbox-field';
import { NumberField } from '#core/components/form/fields/number/number-field';
import { SelectField } from '#core/components/form/fields/select/select-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { UrlField } from '#core/components/form/fields/url/url-field';
import { Form } from '#core/components/form/form';
import { ArrayFormGroup } from '#core/components/form/layout/array-form-group/array-form-group';
import { ArrayFormRow } from '#core/components/form/layout/array-form-row/array-form-row';
import { FormColumn } from '#core/components/form/layout/form-column/form-column';
import { FormGrid } from '#core/components/form/layout/form-grid/form-grid';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { FormNote } from '#core/components/form/layout/form-note/form-note';
import { required } from '#core/components/form/validation/validators';
import { ConfirmDialog } from '#core/components/modal/confirm-dialog/confirm-dialog';
import { TextEditor } from '#core/components/text-editor/text-editor';
import { DetailView } from '#core/components/views/detail-view/detail-view';
import { TaskListRenderer } from '#core/components/views/execution/components/task-list-renderer';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { Execution } from '#core/components/views/execution/execution';
import { MainViewContainer } from '#core/components/views/main-view-container';
import { failurePipelineRun, successfulPipelineRun } from './fixtures/execution-data';

import type { TaskListRendererProps } from '#core/components/views/execution/components/types';
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
      metadata: [
        {
          id: 'state',
          label: 'Status',
          type: CellType.STATE,
          stateTextMap: {
            PIPELINE_RUN_STEP_STATE_SUCCEEDED: 'Success',
            PIPELINE_RUN_STEP_STATE_RUNNING: 'Running',
            PIPELINE_RUN_STEP_STATE_PENDING: 'Pending',
            PIPELINE_RUN_STEP_STATE_FAILED: 'Failed',
            PIPELINE_RUN_STEP_STATE_SKIPPED: 'Skipped',
          },
          stateColorMap: {
            PIPELINE_RUN_STEP_STATE_SUCCEEDED: 'green',
            PIPELINE_RUN_STEP_STATE_RUNNING: 'blue',
            PIPELINE_RUN_STEP_STATE_PENDING: 'blue',
            PIPELINE_RUN_STEP_STATE_FAILED: 'red',
            PIPELINE_RUN_STEP_STATE_SKIPPED: 'gray',
          },
        },
        {
          id: 'startTime.seconds',
          label: 'Started',
          type: CellType.DATE,
        },
        {
          id: 'duration',
          label: 'Duration',
          type: CellType.TEXT,
          accessor: (record: { startTime: { seconds: string }; endTime: { seconds: string } }) => {
            if (record.startTime?.seconds && record.endTime?.seconds) {
              const start = parseInt(record.startTime.seconds) * 1000;
              const end = parseInt(record.endTime.seconds) * 1000;
              const durationMs = end - start;
              const durationSec = Math.round(durationMs / 1000);
              return `${durationSec}s`;
            }
            return null;
          },
        },
        {
          id: 'logUrl',
          label: 'Logs',
        },
      ],
    },
    body: [
      {
        type: 'struct',
        label: 'Input Parameters',
        accessor: 'input',
      },
      {
        type: 'struct',
        label: 'Output Results',
        accessor: 'output',
      },
      {
        type: 'metadata' as const,
        label: 'Execution Info',
        cells: [
          {
            id: 'displayName',
            label: 'Execution Name',
            type: CellType.TEXT,
            accessor: 'displayName',
          },
          {
            id: 'logUrl',
            label: 'Log URL',
            accessor: 'logUrl',
          },
        ],
      },
      {
        type: 'textarea',
        label: 'Task Message',
        accessor: 'message',
        markdown: false,
      },
    ],
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

const parentDebugData = {
  status: {
    steps: [
      {
        subSteps: [
          {
            subSteps: [],
            displayName: 'ASL Step 1',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
          },
          {
            subSteps: [],
            displayName: 'ASL Step 2',
            state: 'PIPELINE_RUN_STEP_STATE_RUNNING',
          },
        ],
        displayName: 'Execute Workflow',
        state: 'PIPELINE_RUN_STEP_STATE_RUNNING',
      },
    ],
  },
};

function ParentDebugRenderer({ taskList, parent, onTaskClick }: TaskListRendererProps) {
  return (
    <Block>
      <Block
        padding="scale300"
        marginBottom="scale300"
        backgroundColor="backgroundSecondary"
        font="font200"
        overrides={{ Block: { style: { borderRadius: '4px', fontFamily: 'monospace' } } }}
      >
        parent = {parent ? <strong>{parent.name}</strong> : <em>undefined</em>}
      </Block>
      <TaskListRenderer taskList={taskList} parent={parent} onTaskClick={onTaskClick} />
    </Block>
  );
}

export function Sandbox() {
  const [activeKey, setActiveKey] = useState('0');
  const [jsonValue, setJsonValue] = useState(JSON.stringify(sampleJson, null, 2));
  const [readOnlyValue] = useState(
    JSON.stringify({ message: 'This is read-only', timestamp: new Date().toISOString() }, null, 2)
  );
  const [isBasicConfirmOpen, setIsBasicConfirmOpen] = useState(false);
  const [isErrorConfirmOpen, setIsErrorConfirmOpen] = useState(false);
  const [isDestructiveConfirmOpen, setIsDestructiveConfirmOpen] = useState(false);

  return (
    <MainViewContainer hasBreadcrumb={false}>
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
            <DetailView
              subtitle="Pipeline Run"
              title="ml-training-job-2024-is-actually-long-really-long-pipeline-runml-training-job-2024-is-actually-long-really-long-pipeline-run"
              onGoBack={() => console.log('Navigate back to list')}
              headerContent={
                <Block>
                  <strong>Status:</strong> Success | <strong>Duration:</strong> 2m 45s |{' '}
                  <strong>Started:</strong> 2024-01-15 14:30:00
                </Block>
              }
            >
              <Execution schema={executionSchema} data={successfulPipelineRun} />
            </DetailView>
          </Block>
        </Tab>

        <Tab title="Execution - Failure">
          <Block marginTop="24px">
            <DetailView
              subtitle="Pipeline Run"
              title="model-validation-job-2024"
              onGoBack={() => console.log('Navigate back to list')}
              headerContent={
                <Block>
                  <strong>Status:</strong> Failed | <strong>Duration:</strong> 1m 20s |{' '}
                  <strong>Started:</strong> 2024-01-15 15:45:00
                </Block>
              }
            >
              <Execution schema={executionSchema} data={failurePipelineRun} />
            </DetailView>
          </Block>
        </Tab>

        <Tab title="Execution - Empty">
          <Block marginTop="24px">
            <DetailView
              subtitle="Pipeline Run"
              title="data-processing-job-2024"
              onGoBack={() => console.log('Navigate back to list')}
              headerContent={
                <Block>
                  <strong>Status:</strong> No Data | <strong>Duration:</strong> - |{' '}
                  <strong>Started:</strong> -
                </Block>
              }
            >
              <Execution schema={executionSchema} data={{}} />
            </DetailView>
          </Block>
        </Tab>

        <Tab title="Execution - Parent Debug">
          <Block marginTop="24px">
            <Block marginBottom="scale600" font="font300">
              Each row in the task flow shows a <code>parent =</code> banner. After the fix, the
              detail panel for <strong>Execute Workflow</strong> should show{' '}
              <code>parent = Execute Workflow</code> — not <code>parent = undefined</code>.
            </Block>
            <DetailView
              subtitle="Debug"
              title="execute-workflow-parent-fix"
              onGoBack={() => console.log('back')}
            >
              <Execution
                schema={executionSchema}
                data={parentDebugData}
                overrides={{
                  TaskListRenderer: { component: ParentDebugRenderer },
                }}
              />
            </DetailView>
          </Block>
        </Tab>

        <Tab title="New Fields">
          <Block marginTop="24px" maxWidth="900px">
            <Form onSubmit={(values) => console.log('Submitted:', values)}>
              <FormGroup title="Checkbox Field">
                <Block display="flex" gridGap="scale800">
                  <Block flex="1">
                    <CheckboxField
                      name="featuresVertical"
                      label="With descriptions (vertical)"
                      options={[
                        { id: 'logging', label: 'Logging', description: 'Enable request logging' },
                        {
                          id: 'metrics',
                          label: 'Metrics',
                          description: 'Collect performance metrics',
                        },
                        { id: 'tracing', label: 'Tracing', description: 'Distributed tracing' },
                      ]}
                    />
                  </Block>
                  <Block flex="1">
                    <CheckboxField
                      name="featuresHorizontal"
                      label="Without descriptions (horizontal wrap)"
                      options={[
                        { id: 'logging', label: 'Logging' },
                        { id: 'metrics', label: 'Metrics' },
                        { id: 'tracing', label: 'Tracing' },
                      ]}
                    />
                  </Block>
                </Block>
              </FormGroup>

              <FormGroup title="Number Field">
                <NumberField name="timeout" label="Timeout (seconds)" placeholder="30" />
                <NumberField name="retries" label="Max Retries" defaultValue={3} />
              </FormGroup>

              <FormGroup title="URL Field">
                <UrlField
                  name="docsUrl"
                  label="Documentation"
                  urlName="View docs"
                  initialValue="https://example.com/docs"
                />
                <UrlField name="emptyUrl" label="Empty URL (no value)" />
              </FormGroup>

              <FormGroup title="FormNote">
                <FormNote content="This is a **formatted** note with [a link](https://example.com) and `inline code`." />
              </FormGroup>

              <FormGroup title="FormGrid + FormColumn">
                <FormNote content="A 4-column grid — each `FormColumn` occupies one column." />
                <FormGrid>
                  <FormColumn>
                    <StringField name="col1" label="Column 1" />
                  </FormColumn>
                  <FormColumn>
                    <StringField name="col2" label="Column 2" />
                  </FormColumn>
                  <FormColumn>
                    <StringField name="col3" label="Column 3" />
                  </FormColumn>
                  <FormColumn>
                    <StringField name="col4" label="Column 4" />
                  </FormColumn>
                </FormGrid>
              </FormGroup>

              <Block display="flex" justifyContent="flex-end" marginTop="scale600">
                <Button type="submit">Submit</Button>
              </Block>
            </Form>
          </Block>
        </Tab>

        <Tab title="Form - Sticky Footer">
          <Block marginTop="24px" maxWidth="600px">
            <Form
              onSubmit={(values) => console.log('Submitted:', values)}
              footer={{
                left: <span>Last saved 2m ago</span>,
                right: <Button type="submit">Save changes</Button>,
              }}
            >
              <FormGroup title="Personal Information" description="Your name and contact details.">
                <StringField name="firstName" label="First Name" />
                <StringField name="lastName" label="Last Name" />
                <StringField name="email" label="Email Address" />
                <StringField name="phone" label="Phone Number" />
              </FormGroup>

              <FormGroup title="Professional Details" description="Your role and team affiliation.">
                <SelectField
                  name="department"
                  label="Department"
                  options={[
                    { id: 'engineering', label: 'Engineering' },
                    { id: 'product', label: 'Product' },
                    { id: 'design', label: 'Design' },
                    { id: 'data', label: 'Data Science' },
                  ]}
                />
                <StringField name="jobTitle" label="Job Title" />
                <StringField name="manager" label="Manager Name" />
              </FormGroup>

              <FormGroup
                title="Account Configuration"
                description="Settings applied to your account on creation."
              >
                <SelectField
                  name="region"
                  label="Region"
                  options={[
                    { id: 'us-east', label: 'US East' },
                    { id: 'us-west', label: 'US West' },
                    { id: 'eu-west', label: 'EU West' },
                    { id: 'ap-south', label: 'AP South' },
                  ]}
                />
                <StringField name="slackHandle" label="Slack Handle" />
              </FormGroup>
            </Form>
          </Block>
        </Tab>

        <Tab title="Repeated Fields">
          <Block marginTop="24px" maxWidth="600px">
            <Form onSubmit={(values) => console.log('Submitted:', values)}>
              <ArrayFormGroup
                rootFieldPath="addresses"
                groupLabel="Address"
                minItems={1}
                description="Addresses are required."
                tooltip="Addresses are required."
              >
                {(name) => (
                  <>
                    <StringField
                      name={`${name}.street`}
                      label="Street"
                      validate={required()}
                      caption="This is a **formatted** caption with [a link](https://example.com) and `inline code`."
                    />
                    <StringField
                      name={`${name}.city`}
                      label="City"
                      validate={required()}
                      caption="This is a very long caption for the City field that demonstrates truncated text and tooltip behavior. The content intentionally exceeds the typical visible width of a form field caption, so when the field is constrained in a narrow container, the caption text will be truncated with an ellipsis. Hovering over the caption will show the full text in a tooltip, making it accessible and user-friendly while ensuring layout consistency. You can include additional helpful details here, such as formatting guidance, requirements, or other user instructions that might not fit easily in a single visible line."
                    />
                    <StringField name={`${name}.postcode`} label="Postcode" />
                  </>
                )}
              </ArrayFormGroup>

              <FormGroup
                title="Supporting links"
                description="Inline repeated fields using ArrayFormRow."
              >
                <ArrayFormRow rootFieldPath="links" span={[1, 2]} minItems={1}>
                  {(name) => (
                    <>
                      <StringField name={`${name}.name`} label="Name" />
                      <StringField name={`${name}.url`} label="URL" />
                    </>
                  )}
                </ArrayFormRow>
              </FormGroup>

              <Block display="flex" justifyContent="flex-end" marginTop="scale600">
                <Button type="submit">Submit</Button>
              </Block>

              <FormSpy subscription={{ values: true }}>
                {({ values }) => (
                  <Block
                    as="pre"
                    marginTop="scale600"
                    padding="scale600"
                    backgroundColor="backgroundSecondary"
                    font="font300"
                    overrides={{ Block: { style: { borderRadius: '8px', overflow: 'auto' } } }}
                  >
                    {JSON.stringify(values, null, 2)}
                  </Block>
                )}
              </FormSpy>
            </Form>
          </Block>
        </Tab>

        <Tab title="Form - Focus on Error">
          <Block marginTop="24px" maxWidth="600px">
            <Notification
              kind="info"
              overrides={{ Body: { style: { width: 'auto', marginBottom: '24px' } } }}
            >
              <LabelLarge marginBottom="scale200">How to see the scroll</LabelLarge>
              <ParagraphSmall margin="0">
                Scroll to the bottom of this page and click <strong>Submit</strong> with fields
                empty. The page will automatically scroll back up and focus the first field with a
                validation error.
              </ParagraphSmall>
            </Notification>

            <Form focusOnError onSubmit={(values) => console.log('Submitted:', values)}>
              <FormGroup
                title="Personal Information"
                description="Your name and contact details. All fields are required."
              >
                <StringField name="firstName" label="First Name" validate={required()} />
                <StringField name="lastName" label="Last Name" validate={required()} />
                <StringField name="email" label="Email Address" validate={required()} />
                <StringField name="phone" label="Phone Number" validate={required()} />
              </FormGroup>

              <FormGroup
                title="Professional Details"
                description="Your role and team affiliation within the organisation."
              >
                <SelectField
                  name="department"
                  label="Department"
                  validate={required()}
                  options={[
                    { id: 'engineering', label: 'Engineering' },
                    { id: 'product', label: 'Product' },
                    { id: 'design', label: 'Design' },
                    { id: 'data', label: 'Data Science' },
                    { id: 'operations', label: 'Operations' },
                  ]}
                />
                <StringField name="jobTitle" label="Job Title" validate={required()} />
                <StringField name="manager" label="Manager Name" validate={required()} />
              </FormGroup>

              <FormGroup
                title="Account Configuration"
                description="Settings applied to your account on creation. These can be changed later."
              >
                <SelectField
                  name="region"
                  label="Region"
                  validate={required()}
                  options={[
                    { id: 'us-east', label: 'US East' },
                    { id: 'us-west', label: 'US West' },
                    { id: 'eu-west', label: 'EU West' },
                    { id: 'ap-south', label: 'AP South' },
                  ]}
                />
                <SelectField
                  name="timezone"
                  label="Timezone"
                  validate={required()}
                  options={[
                    { id: 'utc', label: 'UTC' },
                    { id: 'us-eastern', label: 'US/Eastern' },
                    { id: 'us-pacific', label: 'US/Pacific' },
                    { id: 'europe-london', label: 'Europe/London' },
                    { id: 'asia-tokyo', label: 'Asia/Tokyo' },
                  ]}
                />
                <StringField name="slackHandle" label="Slack Handle" validate={required()} />
              </FormGroup>

              <FormErrorBanner />
              <Block display="flex" justifyContent="flex-end" marginTop="scale600">
                <Button type="submit">Submit — watch it scroll on error</Button>
              </Block>
            </Form>
          </Block>
        </Tab>
        <Tab title="ConfirmDialog">
          <Block marginTop="24px" display="flex" flexDirection="column" gridGap="scale600">
            <Block>
              <Block marginBottom="scale400">
                <strong>Basic — success after 1s</strong>
              </Block>
              <Button onClick={() => setIsBasicConfirmOpen(true)}>Open</Button>
              <ConfirmDialog
                isOpen={isBasicConfirmOpen}
                onDismiss={() => setIsBasicConfirmOpen(false)}
                heading="Delete pipeline"
                onConfirm={async () => {
                  await new Promise((resolve) => setTimeout(resolve, 1000));
                }}
                confirmLabel="Delete"
              >
                Are you sure you want to delete this pipeline? This action cannot be undone.
              </ConfirmDialog>
            </Block>

            <Block>
              <Block marginBottom="scale400">
                <strong>Error — stays open and shows error message</strong>
              </Block>
              <Button onClick={() => setIsErrorConfirmOpen(true)}>Open</Button>
              <ConfirmDialog
                isOpen={isErrorConfirmOpen}
                onDismiss={() => setIsErrorConfirmOpen(false)}
                heading="Archive model"
                onConfirm={async () => {
                  await new Promise((_, reject) =>
                    setTimeout(
                      () => reject(new Error('Server error: unable to archive at this time.')),
                      1000
                    )
                  );
                }}
                confirmLabel="Archive"
              >
                This will archive the model and remove it from active deployments.
              </ConfirmDialog>
            </Block>

            <Block>
              <Block marginBottom="scale400">
                <strong>Destructive — red confirm button</strong>
              </Block>
              <Button onClick={() => setIsDestructiveConfirmOpen(true)}>Open</Button>
              <ConfirmDialog
                isOpen={isDestructiveConfirmOpen}
                onDismiss={() => setIsDestructiveConfirmOpen(false)}
                heading="Delete pipeline"
                onConfirm={async () => {
                  await new Promise((resolve) => setTimeout(resolve, 1000));
                }}
                confirmLabel="Delete"
                destructive
              >
                Are you sure you want to delete this pipeline? This action cannot be undone.
              </ConfirmDialog>
            </Block>
          </Block>
        </Tab>
      </Tabs>
    </MainViewContainer>
  );
}
