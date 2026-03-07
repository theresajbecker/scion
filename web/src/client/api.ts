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
 * API fetch wrapper with automatic 403 handling.
 *
 * Wraps the standard fetch() with credential inclusion and dispatches a
 * `scion:access-denied` CustomEvent on the window when a 403 response is
 * received, allowing the app shell to display a toast notification.
 *
 * This is additive — existing components can continue using fetch() directly.
 * Phase 3 will migrate them to apiFetch().
 */

/** Detail payload for the scion:access-denied custom event. */
export interface AccessDeniedDetail {
  resource?: string;
  action?: string;
  reason?: string;
}

/**
 * Fetch wrapper that includes credentials and handles 403 responses.
 *
 * Returns the raw Response object so callers can handle the body themselves.
 * On 403, dispatches a `scion:access-denied` event on `window` with parsed
 * error details, but does NOT re-throw or alter the response.
 */
export async function apiFetch(path: string, options?: RequestInit): Promise<Response> {
  const response = await fetch(path, {
    ...options,
    credentials: 'include',
  });

  if (response.status === 403) {
    let detail: AccessDeniedDetail = {};
    try {
      const body = await response.clone().json();
      detail = {
        resource: body.resource,
        action: body.action,
        reason: body.reason || body.error || body.message,
      };
    } catch {
      // Body wasn't JSON — use empty detail
    }
    window.dispatchEvent(
      new CustomEvent('scion:access-denied', { detail })
    );
  }

  return response;
}

/**
 * Extract a human-readable error message from an API error response.
 *
 * The backend returns errors in the format: `{"error": {"code": "...", "message": "..."}}`.
 * This helper parses that structure and returns just the message string.
 */
export async function extractApiError(res: Response, fallback: string): Promise<string> {
  try {
    const data = (await res.json()) as {
      error?: { message?: string } | string;
      message?: string;
    };
    if (typeof data.error === 'object' && data.error?.message) return data.error.message;
    if (typeof data.message === 'string') return data.message;
    if (typeof data.error === 'string') return data.error;
  } catch {
    // Response wasn't JSON
  }
  return fallback;
}
