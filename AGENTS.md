# Repository Guidelines

## Project Structure & Module Organization
- Monorepo managed by Bun workspaces: `frontend/` (Vue 3 + Vite + Vuetify/Tailwind), `backend/` (Bun + Express). Shared config at root (`tsconfig.json`, `.prettierrc`).
- Source lives in `backend/src/**` and `frontend/src/**`; build outputs in each package’s `dist/`.
- Backend runtime config persists to `backend/config.json`. Env vars: root `.env` plus per-app files (`frontend/.env.development`, `frontend/.env.production`).
 - Backend runtime config persists to `backend/config.json`. Before each write, a timestamped snapshot is saved under `config.backups/` (keeps last 10, auto-rotated). Env vars: root `.env` plus per-app files (`frontend/.env.development`, `frontend/.env.production`).
- TypeScript path aliases: `@frontend/* → frontend/src/*`, `@backend/* → backend/src/*`.

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
- Prettier enforced: 2 spaces, single quotes, no semicolons, width 120, LF EOL.
- TypeScript in strict mode; prefer explicit types at module boundaries.
- Naming: TS files kebab-case (e.g., `web-routes.ts`); Vue components PascalCase (e.g., `ChannelCard.vue`); types/interfaces PascalCase; constants UPPER_SNAKE_CASE; vars/functions camelCase.
- Use path aliases (`@frontend/*`, `@backend/*`) instead of relative deep paths.

## Testing Guidelines
- No formal suite yet. Before pushing: `bun run type-check` and `bun run build`.
- Smoke test backend: `GET http://localhost:3000/health`.
- For UI changes, include a short test plan and screenshots/GIFs in the PR.

## Commit & Pull Request Guidelines
- Conventional Commits (seen in history): `feat:`, `fix:`, `refactor:`, `chore:`.
  - Examples: `feat(frontend): add ESC to close modal`, `fix(backend): redact Authorization header`.
- PRs must include: purpose, linked issues, testing steps, config/env changes, and screenshots for UI changes.

## Security & Configuration Tips
- Never commit secrets. Use `.env` and `backend/config.json`; see `ENVIRONMENT_CONFIG.md`.
- Required: `PROXY_ACCESS_KEY` for proxy access. Avoid logging full API keys.

## Agent-Specific Notes
- Keep diffs minimal, match existing style, and update docs when behavior changes. Avoid renames/refactors unless necessary.
