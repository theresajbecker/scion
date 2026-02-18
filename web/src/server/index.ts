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
 * Server entry point
 *
 * Starts the Koa server and handles graceful shutdown
 */

import { createApp } from './app.js';
import { config } from './config.js';
import { setupWebSocketProxy } from './routes/index.js';

// Create the Koa application
const app = createApp(config);

// Connect to NATS if enabled (async, non-blocking for server startup)
const { natsClient, sseManager } = app.services;

if (natsClient) {
  natsClient.connect().then(() => {
    console.info('[NATS] Ready for SSE subscriptions');
  }).catch((err) => {
    console.warn(`[NATS] Failed to connect (server will continue without real-time events): ${err}`);
  });
}

// Start the server
const natsStatus = natsClient
  ? `enabled (${config.nats.servers.join(', ')})`
  : 'disabled';

const server = app.listen(config.port, config.host, () => {
  const debugStatus = config.debug ? 'enabled' : 'disabled';
  console.info(`
╔════════════════════════════════════════════════════════════╗
║                   Scion Web Frontend                        ║
╠════════════════════════════════════════════════════════════╣
║  Server running at http://${config.host}:${config.port.toString().padEnd(4)}                       ║
║  Environment: ${config.production ? 'production' : 'development'}                                ║
║  Debug mode: ${debugStatus.padEnd(8)}                                       ║
║  Hub API: ${config.hubApiUrl.substring(0, 40).padEnd(40)}     ║
║  NATS: ${natsStatus.substring(0, 43).padEnd(43)}     ║
╚════════════════════════════════════════════════════════════╝
  `);

  if (config.debug) {
    console.info('[DEBUG] Debug logging is enabled. Set SCION_API_DEBUG=false to disable.');
  }
});

// Set up WebSocket proxy for PTY connections (operates at HTTP server level)
setupWebSocketProxy(server, config);

// Graceful shutdown handling
async function shutdown(signal: string): Promise<void> {
  console.info(`\n${signal} received. Shutting down gracefully...`);

  // Close SSE connections first
  if (sseManager) {
    console.info('[SSE] Closing all connections...');
    sseManager.closeAll();
  }

  // Drain NATS before closing server
  if (natsClient) {
    try {
      await natsClient.close();
    } catch (err) {
      console.error('[NATS] Error during shutdown:', err);
    }
  }

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

process.on('SIGTERM', () => { void shutdown('SIGTERM'); });
process.on('SIGINT', () => { void shutdown('SIGINT'); });

// Handle uncaught exceptions
process.on('uncaughtException', (err) => {
  console.error('Uncaught exception:', err);
  void shutdown('UNCAUGHT_EXCEPTION');
});

// Handle unhandled promise rejections
process.on('unhandledRejection', (reason, promise) => {
  console.error('Unhandled rejection at:', promise, 'reason:', reason);
});
