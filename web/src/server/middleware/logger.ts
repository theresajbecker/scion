/**
 * Logger middleware
 *
 * Logs HTTP requests with timing, status, and basic request info
 */

import type { Context, Next, Middleware } from 'koa';

export interface LogEntry {
    timestamp: string;
    requestId: string;
    method: string;
    path: string;
    status: number;
    duration: number;
    ip: string;
    userAgent: string;
}

/**
 * Generates a unique request ID
 */
function generateRequestId(): string {
    return `${Date.now().toString(36)}-${Math.random().toString(36).substring(2, 9)}`;
}

/**
 * Formats a log entry as JSON (for structured logging)
 */
function formatLogEntry(entry: LogEntry): string {
    return JSON.stringify(entry);
}

/**
 * Creates the logger middleware
 *
 * @returns Koa middleware function
 */
export function logger(): Middleware {
    return async (ctx: Context, next: Next): Promise<void> => {
        const start = Date.now();
        const requestId = generateRequestId();

        // Attach request ID to context state for downstream use
        ctx.state.requestId = requestId;

        // Set request ID header for tracing
        ctx.set('X-Request-ID', requestId);

        try {
            await next();
        } finally {
            const duration = Date.now() - start;

            const entry: LogEntry = {
                timestamp: new Date().toISOString(),
                requestId,
                method: ctx.method,
                path: ctx.path,
                status: ctx.status,
                duration,
                ip: ctx.ip,
                userAgent: ctx.get('User-Agent') || 'unknown',
            };

            // Log to stdout as JSON
            console.info(formatLogEntry(entry));
        }
    };
}
