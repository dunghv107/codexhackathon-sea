# React + TypeScript + Vite

## Tests and coverage

Run `npm ci`, then `npm test`. Vitest runs component and API contract tests in
jsdom and collects V8 coverage for every TypeScript source file in `src/`,
including the application entry point. Lines, branches, functions and statements
must each reach 90%. `coverage/lcov.info` is imported by SonarQube; JSON and HTML
reports are also generated locally. The Docker `test` stage exports the same
LCOV report as `/reports/coverage-frontend.lcov`.

Tests exercise real components, stores and API functions with controlled HTTP
responses. They cover checkout, token refresh, role navigation, review conflicts,
read-only accounts and escaped evidence. They do not replace browser rendering,
backend authorization or live-provider integration tests.

The network graph uses a named, focusable scroll region for keyboard access.
Its isolated `tabIndex` line suppresses Sonar S6845 because scrollable content is
an [explicit accessibility-rule exception](https://github.com/jsx-eslint/eslint-plugin-jsx-a11y/blob/main/docs/rules/no-noninteractive-tabindex.md);
[MDN recommends this pattern](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/overflow#accessibility)
for browsers that do not focus scroll containers automatically. Tests verify the
region's name and keyboard focus; actual scrolling requires a browser.

This template provides a minimal setup to get React working in Vite with HMR and some Oxlint rules.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Oxc](https://oxc.rs)
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/)

## React Compiler

The React Compiler is not enabled on this template because of its impact on dev & build performances. To add it, see [this documentation](https://react.dev/learn/react-compiler/installation).

## Expanding the Oxlint configuration

If you are developing a production application, we recommend enabling type-aware lint rules by installing `oxlint-tsgolint` and editing `.oxlintrc.json`:

```json
{
  "$schema": "./node_modules/oxlint/configuration_schema.json",
  "plugins": ["react", "typescript", "oxc"],
  "options": {
    "typeAware": true
  },
  "rules": {
    "react/rules-of-hooks": "error",
    "react/only-export-components": ["warn", { "allowConstantExport": true }]
  }
}
```

See the [Oxlint rules documentation](https://oxc.rs/docs/guide/usage/linter/rules) for the full list of rules and categories.
