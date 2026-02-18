/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * Koa application setup
 *
 * Configures the Koa app with middleware stack and routes
 */

import Koa from 'koa';
import Router from '@koa/router';
import cors from '@koa/cors';
import bodyParser from 'koa-bodyparser';
import serve from 'koa-static';
import { existsSync } from 'fs';
import { resolve } from 'path';
import { fileURLToPath } from 'url';

import type { AppConfig } from './config.js';
import {
  errorHandler,
  logger,
  security,
  initDevAuth,
  createSessionMiddleware,
  createAuthMiddleware,
} from './middleware/index.js';
import {
  healthRoutes,
  pageRoutes,
  setPageRoutesConfig,
  createApiRouter,
  createAuthRouter,
  createSseRouter,
} from './routes/index.js';
import { NatsClient, SSEManager } from './services/index.js';

const __dirname = fileURLToPath(new URL('.', import.meta.url));

/** Extended app context with NATS/SSE services */
export interface AppServices {
  natsClient: NatsClient | null;
  sseManager: SSEManager | null;
}

/**
 * Creates and configures the Koa application
 *
 * @param config - Application configuration
 * @returns Configured Koa application with services on context
 */
export function createApp(config: AppConfig): Koa & { services: AppServices } {
  const app = new Koa() as Koa & { services: AppServices };
  const router = new Router();

  // Initialize NATS/SSE services (connection happens later in index.ts)
  let natsClient: NatsClient | null = null;
  let sseManager: SSEManager | null = null;

  if (config.nats.enabled && config.nats.servers.length > 0) {
    natsClient = new NatsClient({
      servers: config.nats.servers,
      token: config.nats.token,
      maxReconnectAttempts: config.nats.maxReconnectAttempts,
    });
    sseManager = new SSEManager(natsClient);
  }

  app.services = { natsClient, sseManager };

  // Trust proxy headers (for Cloud Run)
  app.proxy = true;

  // Core middleware stack
  // Order matters: error handler should be first to catch all errors
  app.use(errorHandler());
  app.use(logger(config));
  app.use(security(config));
  app.use(
    cors({
      origin: config.cors.origin,
      credentials: config.cors.credentials,
    })
  );

  // Body parsing for JSON requests
  app.use(bodyParser());

  // Session middleware (must be before auth)
  app.use(createSessionMiddleware(app, config));

  // Dev auth middleware (auto-login for development)
  // This runs before the auth middleware so dev users bypass login
  const devAuth = initDevAuth();
  app.use(devAuth.middleware);

  // Auth middleware (enforces authentication on protected routes)
  // Skips if dev-auth already set a user
  app.use(createAuthMiddleware(config));

  // Static asset serving from public/ directory
  // In dev mode (tsx), __dirname is src/server/ (2 levels from web root)
  // In compiled mode, __dirname is dist/server/server/ (3 levels from web root)
  const publicDir = existsSync(resolve(__dirname, '../../public'))
    ? resolve(__dirname, '../../public')
    : resolve(__dirname, '../../../public');
  app.use(
    serve(publicDir, {
      maxage: config.production ? 86400000 : 0, // 24h in prod, no cache in dev
      gzip: true,
      brotli: true,
    })
  );

  // Set config for page routes (needed for auth config in login page)
  setPageRoutesConfig(config);

  // Mount health check routes
  router.use(healthRoutes.routes());
  router.use(healthRoutes.allowedMethods());

  // Mount auth routes
  const authRouter = createAuthRouter(config);
  router.use('/auth', authRouter.routes());
  router.use('/auth', authRouter.allowedMethods());

  // Mount API proxy routes
  const apiRouter = createApiRouter(config);
  router.use('/api', apiRouter.routes());
  router.use('/api', apiRouter.allowedMethods());

  // Mount SSE route (between API proxy and page routes)
  if (sseManager && natsClient) {
    const sseRouter = createSseRouter(sseManager, natsClient);
    router.use(sseRouter.routes());
    router.use(sseRouter.allowedMethods());
  }

  // Mount SSR page routes (catch-all, should be last)
  router.use(pageRoutes.routes());
  router.use(pageRoutes.allowedMethods());

  // Apply router middleware
  app.use(router.routes());
  app.use(router.allowedMethods());

  return app;
}
