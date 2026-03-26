import { RuleTester } from 'eslint';

import rule from '../no-fixture-constants.js';

RuleTester.describe = describe;
RuleTester.it = it;

const tester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    sourceType: 'module',
  },
});

tester.run('no-fixture-constants', rule, {
  valid: [
    {
      name: 'arrow function export',
      code: `export const createUser = () => ({ id: '1', name: 'Alice' });`,
    },
    {
      name: 'arrow function with overrides parameter',
      code: `export const createUser = (overrides = {}) => ({ id: '1', ...overrides });`,
    },
    {
      name: 'function expression export',
      code: `export const createUser = function(overrides = {}) { return { id: '1', ...overrides }; };`,
    },
    {
      name: 'function declaration export',
      code: `export function createUser(overrides = {}) { return { id: '1', ...overrides }; }`,
    },
    {
      name: 'non-exported object constant',
      code: `const defaults = { id: '1' };`,
    },
    {
      name: 're-export of another module',
      code: `export { createUser } from './user-factory';`,
    },
  ],

  invalid: [
    {
      name: 'exported object literal',
      code: `export const MOCK_USER = { id: '1', name: 'Alice' };`,
      errors: [{ messageId: 'noFixtureConstant', data: { name: 'MOCK_USER' } }],
    },
    {
      name: 'exported array literal',
      code: `export const MOCK_IDS = [1, 2, 3];`,
      errors: [{ messageId: 'noFixtureConstant', data: { name: 'MOCK_IDS' } }],
    },
    {
      name: 'exported string constant',
      code: `export const BASE_URL = 'http://localhost';`,
      errors: [{ messageId: 'noFixtureConstant', data: { name: 'BASE_URL' } }],
    },
    {
      name: 'exported numeric constant',
      code: `export const COUNT = 3;`,
      errors: [{ messageId: 'noFixtureConstant', data: { name: 'COUNT' } }],
    },
    {
      name: 'exported call expression',
      code: `export const mockFn = vi.fn();`,
      errors: [{ messageId: 'noFixtureConstant', data: { name: 'mockFn' } }],
    },
  ],
});
