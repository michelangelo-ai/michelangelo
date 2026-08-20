import { useState } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { SchedulePipelineForm } from '#core/config/entities/pipeline/schedule-pipeline-form';
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

describe('SchedulePipelineForm', () => {
  function buildCronTrigger() {
    return { triggerType: { case: 'cronSchedule' as const, value: { cron: '0 2 * * *' } } };
  }

  /**
   * Only `GetPipeline` carries `spec.manifest`, so this response is the sole source of the
   * dropdown's options — the record the action opens with has no triggers on it.
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
    return <SchedulePipelineForm record={record} onClose={() => setMounted(false)} />;
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
              hourly: {
                triggerType: { case: 'intervalSchedule', value: { interval: { seconds: 3600 } } },
              },
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
      CreateTriggerRun: { triggerRun: { metadata: { name: 'nightly-20240101-120000-abcd1234' } } },
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

    const dialog = screen.getByRole('dialog', { name: 'Schedule pipeline' });
    await user.click(within(dialog).getByRole('button', { name: 'Schedule' }));

    await waitFor(() => {
      expect(request).toHaveBeenCalledWith(
        'CreateTriggerRun',
        {
          metadata: {
            name: expect.stringMatching(/^nightly-\d{8}-\d{6}-.+$/) as string,
            namespace: 'ma-dev-test',
          },
          spec: {
            pipeline: { name: 'test-pipeline', namespace: 'ma-dev-test' },
            trigger: cronTrigger,
            sourceTriggerName: 'nightly',
          },
        },
        {}
      );
    });

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
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

    const dialog = await screen.findByRole('dialog', { name: 'Schedule pipeline' });
    await user.click(within(dialog).getByRole('button', { name: 'Schedule' }));

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
        'This pipeline declares no triggers. Add one to its manifest to schedule it.'
      )
    ).toBeInTheDocument();
  });
});
