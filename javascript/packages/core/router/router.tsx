import { Route, Routes } from 'react-router-dom';

import { MainViewContainer } from '#core/components/views/main-view-container';
import { ProjectDetail } from '#core/components/views/project/project-detail';

export function Router() {
  return (
    <Routes>
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
