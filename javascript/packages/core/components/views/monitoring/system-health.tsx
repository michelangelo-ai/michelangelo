import React from 'react';
import { Block } from 'baseui/block';
import { Grid, Cell } from 'baseui/layout-grid';
import { ParagraphMedium, LabelSmall } from 'baseui/typography';
import { ProgressBar } from 'baseui/progress-bar';

export function SystemHealth() {
  const healthMetrics = [
    {
      name: 'API Server',
      status: 'healthy',
      uptime: '99.9%',
      responseTime: '45ms',
      load: 23
    },
    {
      name: 'Model Serving',
      status: 'healthy', 
      uptime: '99.7%',
      responseTime: '120ms',
      load: 67
    },
    {
      name: 'Database',
      status: 'healthy',
      uptime: '100%',
      responseTime: '8ms',
      load: 34
    },
    {
      name: 'Storage',
      status: 'warning',
      uptime: '99.2%',
      responseTime: '200ms',
      load: 89
    }
  ];

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'healthy': return '#00B44A';
      case 'warning': return '#F59E0B';
      case 'error': return '#EF4444';
      default: return '#6B7280';
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'healthy': return '✅';
      case 'warning': return '⚠️';
      case 'error': return '❌';
      default: return '⚪';
    }
  };

  return (
    <Grid gridColumns={[1, 2, 4]} gridGaps={[16, 16, 24]}>
      {healthMetrics.map((metric) => (
        <Cell key={metric.name}>
          <Block
            padding="scale600"
            backgroundColor="#F8FAFC"
            borderRadius="8px"
            border={`1px solid ${metric.status === 'warning' ? '#F59E0B' : '#E2E8F0'}`}
          >
            <Block display="flex" alignItems="center" marginBottom="scale400">
              <Block marginRight="scale300" fontSize="16px">
                {getStatusIcon(metric.status)}
              </Block>
              <ParagraphMedium margin="0" font="font600">
                {metric.name}
              </ParagraphMedium>
            </Block>

            <Block marginBottom="scale300">
              <Block display="flex" justifyContent="space-between" marginBottom="scale200">
                <LabelSmall color="contentSecondary">Load</LabelSmall>
                <LabelSmall color="contentSecondary">{metric.load}%</LabelSmall>
              </Block>
              <ProgressBar
                value={metric.load}
                overrides={{
                  BarProgress: {
                    style: {
                      backgroundColor: metric.load > 80 ? '#EF4444' : metric.load > 60 ? '#F59E0B' : '#00B44A',
                    }
                  }
                }}
              />
            </Block>

            <Block>
              <Block display="flex" justifyContent="space-between" marginBottom="scale100">
                <LabelSmall color="contentSecondary">Uptime</LabelSmall>
                <LabelSmall color={getStatusColor(metric.status)}>
                  {metric.uptime}
                </LabelSmall>
              </Block>
              <Block display="flex" justifyContent="space-between">
                <LabelSmall color="contentSecondary">Response</LabelSmall>
                <LabelSmall color="contentSecondary">
                  {metric.responseTime}
                </LabelSmall>
              </Block>
            </Block>
          </Block>
        </Cell>
      ))}
    </Grid>
  );
}