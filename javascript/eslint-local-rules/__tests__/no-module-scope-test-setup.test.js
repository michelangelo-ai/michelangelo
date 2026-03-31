import { RuleTester } from 'eslint';

import rule from '../no-module-scope-test-setup.js';

RuleTester.describe = describe;
RuleTester.it = it;

const tester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    sourceType: 'module',
  },
});

tester.run('no-module-scope-test-setup', rule, {
  valid: [
    // buildWrapper called inside a test — not at module scope
    {
      name: 'buildWrapper inside it()',
      code: `it('renders', () => { render(el, buildWrapper([getBaseProviderWrapper()])); });`,
    },
    // buildWrapper called inside beforeEach — not at module scope
    {
      name: 'buildWrapper inside beforeEach()',
      code: `beforeEach(() => { setup(buildWrapper([getBaseProviderWrapper()])); });`,
    },
    // Object literal inside a test — not at module scope
    {
      name: 'object literal inside it()',
      code: `it('renders', () => { const props = { name: 'test' }; });`,
    },
    // Array literal inside a test — not at module scope
    {
      name: 'array literal inside it()',
      code: `it('renders', () => { const options = [{ value: 'a' }]; });`,
    },
    // String constant at module scope — not setup data
    {
      name: 'string constant at module scope',
      code: `const COMPONENT_NAME = 'MyComponent';`,
    },
    // Numeric constant at module scope — not setup data
    {
      name: 'numeric constant at module scope',
      code: `const MAX_RETRIES = 3;`,
    },
    // vi.mock() at module scope — ExpressionStatement, not VariableDeclaration
    {
      name: 'vi.mock() at module scope',
      code: `vi.mock('../foo', () => ({ default: () => null }));`,
    },
    // import at module scope — not a VariableDeclaration
    {
      name: 'import statement at module scope',
      code: `import { render } from '@testing-library/react';`,
    },
    {
      name: 'module-scope function unrelated to buildWrapper',
      code: `function formatLabel(name) { return name.toUpperCase(); }`,
    },
    {
      name: 'buildWrapper inside a function called inside a test',
      code: `it('renders', () => { const w = buildWrapper([getBaseProviderWrapper()]); render(el, w); });`,
    },
  ],

  invalid: [
    // buildWrapper at module scope
    {
      name: 'buildWrapper() at module scope',
      code: `const wrapper = buildWrapper([getBaseProviderWrapper()]);`,
      errors: [{ messageId: 'noModuleScopeWrapper' }],
    },
    // buildWrapper nested inside another call at module scope
    {
      name: 'buildWrapper() nested inside another call at module scope',
      code: `const wrapper = someHelper(buildWrapper([getBaseProviderWrapper()]));`,
      errors: [{ messageId: 'noModuleScopeWrapper' }],
    },
    // Object literal (props/config) at module scope
    {
      name: 'object literal at module scope',
      code: `const defaultProps = { name: 'test', value: 42 };`,
      errors: [{ messageId: 'noModuleScopeSetupConst', data: { name: 'defaultProps' } }],
    },
    // Array literal (options) at module scope
    {
      name: 'array literal at module scope',
      code: `const OPTIONS = [{ value: 'a', label: 'Option A' }];`,
      errors: [{ messageId: 'noModuleScopeSetupConst', data: { name: 'OPTIONS' } }],
    },
    // Multiple declarators in one statement — each should be flagged
    {
      name: 'multiple declarators in one const statement',
      code: `const wrapper = buildWrapper([]), options = [{ value: 'a' }];`,
      errors: [
        { messageId: 'noModuleScopeWrapper' },
        { messageId: 'noModuleScopeSetupConst', data: { name: 'options' } },
      ],
    },
    {
      name: 'function declaration wrapping buildWrapper at module scope',
      code: `function buildTestWrapper(req) { return buildWrapper([getBaseProviderWrapper(), getServiceProviderWrapper({ request: req })]); }`,
      errors: [{ messageId: 'noModuleScopeWrapperHelper', data: { name: 'buildTestWrapper' } }],
    },
    {
      name: 'arrow function variable wrapping buildWrapper at module scope',
      code: `const buildTestWrapper = (req) => buildWrapper([getBaseProviderWrapper(), getServiceProviderWrapper({ request: req })]);`,
      errors: [{ messageId: 'noModuleScopeWrapperHelper', data: { name: 'buildTestWrapper' } }],
    },
    {
      name: 'block-body arrow function variable wrapping buildWrapper at module scope',
      code: `const buildTestWrapper = (req) => { return buildWrapper([getBaseProviderWrapper()]); };`,
      errors: [{ messageId: 'noModuleScopeWrapperHelper', data: { name: 'buildTestWrapper' } }],
    },
  ],
});
