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
    // render() called directly inside a test — not wrapped in a helper
    {
      name: 'render() inside it()',
      code: `it('renders', () => { render(el, getWrapper()); });`,
    },
    {
      name: 'module-scope helper unrelated to render',
      code: `function buildRequest() { return createQueryMockRouter({}); }`,
    },
    // render() only occurs inside a nested closure the helper defines but never itself
    // invokes — the outer helper's own body never calls render, so it isn't flagged
    {
      name: 'render() only inside a nested inner closure, not called by the outer helper itself',
      code: `function setup() { const helper = () => render(el, getWrapper()); return helper; }`,
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
    // render-within-render: function declaration wrapping render() at module scope
    // (uses getWrapper() rather than buildWrapper() so the buildWrapper-helper check
    // above doesn't take priority — that case is covered by the existing subset check)
    {
      name: 'function declaration wrapping render() at module scope',
      code: `function renderDetail(req) { render(el, getWrapper()); }`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // arrow function variable wrapping render() at module scope
    {
      name: 'arrow function variable wrapping render() at module scope',
      code: `const renderDetail = (req) => { render(el, getWrapper()); };`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // render-within-render at top-level describe scope
    {
      name: 'function declaration wrapping render() at top-level describe scope',
      code: `describe('suite', () => { function renderDetail(req) { render(el, getWrapper()); } });`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // render-within-render inside a NESTED describe — unlike shared literals, this is still flagged
    // because wrapping render() in another helper is unnecessary indirection at any nesting depth
    {
      name: 'function declaration wrapping render() inside a nested describe',
      code: `describe('outer', () => { describe('inner', () => { function renderDetail(req) { render(el, buildWrapper([getBaseProviderWrapper()])); } }); });`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    {
      name: 'arrow function variable wrapping render() inside a nested describe',
      code: `describe('outer', () => { describe('inner', () => { const renderDetail = (req) => { render(el, buildWrapper([getBaseProviderWrapper()])); }; }); });`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // render() call is nested inside another call passed to the helper — still detected
    {
      name: 'render() call nested as an argument inside the helper body',
      code: `function renderDetail(req) { return someWrapperFn(render(el, getWrapper())); }`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // returning the render() call directly is still a render-within-render
    {
      name: 'function declaration that returns render() directly',
      code: `function setup() { return render(el, getWrapper()); }`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'setup' } }],
    },
    // render() hidden behind an if statement
    {
      name: 'render() called inside an if statement',
      code: `function renderDetail(cond) { if (cond) { render(el, getWrapper()); } }`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // render() hidden behind an if/else statement's alternate branch
    {
      name: 'render() called inside an if/else alternate branch',
      code: `function renderDetail(cond) { if (cond) { doSomethingElse(); } else { render(el, getWrapper()); } }`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // render() hidden behind a ternary
    {
      name: 'render() called inside a ternary expression',
      code: `function renderDetail(cond) { cond ? render(el, getWrapper()) : null; }`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // render() hidden behind a logical && short-circuit
    {
      name: 'render() called behind a logical && short-circuit',
      code: `function renderDetail(cond) { cond && render(el, getWrapper()); }`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // render() hidden inside a try block
    {
      name: 'render() called inside a try block',
      code: `function renderDetail() { try { render(el, getWrapper()); } catch (e) { handleError(e); } }`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
    // render() assigned to an intermediate variable before being used
    {
      name: 'render() called via an intermediate variable declaration',
      code: `function renderDetail() { const result = render(el, getWrapper()); return result; }`,
      errors: [{ messageId: 'noRenderWithinRenderHelper', data: { name: 'renderDetail' } }],
    },
  ],
});
