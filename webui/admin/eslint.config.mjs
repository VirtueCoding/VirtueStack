import nextPlugin from "eslint-config-next";

/** @type {import('eslint').Linter.Config[]} */
export default [
  ...nextPlugin,
  {
    rules: {
      // Disallow console.log in production
      "no-console": "warn",
    },
  },
];