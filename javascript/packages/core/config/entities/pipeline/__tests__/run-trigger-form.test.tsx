import { useState } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { RunTriggerForm } from '#core/config/entities/pipeline/run-trigger-form';
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

describe('RunTriggerForm', () => {
  function buildCronTrigger() {
    return { triggerType: { case: 'cronSchedule' as const, value: { cron: '0 2 * * *' } } };
  }

  function buildIntervalTrigger() {
    return {
      triggerType: { case: 'intervalSchedule' as const, value: { interval: { seconds: 3600 } } },
    };
  }

  /**
   * The form populates the dropdown from a fresh `GetPipeline` fetch rather than from the
   * record it opened with (see run-trigger-form.tsx), so this response — not the record —
   * is the source of the options. The record below deliberately carries no manifest to pin
   * that: options appearing at all proves they came from the fetch.
   */
  function buildPipelineResponse(triggerMap: Record<string, unknown>) {
    return {
      pipeline: {
        metadata: { name: 'test-pipeline', namespace: 'ma-dev-test' },
        spec: { owner: { name: 'test-owner' }, manifest: { triggerMap } },
      },
    };
  }

  // Mount-when-visible pattern: the dispatcher mounts the component while open and
  // unmounts on close. This wrapper mirrors that — unmounting on onClose.
  function FormWrapper() {
    const [mounted, setMounted] = useState(true);
    const record = {
      metadata: { name: 'test-pipeline', namespace: 'ma-dev-test' },
      spec: { owner: { name: 'test-owner' } },
    };
    if (!mounted) return null;
    return <RunTriggerForm record={record} onClose={() => setMounted(false)} />;
  }

  it('offers every trigger declared on the pipeline, labelled by its schedule', async () => {
    const user = userEvent.setup();
    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetPipeline: buildPipelineResponse({
              nightly: buildCronTrigger(),
              hourly: buildIntervalTrigger(),
            }),
          }),
        }),
      ])
    );

    await user.click(await screen.findByRole('combobox', { name: 'Trigger *' }));

    expect(await screen.findByRole('option', { name: 'hourly — every hour' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'nightly — cron 0 2 * * *' })).toBeInTheDocument();
  });

  /**
   * The schedule has to travel on `spec.trigger`. The reconciler reads it directly to pick a
   * runner (`GetTriggerType` in go/components/triggerrun/util.go) and nothing on the backend
   * resolves `sourceTriggerName`, so a payload carrying only the name would be accepted and
   * then never fire.
   */
  it('copies the selected trigger into the created TriggerRun', async () => {
    const user = userEvent.setup();
    const cronTrigger = buildCronTrigger();
    const request = createQueryMockRouter({
      GetPipeline: buildPipelineResponse({ nightly: cronTrigger }),
      CreateTriggerRun: {
        triggerRun: { metadata: { name: 'cron-nightly-20240101-120000-abcd1234' } },
      },
    });

    render(
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

    await user.click(await screen.findByRole('combobox', { name: 'Trigger *' }));
    await user.click(await screen.findByRole('option', { name: 'nightly — cron 0 2 * * *' }));

    const dialog = screen.getByRole('dialog', { name: 'Run trigger' });
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(request).toHaveBeenCalledWith(
        'CreateTriggerRun',
        {
          metadata: {
            name: expect.stringMatching(/^cron-\d{8}-\d{6}-.+$/) as string,
            namespace: 'ma-dev-test',
          },
          spec: {
            pipeline: { name: 'test-pipeline', namespace: 'ma-dev-test' },
            trigger: cronTrigger,
            sourceTriggerName: 'nightly',
            autoFlip: false,
          },
        },
        {}
      );
    });

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });

  it('shows the autoFlip choice as disabled and "Coming soon", and always sends autoFlip false', async () => {
    const user = userEvent.setup();
    const cronTrigger = buildCronTrigger();
    const request = createQueryMockRouter({
      GetPipeline: buildPipelineResponse({ nightly: cronTrigger }),
      CreateTriggerRun: {},
    });

    render(
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

    await user.click(await screen.findByRole('combobox', { name: 'Trigger *' }));
    await user.click(await screen.findByRole('option', { name: 'nightly — cron 0 2 * * *' }));

    expect(
      screen.getByText(
        'Automatically switch to the latest revision once changes are applied? (Coming soon)'
      )
    ).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Yes' })).toBeDisabled();
    expect(screen.getByRole('radio', { name: 'No' })).toBeDisabled();

    const dialog = screen.getByRole('dialog', { name: 'Run trigger' });
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(request).toHaveBeenCalledWith(
        'CreateTriggerRun',
        {
          metadata: {
            name: expect.stringMatching(/^cron-\d{8}-\d{6}-.+$/) as string,
            namespace: 'ma-dev-test',
          },
          spec: {
            pipeline: { name: 'test-pipeline', namespace: 'ma-dev-test' },
            trigger: cronTrigger,
            sourceTriggerName: 'nightly',
            autoFlip: false,
          },
        },
        {}
      );
    });
  });

  it('reveals the backfill window once the backfill toggle is enabled', async () => {
    const user = userEvent.setup();
    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetPipeline: buildPipelineResponse({ nightly: buildCronTrigger() }),
          }),
        }),
      ])
    );

    expect(screen.queryByText('Execution start date & time')).not.toBeInTheDocument();

    await user.click(await screen.findByRole('checkbox', { name: 'Is this a backfill run?' }));

    expect(screen.getByText('Execution start date & time')).toBeInTheDocument();
    expect(screen.getByText('Execution end date & time')).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Parameter IDs' })).toBeInTheDocument();
    expect(screen.getByText('Max concurrency')).toBeInTheDocument();
    expect(screen.getByRole('spinbutton')).toBeInTheDocument();
  });

  it('sends a backfill window and restricts the trigger to the selected parameters', async () => {
    const user = userEvent.setup();
    const trigger = { ...buildCronTrigger(), parametersMap: { a: {}, b: {} }, maxConcurrency: 5 };
    const request = createQueryMockRouter({
      GetPipeline: buildPipelineResponse({ nightly: trigger }),
      CreateTriggerRun: {},
    });

    render(
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

    await user.click(await screen.findByRole('combobox', { name: 'Trigger *' }));
    await user.click(await screen.findByRole('option', { name: 'nightly — cron 0 2 * * *' }));
    await user.click(screen.getByRole('checkbox', { name: 'Is this a backfill run?' }));

    const [startDateInput, endDateInput] = screen.getAllByPlaceholderText('MM/dd/yyyy');
    await user.type(startDateInput, '01/01/2024');
    await user.type(endDateInput, '01/02/2024');

    await user.click(screen.getByRole('combobox', { name: 'Parameter IDs' }));
    await user.click(await screen.findByRole('option', { name: 'a' }));

    const dialog = screen.getByRole('dialog', { name: 'Run trigger' });
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(request).toHaveBeenCalledWith(
        'CreateTriggerRun',
        {
          metadata: {
            name: expect.stringMatching(/^backfill-\d{8}-\d{6}-.+$/) as string,
            namespace: 'ma-dev-test',
          },
          spec: {
            pipeline: { name: 'test-pipeline', namespace: 'ma-dev-test' },
            trigger: { ...trigger, parametersMap: { a: {} }, maxConcurrency: 5 },
            sourceTriggerName: 'nightly',
            autoFlip: false,
            startTimestamp: { seconds: expect.any(String) as string },
            endTimestamp: { seconds: expect.any(String) as string },
          },
        },
        {}
      );
    });
  });

  it('refuses to submit until a trigger is chosen', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      GetPipeline: buildPipelineResponse({ nightly: buildCronTrigger() }),
      CreateTriggerRun: {},
    });

    render(
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

    const dialog = await screen.findByRole('dialog', { name: 'Run trigger' });
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    expect(request).not.toHaveBeenCalledWith(
      'CreateTriggerRun',
      expect.anything(),
      expect.anything()
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('explains the empty dropdown when the pipeline declares no triggers', async () => {
    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetPipeline: buildPipelineResponse({}) }),
        }),
      ])
    );

    expect(
      await screen.findByText(
        'This pipeline declares no triggers. Add one to its manifest to run it.'
      )
    ).toBeInTheDocument();
  });
});
