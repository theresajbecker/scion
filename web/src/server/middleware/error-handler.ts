/**
 * Error handler middleware
 *
 * Catches all errors and returns appropriate JSON responses
 */

import type { Context, Next, Middleware } from 'koa';

export interface ErrorResponse {
    error: {
        message: string;
        code: string;
        requestId?: string;
    };
}

/**
 * HTTP error class for known error conditions
 */
export class HttpError extends Error {
    constructor(
        public status: number,
        message: string,
        public code: string = 'INTERNAL_ERROR'
    ) {
        super(message);
        this.name = 'HttpError';
    }
}

/**
 * Creates the error handler middleware
 *
 * This should be the first middleware in the stack to catch all errors
 *
 * @returns Koa middleware function
 */
export function errorHandler(): Middleware {
    return async (ctx: Context, next: Next): Promise<void> => {
        try {
            await next();

            // Handle 404 for routes that don't set a body
            if (ctx.status === 404 && !ctx.body) {
                ctx.status = 404;
                ctx.type = 'application/json';
                ctx.body = {
                    error: {
                        message: `Route ${ctx.method} ${ctx.path} not found`,
                        code: 'NOT_FOUND',
                        requestId: ctx.state.requestId as string | undefined,
                    },
                } satisfies ErrorResponse;
            }
        } catch (err) {
            // Handle known HTTP errors
            if (err instanceof HttpError) {
                ctx.status = err.status;
                ctx.type = 'application/json';
                ctx.body = {
                    error: {
                        message: err.message,
                        code: err.code,
                        requestId: ctx.state.requestId as string | undefined,
                    },
                } satisfies ErrorResponse;
                return;
            }

            // Handle unknown errors
            const error = err as Error;
            console.error('Unhandled error:', {
                message: error.message,
                stack: error.stack,
                requestId: ctx.state.requestId,
            });

            ctx.status = 500;
            ctx.type = 'application/json';
            ctx.body = {
                error: {
                    message: 'Internal server error',
                    code: 'INTERNAL_ERROR',
                    requestId: ctx.state.requestId as string | undefined,
                },
            } satisfies ErrorResponse;
        }
    };
}
