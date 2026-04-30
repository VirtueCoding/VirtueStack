import nextPlugin from "eslint-config-next";

/** @type {import('eslint').Linter.Config[]} */
const config = [
  ...nextPlugin,
  {
    rules: {
      // Disallow console.log in production while allowing intentional warn/error reporting.
      "no-console": ["warn", { allow: ["warn", "error"] }],
    },
  },
  {
    // Allow wildcard imports in shadcn/ui components (React 17+ JSX transform convention)
    files: ["components/ui/**/*.tsx"],
    rules: {
      "no-restricted-imports": "off",
    },
  },
];

export default config;
