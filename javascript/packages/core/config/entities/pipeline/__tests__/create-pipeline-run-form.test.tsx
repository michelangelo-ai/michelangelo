import { useState } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CreatePipelineRunForm } from '#core/config/entities/pipeline/create-pipeline-run-form';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

import type { PipelineRun } from '#core/config/entities/run/types';

describe('CreatePipelineRunForm', () => {
  // Mount-when-visible pattern: the dispatcher mounts the component while open and
  // unmounts on close. This wrapper mirrors that — unmounting on onClose.
  function FormWrapper() {
    const [mounted, setMounted] = useState(true);
    const data = {
      metadata: { name: 'test-pipeline', namespace: 'test-namespace' },
      spec: { owner: { name: 'test-owner' } },
    };
    if (!mounted) return null;
    return <CreatePipelineRunForm record={data} onClose={() => setMounted(false)} />;
  }

  it('submits pipeline run with correct data structure and closes dialog', async () => {
    const user = userEvent.setup();
    const mockResponse = { pipelineRun: { metadata: { name: 'created-run' } } };
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
    const submitButton = within(dialog).getByRole('button', { name: 'Run' });
    await user.click(submitButton);

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({
          metadata: expect.objectContaining({
            name: expect.stringMatching(/^run-\d{8}-\d{6}-.+$/) as string,
            namespace: 'ma-dev-test',
          }) as Record<string, unknown>,
          spec: expect.objectContaining({
            pipeline: {
              name: 'test-pipeline',
              namespace: 'ma-dev-test',
            },
          }) as Record<string, unknown>,
        }),
        {}
      );
    });

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });

  it('leaves notifications empty when the toggle is left off', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ CreatePipelineRun: {} });

    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({
          spec: expect.objectContaining({ notifications: [] }) as Record<string, unknown>,
        }),
        {}
      );
    });
  });

  it('submits an email notification covering every event type once opted in', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ CreatePipelineRun: {} });

    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });

    await user.click(
      within(dialog).getByRole('checkbox', {
        name: 'Do you want to receive notifications when pipeline run completed?',
      })
    );
    await user.type(within(dialog).getByPlaceholderText('name@example.com'), 'oncall@example.com');
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({
          spec: expect.objectContaining({
            notifications: [
              {
                notification_type: 'NOTIFICATION_TYPE_EMAIL',
                event_types: [
                  'EVENT_TYPE_PIPELINE_RUN_STATE_STARTED',
                  'EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED',
                  'EVENT_TYPE_PIPELINE_RUN_STATE_FAILED',
                  'EVENT_TYPE_PIPELINE_RUN_STATE_KILLED',
                  'EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED',
                ],
                resource_type: 'RESOURCE_TYPE_PIPELINE_RUN',
                emails: ['oncall@example.com'],
                slack_destinations: [],
              },
            ],
          }) as Record<string, unknown>,
        }),
        {}
      );
    });
  });

  it('submits both an email and a Slack notification when both are filled in', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ CreatePipelineRun: {} });

    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });

    await user.click(
      within(dialog).getByRole('checkbox', {
        name: 'Do you want to receive notifications when pipeline run completed?',
      })
    );
    await user.type(within(dialog).getByPlaceholderText('name@example.com'), 'oncall@example.com');
    await user.type(within(dialog).getByPlaceholderText('#channel or @user'), '#ml-oncall');
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({
          spec: expect.objectContaining({
            notifications: [
              expect.objectContaining({
                notification_type: 'NOTIFICATION_TYPE_EMAIL',
                emails: ['oncall@example.com'],
              }) as Record<string, unknown>,
              expect.objectContaining({
                notification_type: 'NOTIFICATION_TYPE_SLACK',
                slack_destinations: ['#ml-oncall'],
              }) as Record<string, unknown>,
            ],
          }) as Record<string, unknown>,
        }),
        {}
      );
    });
  });

  it('omits a destination type left blank even when the toggle is on', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ CreatePipelineRun: {} });

    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });

    await user.click(
      within(dialog).getByRole('checkbox', {
        name: 'Do you want to receive notifications when pipeline run completed?',
      })
    );
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({
          spec: expect.objectContaining({ notifications: [] }) as Record<string, unknown>,
        }),
        {}
      );
    });
  });

  it('keeps dialog open and displays error when submission fails', async () => {
    const user = userEvent.setup();
    const mockError = new Error('Test error');
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockError,
    });

    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog');
    const submitButton = within(dialog).getByRole('button', { name: 'Run' });
    await user.click(submitButton);

    await screen.findByText(/Test error/);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('includes description in payload when filled in', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ CreatePipelineRun: {} });

    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
    await user.type(
      screen.getByRole('textbox', { name: /description/i }),
      'nightly evaluation run'
    );
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({
          spec: expect.objectContaining({
            description: 'nightly evaluation run',
          }) as Record<string, unknown>,
        }),
        {}
      );
    });
  });

  describe('resuming a previous run', () => {
    const SOURCE_RUN = 'run-20260817-a3f9';

    /**
     * A finished run of `test-pipeline`, plus two runs the picker must exclude: one
     * belonging to a different pipeline, one still running.
     */
    const runListResponse = {
      pipelineRunList: {
        items: [
          {
            metadata: { name: SOURCE_RUN, creationTimestamp: { seconds: '1755440000' } },
            spec: { pipeline: { name: 'test-pipeline' } },
            status: { state: 5 },
          },
          {
            metadata: { name: 'run-other-pipeline', creationTimestamp: { seconds: '1755450000' } },
            spec: { pipeline: { name: 'some-other-pipeline' } },
            status: { state: 3 },
          },
          {
            metadata: { name: 'run-still-running', creationTimestamp: { seconds: '1755460000' } },
            spec: { pipeline: { name: 'test-pipeline' } },
            status: { state: 2 },
          },
        ],
      },
    };

    /**
     * Sub-steps of "Execute Workflow" are the DAG tasks. `name` holds the task path and
     * `displayName` the task name — only the latter is a valid `resumeFrom` value.
     */
    const sourceRunResponse = {
      pipelineRun: {
        metadata: { name: SOURCE_RUN },
        status: {
          steps: [
            { name: 'Image Build', displayName: 'Image Build', state: 3 },
            {
              name: 'Execute Workflow',
              displayName: 'Execute Workflow',
              state: 5,
              subSteps: [
                {
                  name: 'tasks/feature_gen',
                  displayName: 'feature_gen',
                  state: 5,
                  startTime: { seconds: '1755440100' },
                  endTime: { seconds: '1755440652' },
                },
                {
                  name: 'tasks/train_model',
                  displayName: 'train_model',
                  state: 6,
                },
              ],
            },
          ],
        },
      },
    };

    function renderForm(request: ReturnType<typeof createQueryMockRouter>) {
      return render(
        <FormWrapper />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getErrorProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
          getServiceProviderWrapper({ request }),
        ])
      );
    }

    function buildRequest() {
      return createQueryMockRouter({
        CreatePipelineRun: {},
        ListPipelineRun: runListResponse,
        GetPipelineRun: sourceRunResponse,
      });
    }

    async function openResumeGroup(user: ReturnType<typeof userEvent.setup>) {
      await user.click(screen.getByText('Select run to resume from'));
    }

    async function selectSourceRun(user: ReturnType<typeof userEvent.setup>) {
      await user.click(await screen.findByRole('combobox', { name: /pipeline run/i }));
      await user.click(await screen.findByText(new RegExp(SOURCE_RUN)));
    }

    it('offers only finished runs of this pipeline', async () => {
      const user = userEvent.setup();
      renderForm(buildRequest());

      await screen.findByRole('dialog', { name: 'Start new pipeline run' });
      await openResumeGroup(user);
      await user.click(await screen.findByRole('combobox', { name: /pipeline run/i }));

      expect(await screen.findByText(new RegExp(SOURCE_RUN))).toBeInTheDocument();
      // Excluded: belongs to another pipeline, and is still running
      expect(screen.queryByText(/run-other-pipeline/)).not.toBeInTheDocument();
      expect(screen.queryByText(/run-still-running/)).not.toBeInTheDocument();
    });

    it('populates the step picker from Execute Workflow sub-steps once a run is chosen', async () => {
      const user = userEvent.setup();
      renderForm(buildRequest());

      await screen.findByRole('dialog', { name: 'Start new pipeline run' });
      await openResumeGroup(user);

      // Disabled until a source run supplies the option list
      expect(screen.getByRole('combobox', { name: /steps/i })).toBeDisabled();

      await selectSourceRun(user);

      const stepPicker = await screen.findByRole('combobox', { name: /steps/i });
      await waitFor(() => expect(stepPicker).not.toBeDisabled());
      await user.click(stepPicker);

      expect(await screen.findByText('feature_gen')).toBeInTheDocument();
      expect(screen.getByText('train_model')).toBeInTheDocument();
      // Platform stages are not resumable DAG tasks
      expect(screen.queryByText('Image Build')).not.toBeInTheDocument();
    });

    it('submits resumeFrom using step displayName, not the task path', async () => {
      const user = userEvent.setup();
      const mockRequest = buildRequest();
      renderForm(mockRequest);

      const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
      await openResumeGroup(user);
      await selectSourceRun(user);

      const stepPicker = await screen.findByRole('combobox', { name: /steps/i });
      await waitFor(() => expect(stepPicker).not.toBeDisabled());
      await user.click(stepPicker);
      await user.click(await screen.findByText('feature_gen'));

      await user.click(within(dialog).getByRole('button', { name: 'Run' }));

      await waitFor(() => {
        expect(mockRequest).toHaveBeenCalledWith(
          'CreatePipelineRun',
          expect.objectContaining({
            spec: expect.objectContaining({
              resume: {
                pipelineRun: { name: SOURCE_RUN, namespace: 'ma-dev-test' },
                resumeFrom: ['feature_gen'],
              },
            }) as Record<string, unknown>,
          }),
          {}
        );
      });
    });

    it('submits resume without resumeFrom when no step is picked', async () => {
      const user = userEvent.setup();
      const mockRequest = buildRequest();
      renderForm(mockRequest);

      const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
      await openResumeGroup(user);
      await selectSourceRun(user);
      await user.click(within(dialog).getByRole('button', { name: 'Run' }));

      await waitFor(() => {
        expect(mockRequest).toHaveBeenCalledWith(
          'CreatePipelineRun',
          expect.objectContaining({
            spec: expect.objectContaining({
              resume: { pipelineRun: { name: SOURCE_RUN, namespace: 'ma-dev-test' } },
            }) as Record<string, unknown>,
          }),
          {}
        );
      });
    });

    it('omits the resume spec entirely when the group is opened but nothing is chosen', async () => {
      const user = userEvent.setup();
      const router = buildRequest();

      // Captures the payload so the assertion can check for the *absence* of a key,
      // which call matchers express poorly.
      const submitted: PipelineRun[] = [];
      const request: typeof router = (queryName, args, headers) => {
        if (queryName === 'CreatePipelineRun') {
          submitted.push(args as PipelineRun);
        }
        return router(queryName, args, headers);
      };

      renderForm(request);

      const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
      await openResumeGroup(user);
      await user.click(within(dialog).getByRole('button', { name: 'Run' }));

      await waitFor(() => {
        expect(submitted).toHaveLength(1);
      });
      expect(submitted[0].spec.resume).toBeUndefined();
    });
  });
});
