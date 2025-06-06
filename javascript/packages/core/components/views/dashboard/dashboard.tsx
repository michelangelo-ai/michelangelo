import React from 'react';
import { Card } from 'baseui/card';
import { Grid, Cell } from 'baseui/layout-grid';
import { HeadingXLarge, HeadingLarge, HeadingMedium } from 'baseui/typography';
import { Block } from 'baseui/block';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { StatCard } from './stat-card';
import { DeploymentStatusChart } from './deployment-status-chart';
import { RecentActivity } from './recent-activity';

export function Dashboard() {
  // TODO: Replace with actual data queries
  const { data: deployments } = useStudioQuery<{ deployments: any[] }>({
    queryName: 'ListDeployments',
    serviceOptions: { namespace: 'default' },
  });

  const { data: models } = useStudioQuery<{ models: any[] }>({
    queryName: 'ListModels', 
    serviceOptions: { namespace: 'default' },
  });

  // Mock data for demonstration
  const stats = {
    totalModels: models?.models?.length || 8,
    activeDeployments: deployments?.deployments?.filter(d => d.status?.state === 'HEALTHY')?.length || 3,
    totalInferences: 45678,
    uptime: '99.9%'
  };

  return (
    <Block padding="scale800">
      <HeadingXLarge marginBottom="scale800">
        Michelangelo Dashboard
      </HeadingXLarge>
      
      {/* Stats Overview */}
      <Grid gridColumns={[1, 2, 4]} gridGaps={[16, 16, 24]} marginBottom="scale800">
        <Cell>
          <StatCard 
            title="Total Models"
            value={stats.totalModels}
            subtitle="Registered models"
            trend="+2 this week"
            color="#1B7CFC"
          />
        </Cell>
        <Cell>
          <StatCard 
            title="Active Deployments" 
            value={stats.activeDeployments}
            subtitle="Currently serving"
            trend="100% healthy"
            color="#00B44A"
          />
        </Cell>
        <Cell>
          <StatCard 
            title="Total Inferences"
            value={stats.totalInferences.toLocaleString()}
            subtitle="Past 24 hours"
            trend="+12% vs yesterday"
            color="#8B5CF6"
          />
        </Cell>
        <Cell>
          <StatCard 
            title="System Uptime"
            value={stats.uptime}
            subtitle="Past 30 days"
            trend="SLA: 99.5%"
            color="#059669"
          />
        </Cell>
      </Grid>

      {/* Main Content Grid */}
      <Grid gridColumns={[1, 1, 2]} gridGaps={[16, 16, 24]}>
        <Cell>
          <Card>
            <HeadingLarge marginBottom="scale600">
              Deployment Status
            </HeadingLarge>
            <DeploymentStatusChart deployments={deployments?.deployments || []} />
          </Card>
        </Cell>
        
        <Cell>
          <Card>
            <HeadingLarge marginBottom="scale600">
              Recent Activity
            </HeadingLarge>
            <RecentActivity />
          </Card>
        </Cell>
      </Grid>

      {/* Quick Actions */}
      <Grid gridColumns={[1, 2, 3]} gridGaps={[16, 16, 24]} marginTop="scale800">
        <Cell>
          <Card
            overrides={{
              Root: {
                style: {
                  cursor: 'pointer',
                  ':hover': {
                    boxShadow: '0 4px 16px rgba(0, 0, 0, 0.12)',
                  }
                }
              }
            }}
          >
            <HeadingMedium marginBottom="scale300">Deploy New Model</HeadingMedium>
            <Block color="contentSecondary">
              Upload and deploy a new ML model to production
            </Block>
          </Card>
        </Cell>
        
        <Cell>
          <Card
            overrides={{
              Root: {
                style: {
                  cursor: 'pointer',
                  ':hover': {
                    boxShadow: '0 4px 16px rgba(0, 0, 0, 0.12)',
                  }
                }
              }
            }}
          >
            <HeadingMedium marginBottom="scale300">Monitor Deployments</HeadingMedium>
            <Block color="contentSecondary">
              View status and metrics for all active deployments
            </Block>
          </Card>
        </Cell>
        
        <Cell>
          <Card
            overrides={{
              Root: {
                style: {
                  cursor: 'pointer',
                  ':hover': {
                    boxShadow: '0 4px 16px rgba(0, 0, 0, 0.12)',
                  }
                }
              }
            }}
          >
            <HeadingMedium marginBottom="scale300">Manage Models</HeadingMedium>
            <Block color="contentSecondary">
              Browse, edit, and organize your model registry
            </Block>
          </Card>
        </Cell>
      </Grid>
    </Block>
  );
}