/**
 * @fileoverview Disallows module-level variable declarations used for test setup.
 *
 * Wrappers, props, and component configurations defined at the top of a test
 * file accumulate invisible shared state across tests, making failures harder
 * to reason about. Inline everything inside each test instead.
 */

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description:
        'Disallow module-level variable declarations for test setup (wrappers, props, options)',
      recommended: true,
      url: 'https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/eslint-local-rules/no-module-scope-test-setup.md',
    },
    messages: {
      noModuleScopeWrapper:
        'buildWrapper() must not be called at module scope. Move it inside each test so every test is self-contained.',

      noModuleScopeWrapperHelper:
        "'{{ name }}' wraps buildWrapper() at module scope. Move the buildWrapper() call inline into each test so every test is self-contained.",

      noModuleScopeSetupConst:
        "'{{ name }}' is declared at module scope but looks like test setup (props, options, config). Inline it inside each test, or group shared setup in a nested describe with a factory function.",

      noZeroArgFixture:
        "'{{ name }}' takes no parameters — it's a constant as a function. Inline the literal in each test, or move it to beforeEach.",

      noUnusedOverridesFixture:
        "'{{ name }}' accepts overrides but no call site in this scope passes any. Drop the parameter and inline the literal, or add meaningful overrides.",
    },
    schema: [],
  },

  create(context) {
    const TEST_HOOKS = new Set(['it', 'test', 'beforeEach', 'afterEach', 'beforeAll', 'afterAll']);

    function getCalleeName(callExpr) {
      const { callee } = callExpr;
      if (callee.type === 'Identifier') return callee.name;
      // describe.each, describe.skip, etc. — the name is on the object
      if (callee.type === 'MemberExpression' && callee.object?.type === 'Identifier') {
        return callee.object.name;
      }
      return null;
    }

    function hasParentDescribe(callExpr) {
      let current = callExpr.parent;
      while (current) {
        if (current.type === 'Program') return false;
        if (current.type === 'CallExpression') {
          const name = getCalleeName(current);
          if (name === 'describe') return true;
        }
        current = current.parent;
      }
      return false;
    }

    function isStandaloneFunction(node) {
      return node.parent?.type !== 'CallExpression';
    }

    /**
     * The outermost describe() in a file is just a wrapper — variables there are shared across all
     * tests. Nested describes are semantic groups where shared state is the intended pattern.
     *
     * Walk up the AST:
     * - test hook (it/test/beforeEach/…) → inside a test → false
     * - nested describe (has a parent describe) → semantic group → false
     * - top-level describe (no parent describe) → file wrapper → true
     * - Program → module scope → true
     */
    function isModuleScope(node) {
      let current = node.parent;
      while (current) {
        if (current.type === 'Program') return true;
        if (
          (current.type === 'FunctionDeclaration' ||
            current.type === 'FunctionExpression' ||
            current.type === 'ArrowFunctionExpression') &&
          // Only standalone functions create a real scope boundary. Test callbacks
          // (arrows passed to it/describe/etc.) are transparent to this rule.
          isStandaloneFunction(current)
        ) {
          return false;
        }
        if (current.type === 'CallExpression') {
          const name = getCalleeName(current);
          if (name && TEST_HOOKS.has(name)) return false;
          // Top-level describe is a file wrapper → module scope. Nested describe is a
          // semantic group → not module scope.
          if (name === 'describe') return !hasParentDescribe(current);
        }
        current = current.parent;
      }
      return true;
    }

    /**
     * Returns true when the initializer (or any nested call) invokes buildWrapper.
     */
    function callsBuildWrapper(node) {
      if (!node) return false;
      if (node.type === 'CallExpression') {
        const { callee } = node;
        if (callee.type === 'Identifier' && callee.name === 'buildWrapper') return true;
        // Check arguments recursively in case it's wrapped
        return node.arguments.some(callsBuildWrapper);
      }
      if (node.type === 'ArrayExpression') {
        return node.elements.some(callsBuildWrapper);
      }
      return false;
    }

    function bodyCallsBuildWrapper(node) {
      if (!node) return false;
      if (callsBuildWrapper(node)) return true;
      if (node.type === 'BlockStatement') {
        return node.body.some(bodyCallsBuildWrapper);
      }
      if (node.type === 'ReturnStatement') {
        return bodyCallsBuildWrapper(node.argument);
      }
      if (node.type === 'ExpressionStatement') {
        return bodyCallsBuildWrapper(node.expression);
      }
      return false;
    }

    /**
     * Heuristic: does this initializer look like test setup data?
     * Matches object literals, array literals, and JSX — the common shapes
     * for props / options / config objects.
     */
    function looksLikeSetupData(init) {
      if (!init) return false;
      return (
        init.type === 'ObjectExpression' ||
        init.type === 'ArrayExpression' ||
        init.type === 'JSXElement' ||
        init.type === 'JSXFragment'
      );
    }

    /**
     * True for calls like `createQueryMockRouter({ ... })` — a build-/create-prefixed
     * helper invoked with an object/array literal, the shape of a fixture-producing call.
     */
    function isFactoryLikeCall(node) {
      if (!node || node.type !== 'CallExpression') return false;
      const { callee } = node;
      if (callee.type !== 'Identifier' || !/^(build|create)/.test(callee.name)) return false;
      return node.arguments.some(
        (arg) => arg.type === 'ObjectExpression' || arg.type === 'ArrayExpression'
      );
    }

    function returnValueLooksLikeFixture(value) {
      return looksLikeSetupData(value) || isFactoryLikeCall(value);
    }

    /** Walks return statements (including simple if/else branches) collecting their arguments. */
    function collectReturnArguments(node, results) {
      if (!node) return;
      if (node.type === 'ReturnStatement') {
        results.push(node.argument);
        return;
      }
      if (node.type === 'BlockStatement') {
        for (const stmt of node.body) collectReturnArguments(stmt, results);
        return;
      }
      if (node.type === 'IfStatement') {
        collectReturnArguments(node.consequent, results);
        collectReturnArguments(node.alternate, results);
      }
    }

    /**
     * Does this function, given zero arguments, produce fixture-like data? Checks the
     * expression body of a concise arrow, or every return statement of a block body.
     */
    function functionReturnsFixtureLikeValue(fn) {
      if (fn.body.type !== 'BlockStatement') {
        return returnValueLooksLikeFixture(fn.body);
      }
      const returnArgs = [];
      collectReturnArguments(fn.body, returnArgs);
      return returnArgs.some(returnValueLooksLikeFixture);
    }

    /**
     * Like isModuleScope, but treats a describe() callback at ANY nesting depth as
     * qualifying scope — not just the outermost. Used by the zero-arg-fixture and
     * unused-overrides checks, which must catch factories hiding in nested describes.
     */
    function isDescribeOrModuleScope(node) {
      let current = node.parent;
      while (current) {
        if (current.type === 'Program') return true;
        if (
          (current.type === 'FunctionDeclaration' ||
            current.type === 'FunctionExpression' ||
            current.type === 'ArrowFunctionExpression') &&
          isStandaloneFunction(current)
        ) {
          return false;
        }
        if (current.type === 'CallExpression') {
          const name = getCalleeName(current);
          if (name && TEST_HOOKS.has(name)) return false;
          if (name === 'describe') return true;
        }
        current = current.parent;
      }
      return true;
    }

    /**
     * The nearest enclosing describe() call (or Program, if none) — used as a scope
     * key so the unused-overrides and zero-arg checks compare a factory only against
     * call sites that can actually reach its declaration, not the whole file.
     */
    function getNearestScopeContainer(node) {
      let current = node.parent;
      while (current) {
        if (current.type === 'Program') return current;
        if (current.type === 'CallExpression' && getCalleeName(current) === 'describe') {
          return current;
        }
        current = current.parent;
      }
      return null;
    }

    /**
     * True when `node` is lexically inside `scopeContainer` — i.e. a call site that
     * could actually see a factory declared in that scope. Program contains everything.
     * A describe() scope contains itself and any describe/test hook nested inside it,
     * but not a sibling describe that happens to declare a same-named factory.
     */
    function isWithinScope(node, scopeContainer) {
      if (!scopeContainer || scopeContainer.type === 'Program') return true;
      let current = node;
      while (current) {
        if (current === scopeContainer) return true;
        current = current.parent;
      }
      return false;
    }

    /** True for a single `(overrides = {})`-shaped parameter list. */
    function isSingleOverridesParam(params) {
      if (params.length !== 1) return false;
      const [param] = params;
      return (
        param.type === 'AssignmentPattern' &&
        param.right.type === 'ObjectExpression' &&
        param.right.properties.length === 0
      );
    }

    function isZeroArgFactory(fnNode) {
      if (!fnNode) return false;
      if (fnNode.type !== 'FunctionExpression' && fnNode.type !== 'ArrowFunctionExpression') {
        return false;
      }
      return fnNode.params.length === 0 && functionReturnsFixtureLikeValue(fnNode);
    }

    function isOverridesOnlyFactory(fnNode) {
      if (!fnNode) return false;
      if (fnNode.type !== 'FunctionExpression' && fnNode.type !== 'ArrowFunctionExpression') {
        return false;
      }
      return isSingleOverridesParam(fnNode.params) && functionReturnsFixtureLikeValue(fnNode);
    }

    // Factories declared with 0 params, or with a single unused `(overrides = {})` param,
    // collected during the main traversal and checked against call sites in the same
    // scope at Program:exit. A candidate is only a fixture if test code actually calls
    // it — otherwise it may be a React component only ever used as JSX (e.g. `<Foo />`).
    const zeroArgCandidates = [];
    const overrideCandidates = [];
    // Every call `foo(...)` in the file, with the scope it was found in.
    const callSites = [];

    return {
      FunctionDeclaration(node) {
        const name = node.id?.name ?? '<anonymous>';
        const inModuleScope = isModuleScope(node);
        const inDescribeScope = isDescribeOrModuleScope(node);

        if (inModuleScope && bodyCallsBuildWrapper(node.body)) {
          context.report({
            node,
            messageId: 'noModuleScopeWrapperHelper',
            data: { name },
          });
          return;
        }

        if (!inDescribeScope) return;

        if (node.params.length === 0 && functionReturnsFixtureLikeValue(node)) {
          zeroArgCandidates.push({
            name,
            reportNode: node,
            scopeContainer: getNearestScopeContainer(node),
          });
          return;
        }

        if (isOverridesOnlyFactory(node)) {
          overrideCandidates.push({
            name,
            reportNode: node,
            scopeContainer: getNearestScopeContainer(node),
          });
        }
      },

      VariableDeclaration(node) {
        const inModuleScope = isModuleScope(node);
        const inDescribeScope = isDescribeOrModuleScope(node);
        if (!inModuleScope && !inDescribeScope) return;

        for (const declarator of node.declarations) {
          const { init, id } = declarator;
          const name = id.type === 'Identifier' ? id.name : '<destructured>';

          if (inModuleScope) {
            if (callsBuildWrapper(init)) {
              context.report({
                node: declarator,
                messageId: 'noModuleScopeWrapper',
              });
              continue;
            }

            if (
              init &&
              (init.type === 'ArrowFunctionExpression' || init.type === 'FunctionExpression') &&
              bodyCallsBuildWrapper(init.body)
            ) {
              context.report({
                node: declarator,
                messageId: 'noModuleScopeWrapperHelper',
                data: { name },
              });
              continue;
            }

            if (looksLikeSetupData(init)) {
              context.report({
                node: declarator,
                messageId: 'noModuleScopeSetupConst',
                data: { name },
              });
              continue;
            }
          }

          if (!inDescribeScope) continue;

          if (isZeroArgFactory(init)) {
            zeroArgCandidates.push({
              name,
              reportNode: declarator,
              scopeContainer: getNearestScopeContainer(declarator),
            });
            continue;
          }

          if (isOverridesOnlyFactory(init)) {
            overrideCandidates.push({
              name,
              reportNode: declarator,
              scopeContainer: getNearestScopeContainer(declarator),
            });
          }
        }
      },

      CallExpression(node) {
        const { callee } = node;
        if (callee.type !== 'Identifier') return;
        callSites.push({ name: callee.name, argCount: node.arguments.length, node });
      },

      'Program:exit'() {
        for (const candidate of zeroArgCandidates) {
          const matchingCalls = callSites.filter(
            (call) =>
              call.name === candidate.name && isWithinScope(call.node, candidate.scopeContainer)
          );
          // Not actually called from test code — e.g. a React component only ever
          // referenced as JSX (`<Foo />`), which never appears as a CallExpression.
          if (matchingCalls.length === 0) continue;
          context.report({
            node: candidate.reportNode,
            messageId: 'noZeroArgFixture',
            data: { name: candidate.name },
          });
        }

        for (const candidate of overrideCandidates) {
          const matchingCalls = callSites.filter(
            (call) =>
              call.name === candidate.name && isWithinScope(call.node, candidate.scopeContainer)
          );
          if (matchingCalls.length === 0) continue;
          if (matchingCalls.every((call) => call.argCount === 0)) {
            context.report({
              node: candidate.reportNode,
              messageId: 'noUnusedOverridesFixture',
              data: { name: candidate.name },
            });
          }
        }
      },
    };
  },
};

export default rule;
