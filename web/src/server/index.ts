/**
 * Server entry point
 *
 * Starts the Koa server and handles graceful shutdown
 */

import { createApp } from './app.js';
import { config } from './config.js';

// Create the Koa application
const app = createApp(config);

// Start the server
const server = app.listen(config.port, config.host, () => {
    console.info(`
╔════════════════════════════════════════════════════════════╗
║                   Scion Web Frontend                        ║
╠════════════════════════════════════════════════════════════╣
║  Server running at http://${config.host}:${config.port.toString().padEnd(4)}                       ║
║  Environment: ${config.production ? 'production' : 'development'}                                ║
║  Hub API: ${config.hubApiUrl.substring(0, 40).padEnd(40)}     ║
╚════════════════════════════════════════════════════════════╝
  `);
});

// Graceful shutdown handling
function shutdown(signal: string): void {
    console.info(`\n${signal} received. Shutting down gracefully...`);

    server.close((err) => {
        if (err) {
            console.error('Error during shutdown:', err);
            process.exit(1);
        }

        console.info('Server closed successfully');
        process.exit(0);
    });

    // Force shutdown after 10 seconds
    setTimeout(() => {
        console.error('Forced shutdown after timeout');
        process.exit(1);
    }, 10000);
}

process.on('SIGTERM', () => shutdown('SIGTERM'));
process.on('SIGINT', () => shutdown('SIGINT'));

// Handle uncaught exceptions
process.on('uncaughtException', (err) => {
    console.error('Uncaught exception:', err);
    shutdown('UNCAUGHT_EXCEPTION');
});

// Handle unhandled promise rejections
process.on('unhandledRejection', (reason, promise) => {
    console.error('Unhandled rejection at:', promise, 'reason:', reason);
});
