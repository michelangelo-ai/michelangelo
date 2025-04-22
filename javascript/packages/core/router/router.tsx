import { Route, Routes } from 'react-router';
import { BrowserRouter } from 'react-router-dom';

import { MainViewContainer } from '#core/components/views/main-view-container';
import { ProjectDetail } from '#core/components/views/project/project-detail';

export function Router() {
  return (
    <BrowserRouter basename={'/'}>
      <Routes>
        <Route
          path={':projectId'}
          element={
            <MainViewContainer hasBreadcrumb={false}>
              <ProjectDetail />
            </MainViewContainer>
          }
        />
      </Routes>
    </BrowserRouter>
  );
}
