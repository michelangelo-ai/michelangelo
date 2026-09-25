import { render, screen, waitFor, within } from '@testing-library/react';

import { TRIGGERED_BY_LABEL } from '#core/config/entities/run/shared';
import { RETRAIN_PHASE } from '#core/config/phases/retrain';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

describe('Trigger detail "Triggered Runs"', () => {
  const SELECTOR = `${TRIGGERED_BY_LABEL}=nightly-trigger`;

  /**
   * The runs this trigger produced are found only through the label selector. The
   * storage layer drops `listOptions.labelSelector` outright if a caller ever also
   * sets `listOptionsExt.operation` (go/storage/mysql/mysql.go), which would silently
   * turn this tab into a list of every run in the namespace. Pin the exact request.
   */
  it('lists runs filtered by the triggered-by label for this trigger', async () => {
    const request = createQueryMockRouter({
      GetTriggerRun: {
        triggerRun: {
          metadata: { name: 'nightly-trigger', namespace: 'myproject' },
          spec: { pipeline: { name: 'my-pipeline', namespace: 'myproject' } },
          status: { state: 1 },
        },
      },
      [`ListPipelineRun:{"listOptions":{"labelSelector":"${SELECTOR}"},"namespace":"myproject"}`]: {
        pipelineRunList: {
          items: [
            {
              metadata: { name: 'run-1', labels: { [TRIGGERED_BY_LABEL]: 'nightly-trigger' } },
              status: { state: 3 },
            },
          ],
        },
      },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({ request }),
      ])
    );

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        'ListPipelineRun',
        { namespace: 'myproject', listOptions: { labelSelector: SELECTOR } },
        {}
      )
    );

    await screen.findByRole('row', { name: /run-1/ });
  });

  it('omits the redundant "Triggered by" column, since every row shares this trigger', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetTriggerRun: {
              triggerRun: {
                metadata: { name: 'nightly-trigger', namespace: 'myproject' },
                spec: { pipeline: { name: 'my-pipeline', namespace: 'myproject' } },
                status: { state: 1 },
              },
            },
            [`ListPipelineRun:{"listOptions":{"labelSelector":"${SELECTOR}"},"namespace":"myproject"}`]:
              {
                pipelineRunList: {
                  items: [
                    {
                      metadata: {
                        name: 'run-1',
                        labels: { [TRIGGERED_BY_LABEL]: 'nightly-trigger' },
                      },
                      status: { state: 3 },
                    },
                  ],
                },
              },
          }),
        }),
      ])
    );

    await screen.findByRole('row', { name: /run-1/ });
    expect(screen.queryByRole('columnheader', { name: 'Triggered by' })).not.toBeInTheDocument();
  });

  /**
   * Data for the mocked `GetTriggerRun`/`ListPipelineRun` responses this tab issues, given
   * the rows to return for the (fixed) label-selector query. Pure data only — no rendering —
   * so each test still performs its own `render`/`buildWrapper` call directly.
   */
  const buildRunsListMocks = (items: Record<string, unknown>[]) => ({
    GetTriggerRun: {
      triggerRun: {
        metadata: { name: 'nightly-trigger', namespace: 'myproject' },
        spec: { pipeline: { name: 'my-pipeline', namespace: 'myproject' } },
        status: { state: 1 },
      },
    },
    [`ListPipelineRun:{"listOptions":{"labelSelector":"${SELECTOR}"},"namespace":"myproject"}`]: {
      pipelineRunList: { items },
    },
  });

  it('renders all 9 target column headers, in order', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter(
            buildRunsListMocks([{ metadata: { name: 'run-1' }, status: { state: 3 } }])
          ),
        }),
      ])
    );

    await screen.findByRole('row', { name: /run-1/ });

    const headers = screen.getAllByRole('columnheader');
    // Trailing columns with no text content are table chrome (e.g. a row-actions column),
    // not a named data column, so they're excluded from the ordering assertion — same
    // convention as `run/__tests__/run.test.tsx`'s equivalent header-order test.
    const headerLabels = headers.map((header) => header.textContent).filter(Boolean);
    expect(headerLabels).toEqual([
      'Pipeline run name',
      'Pipeline',
      'Last updated',
      'Parameter ID',
      'Execution Timestamp',
      'Environment',
      'Resume from',
      'Owner',
      'State',
    ]);
  });

  it('links "Pipeline run name" to the run detail route, unchanged by the relabel', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter(
            buildRunsListMocks([{ metadata: { name: 'run-1' }, status: { state: 3 } }])
          ),
        }),
      ])
    );

    const link = await screen.findByRole('link', { name: 'run-1' });
    expect(link).toHaveAttribute('href', '/myproject/retrain/runs/run-1');
  });

  it('renders "Last updated" from the SpecUpdateTimestamp label, falling back to creation time', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter(
            buildRunsListMocks([
              {
                metadata: {
                  name: 'run-with-label',
                  labels: { 'michelangelo/SpecUpdateTimestamp': '1700000000000000' },
                },
                status: { state: 3 },
              },
              {
                metadata: {
                  name: 'run-without-label',
                  creationTimestamp: { seconds: 1650000000 },
                },
                status: { state: 3 },
              },
            ])
          ),
        }),
      ])
    );

    const rowWithLabel = await screen.findByRole('row', { name: /run-with-label/ });
    const rowWithoutLabel = await screen.findByRole('row', { name: /run-without-label/ });
    // Column index 2 is "Last updated"; both values should resolve to a real formatted
    // date (not blank, not "Invalid date"), and the label-derived and fallback values
    // should differ since they come from different underlying timestamps.
    const lastUpdatedWithLabel = within(rowWithLabel).getAllByRole('cell')[2].textContent;
    const lastUpdatedWithoutLabel = within(rowWithoutLabel).getAllByRole('cell')[2].textContent;
    expect(lastUpdatedWithLabel).toMatch(/\d{4}\/\d{2}\/\d{2}/);
    expect(lastUpdatedWithoutLabel).toMatch(/\d{4}\/\d{2}\/\d{2}/);
    expect(lastUpdatedWithLabel).not.toEqual(lastUpdatedWithoutLabel);
  });

  it('renders "Parameter ID" from the raw label, blank when absent', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter(
            buildRunsListMocks([
              {
                metadata: {
                  name: 'run-with-param',
                  labels: { 'pipelinerun.michelangelo/parameter-id': 'param-42' },
                },
                status: { state: 3 },
              },
              { metadata: { name: 'run-without-param' }, status: { state: 3 } },
            ])
          ),
        }),
      ])
    );

    const rowWithParam = await screen.findByRole('row', { name: /run-with-param/ });
    const rowWithoutParam = await screen.findByRole('row', { name: /run-without-param/ });
    // Column index 3 is "Parameter ID".
    expect(within(rowWithParam).getAllByRole('cell')[3]).toHaveTextContent('param-42');
    expect(within(rowWithoutParam).getAllByRole('cell')[3]).toHaveTextContent('—');
  });

  it('renders "Execution Timestamp" from its label, falling back to creation time', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter(
            buildRunsListMocks([
              {
                metadata: {
                  name: 'run-with-exec-ts',
                  labels: { 'pipelinerun.michelangelo/execution-timestamp': '1700000000' },
                },
                status: { state: 3 },
              },
              {
                metadata: {
                  name: 'run-without-exec-ts',
                  creationTimestamp: { seconds: 1650000000 },
                },
                status: { state: 3 },
              },
            ])
          ),
        }),
      ])
    );

    const rowWithLabel = await screen.findByRole('row', { name: /run-with-exec-ts/ });
    const rowWithoutLabel = await screen.findByRole('row', { name: /run-without-exec-ts/ });
    // Column index 4 is "Execution Timestamp".
    const execTsWithLabel = within(rowWithLabel).getAllByRole('cell')[4].textContent;
    const execTsWithoutLabel = within(rowWithoutLabel).getAllByRole('cell')[4].textContent;
    expect(execTsWithLabel).toMatch(/\d{4}\/\d{2}\/\d{2}/);
    expect(execTsWithoutLabel).toMatch(/\d{4}\/\d{2}\/\d{2}/);
    expect(execTsWithLabel).not.toEqual(execTsWithoutLabel);
  });

  it('renders "Environment" from its normalized label, blank when absent', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter(
            buildRunsListMocks([
              {
                metadata: {
                  name: 'run-in-prod',
                  labels: { 'michelangelo/environment': 'production' },
                },
                status: { state: 3 },
              },
              { metadata: { name: 'run-no-env' }, status: { state: 3 } },
            ])
          ),
        }),
      ])
    );

    const rowInProd = await screen.findByRole('row', { name: /run-in-prod/ });
    const rowNoEnv = await screen.findByRole('row', { name: /run-no-env/ });
    // Column index 5 is "Environment".
    expect(within(rowInProd).getAllByRole('cell')[5]).toHaveTextContent('Production');
    expect(within(rowNoEnv).getAllByRole('cell')[5]).toHaveTextContent('—');
  });

  it('renders "Resume from" for a resumed run, blank for a non-resume run', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter(
            buildRunsListMocks([
              {
                metadata: { name: 'run-resumed' },
                spec: { resume: { pipelineRun: { name: 'run-original' } } },
                status: { state: 3 },
              },
              { metadata: { name: 'run-fresh' }, status: { state: 3 } },
            ])
          ),
        }),
      ])
    );

    const rowResumed = await screen.findByRole('row', { name: /run-resumed/ });
    const rowFresh = await screen.findByRole('row', { name: /run-fresh/ });
    // Column index 6 is "Resume from".
    expect(within(rowResumed).getAllByRole('cell')[6]).toHaveTextContent('run-original');
    expect(within(rowFresh).getAllByRole('cell')[6]).toHaveTextContent('—');
  });

  it('renders the actor under the "Owner" header, unchanged by the relabel', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter(
            buildRunsListMocks([
              {
                metadata: { name: 'run-1' },
                spec: { actor: { name: 'owner-name' } },
                status: { state: 3 },
              },
            ])
          ),
        }),
      ])
    );

    const row = await screen.findByRole('row', { name: /run-1/ });
    expect(row).toHaveTextContent('owner-name');
  });

  it('renders the synthetic "Killing" state only while the real state has not yet reached KILLED', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter(
            buildRunsListMocks([
              {
                metadata: { name: 'run-killing' },
                spec: { kill: true },
                status: { state: 2 }, // RUNNING, kill requested but not yet applied
              },
              {
                metadata: { name: 'run-not-killed' },
                status: { state: 2 }, // RUNNING, no kill requested
              },
              {
                metadata: { name: 'run-already-killed' },
                spec: { kill: true },
                status: { state: 4 }, // KILLED — must not show "Killing" once state catches up
              },
            ])
          ),
        }),
      ])
    );

    const killingRow = await screen.findByRole('row', { name: /run-killing/ });
    expect(killingRow).toHaveTextContent('Killing');

    const runningRow = await screen.findByRole('row', { name: /run-not-killed/ });
    expect(runningRow).toHaveTextContent('Running');
    expect(runningRow).not.toHaveTextContent('Killing');

    const killedRow = await screen.findByRole('row', { name: /run-already-killed/ });
    expect(killedRow).toHaveTextContent('Killed');
    expect(killedRow).not.toHaveTextContent('Killing');
  });
});
