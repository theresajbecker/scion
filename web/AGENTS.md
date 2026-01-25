# Scion Web Frontend - Agent Instructions

This document provides instructions for AI agents working on the Scion Web Frontend.

## Design Documents

Before making changes, review the relevant design documentation:

- **[Web Frontend Design](../.design/hosted/web-frontend-design.md)** - Architecture, technology stack, component patterns
- **[Frontend Milestones](../.design/hosted/frontend-milestones.md)** - Implementation phases and test criteria

## Development Workflow

### Starting the Development Server

```bash
cd web
npm install    # First time only, or after package.json changes
npm run dev    # Starts server with hot reload on port 8080
```

### Common Commands

| Command | Purpose |
|---------|---------|
| `npm run dev` | Start development server (port 8080) |
| `npm run build` | Build for production |
| `npm start` | Run production build |
| `npm run lint` | Check for linting errors |
| `npm run lint:fix` | Auto-fix linting errors |
| `npm run format` | Format code with Prettier |
| `npm run typecheck` | Run TypeScript type checking |

### Verifying Changes

After making changes, verify:

1. **Server starts:** `npm run dev` should start without errors
2. **Type checking passes:** `npm run typecheck` 
3. **Linting passes:** `npm run lint`
4. **Health endpoint works:** `curl localhost:8080/healthz`

## Project Structure

```
web/
├── src/
│   ├── server/          # Koa server code
│   │   ├── index.ts     # Entry point
│   │   ├── app.ts       # Koa app configuration
│   │   ├── config.ts    # Environment config
│   │   ├── middleware/  # Koa middleware
│   │   └── routes/      # Route handlers
│   ├── client/          # Browser-side code (future)
│   ├── components/      # Lit components (future)
│   └── styles/          # CSS styles (future)
├── public/              # Static assets
└── dist/                # Build output (gitignored)
```

## Technology Stack

- **Server:** Koa 2.x with TypeScript
- **Components:** Lit 3.x (future milestones)
- **UI Library:** Web Awesome / Shoelace (future milestones)
- **Build:** Vite
- **SSR:** @lit-labs/ssr (future milestones)

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `HOST` | `0.0.0.0` | Server hostname |
| `NODE_ENV` | `development` | Environment mode |
| `HUB_API_URL` | `http://localhost:9810` | Hub API endpoint |

## Milestone Progress

Track implementation progress in the [frontend milestones](../.design/hosted/frontend-milestones.md) document.

Current status:
- ✅ **M1: Koa Server Foundation** - Complete
- ⬜ M2: Lit SSR Integration
- ⬜ M3: Web Awesome Component Library
- ⬜ M4: Authentication Flow
- ⬜ M5: Hub API Proxy
- ⬜ M6: Grove & Agent Pages
- ⬜ M7: SSE + NATS Real-Time Updates
- ⬜ M8: Terminal Component
- ⬜ M9: Agent Creation Workflow
- ⬜ M10: Production Hardening
- ⬜ M11: Cloud Run Deployment

## Key Patterns

### Adding a New Route

1. Create route file in `src/server/routes/`
2. Export from `src/server/routes/index.ts`
3. Mount in `src/server/app.ts`

### Adding Middleware

1. Create middleware in `src/server/middleware/`
2. Export from `src/server/middleware/index.ts`
3. Add to middleware stack in `src/server/app.ts` (order matters!)

### Error Handling

Use `HttpError` from `middleware/error-handler.ts` for known errors:

```typescript
import { HttpError } from '../middleware/error-handler.js';

throw new HttpError(404, 'Resource not found', 'NOT_FOUND');
```
