/**
 * Runtime configuration injected by the Go server into index.html as <meta>
 * tags.  This lets a pre-built SPA be configured at serve-time without
 * requiring a rebuild — useful in ui-only mode where the frontend and backend
 * run on different servers.
 */

const META_API_BASE_URL = "paperclip-api-base-url";

function readMetaContent(name: string): string | null {
  if (typeof document === "undefined") return null;
  const el = document.querySelector(`meta[name="${name}"]`);
  const content = el?.getAttribute("content")?.trim();
  return content ? content : null;
}

/**
 * Returns the configured API origin (e.g. "https://api.example.com") read
 * from the injected <meta name="paperclip-api-base-url"> tag, with any
 * trailing slashes stripped.  Returns an empty string when no override is
 * configured, meaning the UI and API share the same origin.
 */
export function getConfiguredApiOrigin(): string {
  const raw = readMetaContent(META_API_BASE_URL);
  if (!raw) return "";
  return raw.replace(/\/+$/, "");
}

/**
 * Returns the HTTP(S) base URL for all REST API calls.
 *
 * - Same-origin (default):       "/api"
 * - Remote backend configured:   "https://api.example.com/api"
 */
export function getApiBaseUrl(): string {
  const origin = getConfiguredApiOrigin();
  return origin ? `${origin}/api` : "/api";
}

/**
 * Returns the base URL prefix for WebSocket connections.
 *
 * - Same-origin (default):       derived from window.location  ("ws://host" / "wss://host")
 * - Remote backend configured:   "wss://api.example.com"  (or "ws://..." for http)
 *
 * Callers append the API path, e.g.:
 *   `${getApiWsBase()}/api/companies/${id}/events/ws`
 */
export function getApiWsBase(): string {
  const origin = getConfiguredApiOrigin();
  if (origin) {
    // Convert http(s):// → ws(s)://
    return origin.replace(/^https:\/\//, "wss://").replace(/^http:\/\//, "ws://");
  }
  // Fall back to the current page's host.
  if (typeof window === "undefined") return "";
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}`;
}
