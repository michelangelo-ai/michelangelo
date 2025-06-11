import { Route, Routes } from 'react-router-dom-v5-compat';

import { MainViewContainer } from '#core/components/views/main-view-container';
import { ProjectDetail } from '#core/components/views/project/project-detail';
import { ProjectList } from '#core/components/views/project/project-list';

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
