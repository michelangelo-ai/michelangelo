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
    // describe-scope: variables inside test hooks are allowed
    {
      name: 'variable inside it() inside describe()',
      code: `describe('suite', () => { it('renders', () => { const props = { name: 'test' }; }); });`,
    },
    {
      name: 'variable inside beforeEach() inside describe()',
      code: `describe('suite', () => { beforeEach(() => { const props = { name: 'test' }; }); });`,
    },
    {
      name: 'string constant inside describe()',
      code: `describe('suite', () => { const LABEL = 'hello'; });`,
    },
    {
      name: 'variable inside nested it() inside nested describe()',
      code: `describe('outer', () => { describe('inner', () => { it('works', () => { const props = { a: 1 }; }); }); });`,
    },
    // nested describe: shared state is allowed (semantic grouping)
    {
      name: 'object literal in nested describe scope',
      code: `describe('outer', () => { describe('inner', () => { const props = { a: 1 }; }); });`,
    },
    {
      name: 'array literal in nested describe scope',
      code: `describe('outer', () => { describe('disabled', () => { const options = [{ value: 'a' }]; }); });`,
    },
    {
      name: 'buildWrapper in nested describe scope',
      code: `describe('outer', () => { describe('inner', () => { const wrapper = buildWrapper([getBaseProviderWrapper()]); }); });`,
    },
    // function body: variables inside functions are never shared state
    {
      name: 'object literal inside function declaration at describe scope',
      code: `describe('suite', () => { function Wrapper() { const data = { name: 'test' }; } });`,
    },
    {
      name: 'object literal inside arrow function at describe scope',
      code: `describe('suite', () => { const build = () => { const data = { name: 'test' }; return data; }; });`,
    },
    // zero-arg factory that is never called from test code — no CallExpression usage
    {
      name: 'zero-arg arrow returning an object, never called',
      code: `describe('suite', () => { const buildData = () => { return { name: 'test' }; }; });`,
    },
    // factory takes a real parameter — not a zero-arg fixture
    {
      name: 'arrow factory with a parameter',
      code: `describe('suite', () => { const buildProps = (name) => ({ name }); it('renders', () => { render(buildProps('a')); }); });`,
    },
    // overrides parameter genuinely used at a call site in the same scope
    {
      name: 'overrides factory used with arguments in the same describe',
      code: `describe('suite', () => { const buildProps = (overrides = {}) => ({ name: 'a', ...overrides }); it('renders', () => { render(buildProps({ name: 'b' })); }); });`,
    },
    // overrides factory has a mix of zero-arg and with-arg call sites — real usage exists
    {
      name: 'overrides factory with mixed call sites in the same describe',
      code: `describe('suite', () => { const buildProps = (overrides = {}) => ({ name: 'a', ...overrides }); it('default', () => { render(buildProps()); }); it('custom', () => { render(buildProps({ name: 'b' })); }); });`,
    },
    // overrides factory never called at all — not our concern (unused-vars territory)
    {
      name: 'overrides factory declared but never called',
      code: `describe('suite', () => { const buildProps = (overrides = {}) => ({ name: 'a', ...overrides }); });`,
    },
    // same factory name in a sibling describe DOES pass overrides — must not affect this scope
    {
      name: 'overrides used in a sibling describe with the same factory name',
      code: `describe('outer', () => {
        describe('a', () => {
          const buildProps = (overrides = {}) => ({ name: 'a', ...overrides });
          it('uses overrides', () => { render(buildProps({ name: 'custom' })); });
        });
        describe('b', () => {
          const buildProps = (overrides = {}) => ({ name: 'a', ...overrides });
          it('no overrides here either, but different scope', () => { render(buildProps({ x: 1 })); });
        });
      });`,
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
    // describe-scope: should flag the same patterns
    {
      name: 'buildWrapper() at describe scope',
      code: `describe('suite', () => { const wrapper = buildWrapper([getBaseProviderWrapper()]); });`,
      errors: [{ messageId: 'noModuleScopeWrapper' }],
    },
    {
      name: 'object literal at describe scope',
      code: `describe('suite', () => { const defaultProps = { name: 'test', value: 42 }; });`,
      errors: [{ messageId: 'noModuleScopeSetupConst', data: { name: 'defaultProps' } }],
    },
    {
      name: 'array literal at describe scope',
      code: `describe('suite', () => { const options = [{ value: 'a' }]; });`,
      errors: [{ messageId: 'noModuleScopeSetupConst', data: { name: 'options' } }],
    },
    {
      name: 'describe.each() scope (top-level)',
      code: `describe.each([1, 2])('case %i', () => { const props = { a: 1 }; });`,
      errors: [{ messageId: 'noModuleScopeSetupConst', data: { name: 'props' } }],
    },
    {
      name: 'wrapper helper function at top-level describe scope',
      code: `describe('suite', () => { function buildTestWrapper(req) { return buildWrapper([getBaseProviderWrapper()]); } });`,
      errors: [{ messageId: 'noModuleScopeWrapperHelper', data: { name: 'buildTestWrapper' } }],
    },

    // zero-arg fixture: arrow function const, called from a test
    {
      name: 'zero-arg arrow returning an object literal, called from a test',
      code: `describe('suite', () => { const buildProps = () => ({ name: 'test' }); it('renders', () => { render(buildProps()); }); });`,
      errors: [{ messageId: 'noZeroArgFixture', data: { name: 'buildProps' } }],
    },
    // zero-arg fixture: function declaration at module scope
    {
      name: 'zero-arg function declaration returning an object, called from a test',
      code: `function buildProps() { return { name: 'test' }; } it('renders', () => { render(buildProps()); });`,
      errors: [{ messageId: 'noZeroArgFixture', data: { name: 'buildProps' } }],
    },
    // zero-arg fixture: return value is a build*/create* call rather than a literal
    {
      name: 'zero-arg function returning a factory-like call, called from a test',
      code: `function buildRequest() { return createQueryMockRouter({ GetThing: {} }); } it('renders', () => { render(buildRequest()); });`,
      errors: [{ messageId: 'noZeroArgFixture', data: { name: 'buildRequest' } }],
    },
    // zero-arg fixture: nested describe scope (not just the outermost describe)
    {
      name: 'zero-arg arrow at nested describe scope, called from a test',
      code: `describe('outer', () => { describe('inner', () => { const buildProps = () => ({ name: 'test' }); it('renders', () => { render(buildProps()); }); }); });`,
      errors: [{ messageId: 'noZeroArgFixture', data: { name: 'buildProps' } }],
    },
    // unused-overrides fixture: only call site in scope passes no arguments
    {
      name: 'overrides factory with a single zero-arg call site in the same describe',
      code: `describe('suite', () => { const buildProps = (overrides = {}) => ({ name: 'test', ...overrides }); it('renders', () => { render(buildProps()); }); });`,
      errors: [{ messageId: 'noUnusedOverridesFixture', data: { name: 'buildProps' } }],
    },
    // unused-overrides fixture: all call sites in scope pass no arguments (multiple tests)
    {
      name: 'overrides factory with multiple zero-arg call sites in the same describe',
      code: `describe('suite', () => { const buildProps = (overrides = {}) => ({ name: 'test', ...overrides }); it('a', () => { render(buildProps()); }); it('b', () => { render(buildProps()); }); });`,
      errors: [{ messageId: 'noUnusedOverridesFixture', data: { name: 'buildProps' } }],
    },
    // unused-overrides fixture: same name defined twice, only the unused copy is flagged
    {
      name: 'same factory name in two describes — only the unused-overrides copy is flagged',
      code: `describe('outer', () => {
        describe('unused', () => {
          const buildProps = (overrides = {}) => ({ name: 'a', ...overrides });
          it('never passes overrides', () => { render(buildProps()); });
        });
        describe('used', () => {
          const buildProps = (overrides = {}) => ({ name: 'a', ...overrides });
          it('passes overrides', () => { render(buildProps({ name: 'custom' })); });
        });
      });`,
      errors: [{ messageId: 'noUnusedOverridesFixture', data: { name: 'buildProps' } }],
    },
  ],
});
