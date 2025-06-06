import React from 'react';
import { Block } from 'baseui/block';
import { Card } from 'baseui/card';
import { Grid, Cell } from 'baseui/layout-grid';
import { HeadingXLarge, HeadingLarge, HeadingMedium } from 'baseui/typography';
import { Select } from 'baseui/select';
import { MetricsChart } from './metrics-chart';
import { SystemHealth } from './system-health';
import { AlertsList } from './alerts-list';

export function MonitoringDashboard() {
  const [timeRange, setTimeRange] = React.useState([{ label: 'Last 24 hours', id: '24h' }]);
  const [selectedDeployment, setSelectedDeployment] = React.useState([{ label: 'All Deployments', id: 'all' }]);

  return (
    <Block padding="scale800">
      <Block display="flex" justifyContent="space-between" alignItems="center" marginBottom="scale800">
        <HeadingXLarge>Monitoring Dashboard</HeadingXLarge>
        <Block display="flex" gridGap="scale400">
          <Select
            options={[
              { label: 'Last 1 hour', id: '1h' },
              { label: 'Last 6 hours', id: '6h' },
              { label: 'Last 24 hours', id: '24h' },
              { label: 'Last 7 days', id: '7d' },
            ]}
            value={timeRange}
            placeholder="Select time range"
            onChange={({ value }) => setTimeRange(value)}
            clearable={false}
          />
          <Select
            options={[
              { label: 'All Deployments', id: 'all' },
              { label: 'BERT-CoLA', id: 'bert-cola' },
              { label: 'Sentiment Analyzer', id: 'sentiment' },
            ]}
            value={selectedDeployment}
            placeholder="Select deployment"
            onChange={({ value }) => setSelectedDeployment(value)}
            clearable={false}
          />
        </Block>
      </Block>

      {/* System Health Overview */}
      <Grid gridColumns={1} marginBottom="scale800">
        <Cell>
          <Card>
            <HeadingLarge marginBottom="scale600">System Health</HeadingLarge>
            <SystemHealth />
          </Card>
        </Cell>
      </Grid>

      {/* Metrics Charts */}
      <Grid gridColumns={[1, 1, 2]} gridGaps={[16, 16, 24]} marginBottom="scale800">
        <Cell>
          <Card>
            <HeadingMedium marginBottom="scale600">Request Rate</HeadingMedium>
            <MetricsChart
              type="requests"
              timeRange={timeRange[0]?.id || '24h'}
              deployment={selectedDeployment[0]?.id || 'all'}
            />
          </Card>
        </Cell>
        
        <Cell>
          <Card>
            <HeadingMedium marginBottom="scale600">Response Latency</HeadingMedium>
            <MetricsChart
              type="latency"
              timeRange={timeRange[0]?.id || '24h'}
              deployment={selectedDeployment[0]?.id || 'all'}
            />
          </Card>
        </Cell>
      </Grid>

      <Grid gridColumns={[1, 1, 2]} gridGaps={[16, 16, 24]} marginBottom="scale800">
        <Cell>
          <Card>
            <HeadingMedium marginBottom="scale600">Error Rate</HeadingMedium>
            <MetricsChart
              type="errors"
              timeRange={timeRange[0]?.id || '24h'}
              deployment={selectedDeployment[0]?.id || 'all'}
            />
          </Card>
        </Cell>
        
        <Cell>
          <Card>
            <HeadingMedium marginBottom="scale600">Resource Usage</HeadingMedium>
            <MetricsChart
              type="resources"
              timeRange={timeRange[0]?.id || '24h'}
              deployment={selectedDeployment[0]?.id || 'all'}
            />
          </Card>
        </Cell>
      </Grid>

      {/* Alerts */}
      <Grid gridColumns={1}>
        <Cell>
          <Card>
            <HeadingLarge marginBottom="scale600">Recent Alerts</HeadingLarge>
            <AlertsList />
          </Card>
        </Cell>
      </Grid>
    </Block>
  );
}