/**
 * Health check routes
 *
 * Provides liveness and readiness probes for Kubernetes/Cloud Run
 */

import Router from '@koa/router';

const router = new Router();

interface HealthResponse {
    status: 'healthy' | 'unhealthy';
    timestamp: string;
    uptime: number;
}

/**
 * GET /healthz - Liveness probe
 *
 * Returns healthy if the server is running and can respond to requests.
 * This is used by Kubernetes/Cloud Run to determine if the container is alive.
 */
router.get('/healthz', (ctx) => {
    const response: HealthResponse = {
        status: 'healthy',
        timestamp: new Date().toISOString(),
        uptime: process.uptime(),
    };

    ctx.status = 200;
    ctx.type = 'application/json';
    ctx.body = response;
});

/**
 * GET /readyz - Readiness probe
 *
 * Returns healthy if the server is ready to serve traffic.
 * In the future, this will check database connections, NATS connectivity, etc.
 */
router.get('/readyz', (ctx) => {
    // TODO: Add checks for:
    // - Hub API connectivity
    // - NATS connection
    // - Session store

    const response: HealthResponse = {
        status: 'healthy',
        timestamp: new Date().toISOString(),
        uptime: process.uptime(),
    };

    ctx.status = 200;
    ctx.type = 'application/json';
    ctx.body = response;
});

export const healthRoutes = router;
