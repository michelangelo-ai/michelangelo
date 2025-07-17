/**
 * Shared mock context objects for interpolation testing
 */

export const createMockUser = (
  overrides: Partial<{
    uuid: string;
    email: string;
    username: string;
  }> = {}
) => ({
  uuid: 'test-uuid',
  email: 'test@uber.com',
  username: 'testuser',
  ...overrides,
});

export const createMockProject = (
  overrides: Partial<{
    id: string;
    name: string;
  }> = {}
) => ({
  id: 'test-project',
  name: 'Test Project',
  ...overrides,
});
