# Scion Web Frontend

Browser-based dashboard for managing Scion agents and groves.

## Prerequisites

- Node.js 20.x or later
- npm 10.x or later

## Getting Started

### Install Dependencies

```bash
npm install
```

### Development

Start the development server with hot reload:

```bash
npm run dev
```

The server will be available at `http://localhost:8080`.

### Build

Build for production:

```bash
npm run build
```

### Production

Start the production server:

```bash
npm run start
```

## Available Scripts

| Script | Description |
|--------|-------------|
| `npm run dev` | Start development server with hot reload |
| `npm run build` | Build for production |
| `npm start` | Start production server |
| `npm run lint` | Run ESLint |
| `npm run lint:fix` | Run ESLint with auto-fix |
| `npm run format` | Format code with Prettier |
| `npm run typecheck` | Run TypeScript type checking |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `HOST` | `0.0.0.0` | Server hostname |
| `NODE_ENV` | `development` | Environment (development/production) |
| `HUB_API_URL` | `http://localhost:9810` | Hub API URL |
| `CORS_ORIGIN` | `*` | CORS allowed origins |

## API Endpoints

### Health Checks

- `GET /healthz` - Liveness probe
- `GET /readyz` - Readiness probe

### Static Assets

- `GET /assets/*` - Static files from the public directory

## Project Structure

```
web/
├── src/
│   ├── server/              # Koa server
│   │   ├── index.ts         # Entry point
│   │   ├── app.ts           # Koa app setup
│   │   ├── config.ts        # Configuration
│   │   ├── middleware/      # Middleware
│   │   │   ├── error-handler.ts
│   │   │   ├── logger.ts
│   │   │   └── security.ts
│   │   └── routes/          # Route handlers
│   │       └── health.ts
│   └── client/              # Client-side code
│       └── main.ts
├── public/                   # Static assets
│   └── assets/
├── package.json
├── tsconfig.json
├── tsconfig.server.json
└── vite.config.ts
```

## Milestone Status

- [x] **M1: Koa Server Foundation** - Complete
- [ ] M2: Lit SSR Integration
- [ ] M3: Web Awesome Component Library
- [ ] M4: Authentication Flow
- [ ] M5: Hub API Proxy
- [ ] M6: Grove & Agent Pages
- [ ] M7: SSE + NATS Real-Time Updates
- [ ] M8: Terminal Component
- [ ] M9: Agent Creation Workflow
- [ ] M10: Production Hardening
- [ ] M11: Cloud Run Deployment
