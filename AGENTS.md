# Repository Guidelines

## Project Structure & Module Organization
- Monorepo managed by Bun workspaces: `frontend/` (Vue 3 + Vite + Vuetify/Tailwind), `backend/` (Bun + Express), shared config at root (`tsconfig.json`, `.prettierrc`).
- Backend config persists to `backend/config.json`; env vars live in `.env` (root) and per-app env files (`frontend/.env.development`, `frontend/.env.production`).
- Source: `backend/src/**`, `frontend/src/**`; build outputs in each package’s `dist/`.
- TS path aliases: `@frontend/*` → `frontend/src/*`, `@backend/*` → `backend/src/*`.

Example import:
```ts
import { api } from '@frontend/services/api'
```

## Build, Test, and Development Commands
From repo root (runs through workspaces):
```bash
bun install
bun run dev          # frontend + backend (dev)
bun run build        # build all
bun run start        # start backend serving built app
bun run type-check   # strict TS checks
```
Per workspace:
```bash
cd backend  && bun run dev|build|start|type-check
cd frontend && bun run dev|build|type-check
```

## Coding Style & Naming Conventions
- Prettier enforced (.prettierrc): 2 spaces, single quotes, no semicolons, width 120, LF EOL.
- TypeScript strict mode; prefer explicit types at module boundaries.
- Naming: files (TS) kebab-case (`web-routes.ts`), Vue components PascalCase (`ChannelCard.vue`), types/interfaces PascalCase, constants UPPER_SNAKE_CASE, vars/functions camelCase.
- Keep imports using path aliases where available.

## Testing Guidelines
- No formal test suite yet. Before pushing:
  - Run: `bun run type-check` and `bun run build`.
  - Smoke test backend: GET `http://localhost:3000/health`.
  - For UI changes, include a short test plan and screenshots/GIFs in the PR.

## Commit & Pull Request Guidelines
- Use Conventional Commits (seen in history): `feat:`, `fix:`, `refactor:`, `chore:` …
  - Examples: `feat(frontend): add ESC to close modal`, `fix(backend): redact Authorization header`.
- PRs must include: purpose, linked issues, testing steps, config/env changes, and screenshots for UI.

## Security & Configuration Tips
- Never commit secrets. Use `.env` and `backend/config.json`; see `ENVIRONMENT_CONFIG.md`.
- Required: `PROXY_ACCESS_KEY` for proxy access; avoid logging full API keys.

## Agent-Specific Notes
- Keep diffs minimal, match existing style, and update docs when behavior changes. Avoid renames/refactors unless necessary.
