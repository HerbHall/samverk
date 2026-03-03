---
applyTo: "web/**/*.{ts,tsx}"
---

# React/TypeScript Instructions

## Component Patterns

- Functional components only, no class components
- Props interfaces defined above the component
- Destructure props in the function signature
- Use `React.FC` sparingly -- prefer explicit return types

## State Management

- Server state: TanStack Query (`useQuery`, `useMutation`)
- Client state: Zustand stores (minimal, focused stores)
- Form state: local `useState` or React Hook Form for complex forms
- No `useEffect` for syncing server data to state -- use nullable local override pattern

## API Integration

- All API calls through typed fetch wrappers in `web/src/api/`
- Use TanStack Query for data fetching with 30-second refetch intervals
- Type API responses explicitly -- no `any`
- Handle loading, error, and empty states in every data component

## UI Components

- shadcn/ui components from `web/src/components/ui/`
- Tailwind CSS for styling, no CSS modules or styled-components
- MUI v5 is NOT used -- this is shadcn/ui + Tailwind
- Responsive layouts with Tailwind breakpoint utilities

## TypeScript

- Strict mode enabled -- no `any`, no implicit `any`
- Use `unknown` with type guards instead of `any`
- Union return types require type guards at ALL call sites
- JSX short-circuit: use `value != null &&` not bare `value &&` for `unknown` types

## Testing

- Vitest + Testing Library
- Test files alongside source: `component.test.tsx`
- Mock API calls with `vi.fn()` and TanStack Query's `QueryClientProvider`
- Assert structure and behavior, not specific text content that may change
