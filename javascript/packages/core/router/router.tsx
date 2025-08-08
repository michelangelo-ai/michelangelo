import { Route, Routes } from 'react-router-dom-v5-compat';

import { MainViewContainer } from '#core/components/views/main-view-container';
import { ProjectDetail } from '#core/components/views/project/project-detail';
import { ProjectList } from '#core/components/views/project/project-list';
import { Sandbox } from '#core/components/views/sandbox/sandbox';
import { PhaseListRoute } from './phase-list-route';

export function Router() {
  return (
    <Routes>
      <Route
        index
        element={
          <MainViewContainer hasBreadcrumb={false}>
            <ProjectList />
          </MainViewContainer>
        }
      />
      <Route
        path="sandbox"
        element={
          <MainViewContainer hasBreadcrumb={false}>
            <Sandbox />
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
      <Route
        path=":projectId/:phase/:entity?"
        element={
          <MainViewContainer hasBreadcrumb={false}>
            <PhaseListRoute />
          </MainViewContainer>
        }
      />
    </Routes>
  );
}
