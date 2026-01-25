/**
 * Koa application setup
 *
 * Configures the Koa app with middleware stack and routes
 */

import Koa from 'koa';
import Router from '@koa/router';
import cors from '@koa/cors';
import serve from 'koa-static';
import { resolve } from 'path';
import { fileURLToPath } from 'url';

import type { AppConfig } from './config.js';
import { errorHandler, logger, security } from './middleware/index.js';
import { healthRoutes } from './routes/index.js';

const __dirname = fileURLToPath(new URL('.', import.meta.url));

/**
 * Creates and configures the Koa application
 *
 * @param config - Application configuration
 * @returns Configured Koa application
 */
export function createApp(config: AppConfig): Koa {
    const app = new Koa();
    const router = new Router();

    // Trust proxy headers (for Cloud Run)
    app.proxy = true;

    // Core middleware stack
    // Order matters: error handler should be first to catch all errors
    app.use(errorHandler());
    app.use(logger());
    app.use(security(config));
    app.use(
        cors({
            origin: config.cors.origin,
            credentials: config.cors.credentials,
        })
    );

    // Static asset serving from public/ directory
    const publicDir = resolve(__dirname, '../../public');
    app.use(
        serve(publicDir, {
            maxage: config.production ? 86400000 : 0, // 24h in prod, no cache in dev
            gzip: true,
            brotli: true,
        })
    );

    // Mount health check routes
    router.use(healthRoutes.routes());
    router.use(healthRoutes.allowedMethods());

    // Apply router middleware
    app.use(router.routes());
    app.use(router.allowedMethods());

    return app;
}
