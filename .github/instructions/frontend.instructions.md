---
applyTo: "webui/**/*.ts,webui/**/*.tsx"
---

# Frontend Code Instructions

## Stack

- **Framework:** Next.js with React 19 and TypeScript (strict mode).
- **Styling:** Tailwind CSS with shadcn/ui components (Radix primitives).
- **State:** TanStack Query for server state, Zustand for client state.
- **Forms:** react-hook-form with Zod validation schemas.
- **API calls:** Centralized in `lib/api-client.ts` — never call `fetch` directly from components.
- **Charts:** uPlot and Apache ECharts.

## Package Manager

Always use **npm** (not yarn or pnpm). Install with `npm ci`, not `npm install`.

## TypeScript

- Strict mode is enabled — no `any` types allowed.
- Validate all API responses with Zod schemas.
- Use specific types or generics instead of `any` or `unknown`.

## Component Patterns

- Thin page components that delegate to hooks and API client.
- Shared UI components from `components/ui/` (shadcn/ui).
- Wildcard imports are allowed only in `components/ui/**/*.tsx`.

## Validation

Always run before committing:
```bash
npm run lint        # ESLint
npm run type-check  # tsc --noEmit
npm run build       # Next.js production build
```

## Naming

- Components: PascalCase (e.g., `VMDetailPage`).
- Hooks: camelCase with `use` prefix (e.g., `useVMMetrics`).
- API functions: camelCase verb + noun (e.g., `fetchVMList`, `createBackup`).
- Constants: `UPPER_SNAKE_CASE`.
- Booleans: prefix with `is`, `has`, `can`, `should`.

## Prohibitions

- No `console.log` in production code (ESLint warns).
- No `any` types.
- No direct `fetch` calls — use the API client in `lib/api-client.ts`.
- No `TODO`/`FIXME`/`HACK` comments.
- No commented-out code.
