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
 * SSE connection manager
 *
 * Bridges NATS messages to SSE streams. Each SSE connection holds a set
 * of NATS subscriptions (immutable after creation). Handles heartbeats,
 * sequential event IDs, and cleanup on disconnect.
 */

import { PassThrough } from 'stream';
import type { Subscription } from 'nats';
import { StringCodec } from 'nats';

import type { NatsClient } from './nats-client.js';

const sc = StringCodec();

const HEARTBEAT_INTERVAL_MS = 30_000;

export interface SSEConnection {
  id: string;
  stream: PassThrough;
  userId: string;
  subjects: string[];
  subscriptions: Subscription[];
  lastEventId: number;
  heartbeatTimer: ReturnType<typeof setInterval>;
}

export interface SSEManagerStats {
  activeConnections: number;
}

export class SSEManager {
  private connections = new Map<string, SSEConnection>();
  private natsClient: NatsClient;
  private nextConnId = 1;

  constructor(natsClient: NatsClient) {
    this.natsClient = natsClient;
  }

  /**
   * Create a new SSE connection with NATS subscriptions for the given subjects.
   *
   * @param userId - ID of the authenticated user
   * @param subjects - NATS subjects to subscribe to (immutable for the connection lifetime)
   * @param resumeFrom - Optional Last-Event-ID for the starting event counter
   * @returns The SSEConnection with a PassThrough stream to pipe to the HTTP response
   */
  createConnection(userId: string, subjects: string[], resumeFrom?: number): SSEConnection {
    const id = `sse-${this.nextConnId++}`;
    const stream = new PassThrough();

    const conn: SSEConnection = {
      id,
      stream,
      userId,
      subjects,
      subscriptions: [],
      lastEventId: resumeFrom ?? 0,
      heartbeatTimer: setInterval(() => {
        this.sendHeartbeat(conn);
      }, HEARTBEAT_INTERVAL_MS),
    };

    // Subscribe to each NATS subject and bridge messages to SSE
    for (const subject of subjects) {
      try {
        const sub = this.natsClient.subscribe(subject);
        conn.subscriptions.push(sub);
        this.bridgeSubscription(conn, sub, subject);
      } catch (err) {
        console.error(`[SSE] Failed to subscribe to ${subject} for connection ${id}:`, err);
        // Clean up any subscriptions already created
        this.cleanupSubscriptions(conn);
        clearInterval(conn.heartbeatTimer);
        stream.destroy();
        throw err;
      }
    }

    this.connections.set(id, conn);

    // Send initial connected event
    this.sendEvent(conn, 'connected', { connectionId: id, subjects });

    return conn;
  }

  /**
   * Remove a connection: unsubscribe from NATS, stop heartbeat, end stream.
   */
  removeConnection(connId: string): void {
    const conn = this.connections.get(connId);
    if (!conn) {
      return;
    }

    this.connections.delete(connId);
    clearInterval(conn.heartbeatTimer);
    this.cleanupSubscriptions(conn);

    // End the stream if still writable
    if (!conn.stream.destroyed) {
      conn.stream.end();
    }
  }

  /**
   * Get stats about active connections.
   */
  getStats(): SSEManagerStats {
    return {
      activeConnections: this.connections.size,
    };
  }

  /**
   * Close all connections. Called during server shutdown.
   */
  closeAll(): void {
    for (const [connId] of this.connections) {
      this.removeConnection(connId);
    }
  }

  /**
   * Send a formatted SSE event to a connection.
   */
  private sendEvent(conn: SSEConnection, event: string, data: unknown): void {
    if (conn.stream.destroyed) {
      return;
    }

    conn.lastEventId++;
    const payload = typeof data === 'string' ? data : JSON.stringify(data);
    const message = `id: ${conn.lastEventId}\nevent: ${event}\ndata: ${payload}\n\n`;

    conn.stream.write(message);
  }

  /**
   * Send a heartbeat comment to keep the connection alive.
   */
  private sendHeartbeat(conn: SSEConnection): void {
    if (conn.stream.destroyed) {
      this.removeConnection(conn.id);
      return;
    }

    // SSE comments (lines starting with ':') are ignored by EventSource
    // but keep the TCP connection alive
    conn.stream.write(`:heartbeat ${Date.now()}\n\n`);
  }

  /**
   * Bridge a NATS subscription to SSE events on a connection.
   */
  private bridgeSubscription(conn: SSEConnection, sub: Subscription, subject: string): void {
    (async () => {
      for await (const msg of sub) {
        if (conn.stream.destroyed) {
          break;
        }

        try {
          const raw = sc.decode(msg.data);
          // Try to parse as JSON; if it fails, send as raw string
          let parsed: unknown;
          try {
            parsed = JSON.parse(raw);
          } catch {
            parsed = raw;
          }

          this.sendEvent(conn, 'update', {
            subject: msg.subject,
            data: parsed,
          });
        } catch (err) {
          console.error(`[SSE] Error processing message on ${subject} for ${conn.id}:`, err);
        }
      }
    })().catch((err) => {
      // Subscription iterator ends on unsubscribe or connection close
      if (!conn.stream.destroyed) {
        console.error(`[SSE] Subscription iterator error for ${conn.id} on ${subject}:`, err);
      }
    });
  }

  /**
   * Drain/unsubscribe all NATS subscriptions on a connection.
   */
  private cleanupSubscriptions(conn: SSEConnection): void {
    for (const sub of conn.subscriptions) {
      try {
        sub.unsubscribe();
      } catch {
        // Ignore errors during cleanup (connection may already be closed)
      }
    }
    conn.subscriptions = [];
  }
}
