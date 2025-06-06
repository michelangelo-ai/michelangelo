import { Route, Routes } from 'react-router-dom';

import { MainViewContainer } from '#core/components/views/main-view-container';
import { ProjectDetail } from '#core/components/views/project/project-detail';
import { Dashboard } from '#core/components/views/dashboard/dashboard';
import { DeploymentsList } from '#core/components/views/deployments/deployments-list';
import { ModelsList } from '#core/components/views/models/models-list';
import { MonitoringDashboard } from '#core/components/views/monitoring/monitoring-dashboard';

export function Router() {
  return (
    <Routes>
      <Route
        path="/"
        element={
          <MainViewContainer hasBreadcrumb={false}>
            <Dashboard />
          </MainViewContainer>
        }
      />
      <Route
        path="/dashboard"
        element={
          <MainViewContainer hasBreadcrumb={false}>
            <Dashboard />
          </MainViewContainer>
        }
      />
      <Route
        path="/deployments"
        element={
          <MainViewContainer hasBreadcrumb={false}>
            <DeploymentsList />
          </MainViewContainer>
        }
      />
      <Route
        path="/models"
        element={
          <MainViewContainer hasBreadcrumb={false}>
            <ModelsList />
          </MainViewContainer>
        }
      />
      <Route
        path="/monitoring"
        element={
          <MainViewContainer hasBreadcrumb={false}>
            <MonitoringDashboard />
          </MainViewContainer>
        }
      />
      <Route
        path=":projectId"
        element={
          <MainViewContainer hasBreadcrumb={false}>
            <ProjectDetail />
          </MainViewContainer>
        }
      />
    </Routes>
  );
}
