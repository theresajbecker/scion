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
 * NATS connection manager
 *
 * Wraps nats.js with connection lifecycle management, status tracking,
 * and graceful shutdown. Owns the connection only -- callers manage
 * their own Subscription handles.
 */

import {
  connect as natsConnect,
  type NatsConnection,
  type Subscription,
  type ConnectionOptions,
} from 'nats';

export type NatsConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'reconnecting' | 'closed';

export interface NatsClientConfig {
  /** NATS server URLs */
  servers: string[];
  /** Optional auth token */
  token?: string | undefined;
  /** Max reconnect attempts (-1 for infinite) */
  maxReconnectAttempts: number;
}

export class NatsClient {
  private connection: NatsConnection | null = null;
  private _status: NatsConnectionStatus = 'disconnected';
  private config: NatsClientConfig;

  constructor(config: NatsClientConfig) {
    this.config = config;
  }

  /** Current connection status */
  get status(): NatsConnectionStatus {
    return this._status;
  }

  /** Whether the client is currently connected and ready */
  get isConnected(): boolean {
    return this._status === 'connected';
  }

  /**
   * Connect to the NATS server(s).
   * Resolves when connected; rejects on initial connection failure.
   */
  async connect(): Promise<void> {
    if (this.connection) {
      return;
    }

    this._status = 'connecting';

    const opts: ConnectionOptions = {
      servers: this.config.servers,
      maxReconnectAttempts: this.config.maxReconnectAttempts,
      reconnectTimeWait: 2000,
      // nats.js defaults to a reasonable reconnect jitter
    };

    if (this.config.token) {
      opts.token = this.config.token;
    }

    try {
      this.connection = await natsConnect(opts);
      this._status = 'connected';
      console.info(`[NATS] Connected to ${this.config.servers.join(', ')}`);

      // Monitor connection lifecycle events
      this.monitorConnection(this.connection);
    } catch (err) {
      this._status = 'disconnected';
      throw err;
    }
  }

  /**
   * Subscribe to a NATS subject. Returns a Subscription handle
   * that the caller must manage (drain/unsubscribe).
   */
  subscribe(subject: string): Subscription {
    if (!this.connection) {
      throw new Error('NATS client is not connected');
    }
    return this.connection.subscribe(subject);
  }

  /**
   * Gracefully drain the connection (finishes in-flight messages,
   * then closes). Safe to call if not connected.
   */
  async close(): Promise<void> {
    if (!this.connection) {
      return;
    }

    try {
      console.info('[NATS] Draining connection...');
      await this.connection.drain();
      console.info('[NATS] Connection drained and closed');
    } catch (err) {
      console.error('[NATS] Error during drain:', err);
      // Force close if drain fails
      try {
        await this.connection.close();
      } catch {
        // Ignore close errors after drain failure
      }
    } finally {
      this.connection = null;
      this._status = 'closed';
    }
  }

  /**
   * Monitor the connection for lifecycle events (reconnect, disconnect, close).
   */
  private monitorConnection(nc: NatsConnection): void {
    // The status monitor is an async iterable
    (async () => {
      for await (const s of nc.status()) {
        switch (s.type) {
          case 'disconnect':
            this._status = 'reconnecting';
            console.warn(`[NATS] Disconnected: ${s.data}`);
            break;
          case 'reconnect':
            this._status = 'connected';
            console.info(`[NATS] Reconnected to ${s.data}`);
            break;
          case 'reconnecting':
            this._status = 'reconnecting';
            console.info('[NATS] Reconnecting...');
            break;
          case 'error':
            console.error('[NATS] Error:', s.data);
            break;
          case 'update':
            console.info('[NATS] Server update:', s.data);
            break;
        }
      }
    })().catch((err) => {
      // Status iterator ends when connection closes
      if (this._status !== 'closed') {
        console.error('[NATS] Status monitor error:', err);
      }
    });

    // Handle the closed promise
    nc.closed().then((err) => {
      if (err) {
        console.error('[NATS] Connection closed with error:', err);
      }
      if (this._status !== 'closed') {
        this._status = 'closed';
        this.connection = null;
      }
    });
  }
}
