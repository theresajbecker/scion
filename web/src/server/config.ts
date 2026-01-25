/**
 * Server configuration
 *
 * Handles environment-based configuration for the Koa server
 */

export interface AppConfig {
    /** Server port */
    port: number;

    /** Server hostname */
    host: string;

    /** Whether running in production mode */
    production: boolean;

    /** Hub API URL */
    hubApiUrl: string;

    /** CORS configuration */
    cors: {
        origin: string | string[];
        credentials: boolean;
    };

    /** Security settings */
    security: {
        /** Content Security Policy */
        csp: string;
        /** HSTS max-age in seconds */
        hstsMaxAge: number;
    };
}

function getEnvString(key: string, defaultValue: string): string {
    return process.env[key] ?? defaultValue;
}

function getEnvNumber(key: string, defaultValue: number): number {
    const value = process.env[key];
    if (value === undefined) return defaultValue;
    const parsed = parseInt(value, 10);
    return isNaN(parsed) ? defaultValue : parsed;
}

function getEnvBoolean(key: string, defaultValue: boolean): boolean {
    const value = process.env[key];
    if (value === undefined) return defaultValue;
    return value.toLowerCase() === 'true' || value === '1';
}

export function loadConfig(): AppConfig {
    const production = getEnvString('NODE_ENV', 'development') === 'production';

    return {
        port: getEnvNumber('PORT', 8080),
        host: getEnvString('HOST', '0.0.0.0'),
        production,
        hubApiUrl: getEnvString('HUB_API_URL', 'http://localhost:9810'),

        cors: {
            origin: production ? getEnvString('CORS_ORIGIN', '*') : '*',
            credentials: true,
        },

        security: {
            csp: [
                "default-src 'self'",
                "script-src 'self' 'unsafe-inline' https://cdn.webawesome.com",
                "style-src 'self' 'unsafe-inline' https://cdn.webawesome.com https://fonts.googleapis.com",
                "font-src 'self' https://fonts.gstatic.com https://cdn.webawesome.com",
                "img-src 'self' data: https:",
                "connect-src 'self' ws: wss:",
            ].join('; '),
            hstsMaxAge: production ? 31536000 : 0, // 1 year in production
        },
    };
}

export const config = loadConfig();
