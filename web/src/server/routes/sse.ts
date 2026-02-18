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
 * SSE endpoint
 *
 * GET /events?sub=grove.abc.>&sub=agent.xyz.>
 *
 * Opens an SSE stream that bridges NATS messages for the requested
 * subjects to the browser. Subjects are declared at connection time
 * via query parameters and are immutable for the connection lifetime.
 */

import Router from '@koa/router';
import type { Context } from 'koa';

import type { SSEManager, SSEConnection } from '../services/sse-manager.js';
import type { NatsClient } from '../services/nats-client.js';
import type { AuthState } from '../middleware/auth.js';

/** Allowed subject prefixes */
const ALLOWED_PREFIXES = ['grove.', 'agent.', 'broker.'];

/**
 * Validate that a subject is safe to subscribe to.
 * Returns null if valid, or an error message string if invalid.
 */
function validateSubject(subject: string): string | null {
  if (!subject || subject.trim().length === 0) {
    return 'Empty subject';
  }

  // Reject bare wildcards
  if (subject === '>' || subject === '*') {
    return 'Bare wildcards are not allowed';
  }

  // Must start with an allowed prefix
  const hasValidPrefix = ALLOWED_PREFIXES.some((prefix) => subject.startsWith(prefix));
  if (!hasValidPrefix) {
    return `Subject must start with one of: ${ALLOWED_PREFIXES.join(', ')}`;
  }

  // Must have at least two tokens (prefix.something)
  const tokens = subject.split('.');
  if (tokens.length < 2 || (tokens.length === 2 && tokens[1] === '')) {
    return 'Subject must have at least two tokens (e.g., grove.mygrove)';
  }

  return null;
}

/**
 * Check if a user can subscribe to a given subject.
 * Initial implementation is permissive -- the Hub API handles real authz.
 */
function canSubscribe(_userId: string, subject: string): boolean {
  // Validate subject format
  const error = validateSubject(subject);
  if (error) {
    return false;
  }

  // For now, all authenticated users can subscribe to any valid subject.
  // The Hub NATS publisher will handle fine-grained access control.
  return true;
}

/**
 * Parse the `sub` query parameter(s) from the request.
 * Supports both `?sub=a&sub=b` and `?sub=a,b` formats.
 */
function parseSubjects(ctx: Context): string[] {
  const raw = ctx.query['sub'];
  if (!raw) {
    return [];
  }

  const subjects: string[] = [];
  const values = Array.isArray(raw) ? raw : [raw];

  for (const v of values) {
    // Support comma-separated subjects within a single param
    const parts = v.split(',').map((s) => s.trim()).filter((s) => s.length > 0);
    subjects.push(...parts);
  }

  // Deduplicate
  return [...new Set(subjects)];
}

/**
 * Creates the SSE router.
 *
 * @param sseManager - SSE connection manager
 * @param natsClient - NATS client (used to check connectivity)
 */
export function createSseRouter(sseManager: SSEManager, natsClient: NatsClient): Router {
  const router = new Router();

  router.get('/events', (ctx: Context) => {
    // Check NATS connectivity
    if (!natsClient.isConnected) {
      ctx.status = 503;
      ctx.body = {
        error: 'Service Unavailable',
        message: 'Real-time event service is not available',
      };
      return;
    }

    // Parse subjects from query
    const subjects = parseSubjects(ctx);
    if (subjects.length === 0) {
      ctx.status = 400;
      ctx.body = {
        error: 'Bad Request',
        message: 'At least one subject is required. Use ?sub=grove.mygrove.>',
      };
      return;
    }

    // Validate and check permissions
    const state = ctx.state as AuthState;
    const userId = state.user?.id ?? 'anonymous';
    const forbidden: string[] = [];
    const invalid: string[] = [];

    for (const subject of subjects) {
      const validationError = validateSubject(subject);
      if (validationError) {
        invalid.push(`${subject}: ${validationError}`);
        continue;
      }
      if (!canSubscribe(userId, subject)) {
        forbidden.push(subject);
      }
    }

    if (invalid.length > 0) {
      ctx.status = 400;
      ctx.body = {
        error: 'Bad Request',
        message: 'Invalid subjects',
        details: invalid,
      };
      return;
    }

    if (forbidden.length > 0) {
      ctx.status = 403;
      ctx.body = {
        error: 'Forbidden',
        message: 'Not authorized to subscribe to subjects',
        details: forbidden,
      };
      return;
    }

    // Parse Last-Event-ID for resume
    const lastEventIdHeader = ctx.get('Last-Event-ID');
    const resumeFrom = lastEventIdHeader ? parseInt(lastEventIdHeader, 10) : undefined;

    // Create SSE connection
    let conn: SSEConnection;
    try {
      conn = sseManager.createConnection(
        userId,
        subjects,
        resumeFrom && !isNaN(resumeFrom) ? resumeFrom : undefined
      );
    } catch (err) {
      console.error('[SSE] Failed to create connection:', err);
      ctx.status = 500;
      ctx.body = {
        error: 'Internal Server Error',
        message: 'Failed to establish event stream',
      };
      return;
    }

    // Set SSE response headers
    ctx.status = 200;
    ctx.type = 'text/event-stream';
    ctx.set('Cache-Control', 'no-cache');
    ctx.set('Connection', 'keep-alive');
    ctx.set('X-Accel-Buffering', 'no');
    ctx.flushHeaders();

    // Pipe the SSE stream to the response
    ctx.body = conn.stream;

    // Clean up when client disconnects
    const onClose = () => {
      sseManager.removeConnection(conn.id);
    };

    ctx.req.on('close', onClose);

    // Also clean up if the stream errors
    conn.stream.on('error', () => {
      ctx.req.removeListener('close', onClose);
      sseManager.removeConnection(conn.id);
    });
  });

  return router;
}
