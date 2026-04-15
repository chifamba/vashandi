// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { getApiBaseUrl, getApiWsBase, getConfiguredApiOrigin } from "./runtime-config";

function setApiBaseUrlMeta(value: string | null) {
  document.querySelectorAll('meta[name="paperclip-api-base-url"]').forEach((el) => el.remove());
  if (value !== null) {
    const meta = document.createElement("meta");
    meta.setAttribute("name", "paperclip-api-base-url");
    meta.setAttribute("content", value);
    document.head.appendChild(meta);
  }
}

beforeEach(() => {
  setApiBaseUrlMeta(null);
});

afterEach(() => {
  setApiBaseUrlMeta(null);
});

describe("getConfiguredApiOrigin", () => {
  it("returns empty string when no meta tag is present", () => {
    expect(getConfiguredApiOrigin()).toBe("");
  });

  it("returns the configured origin", () => {
    setApiBaseUrlMeta("https://api.example.com");
    expect(getConfiguredApiOrigin()).toBe("https://api.example.com");
  });

  it("strips trailing slashes", () => {
    setApiBaseUrlMeta("https://api.example.com/");
    expect(getConfiguredApiOrigin()).toBe("https://api.example.com");
  });

  it("strips multiple trailing slashes", () => {
    setApiBaseUrlMeta("https://api.example.com///");
    expect(getConfiguredApiOrigin()).toBe("https://api.example.com");
  });

  it("returns empty string for blank content", () => {
    setApiBaseUrlMeta("   ");
    expect(getConfiguredApiOrigin()).toBe("");
  });
});

describe("getApiBaseUrl", () => {
  it("returns /api when no meta tag is present", () => {
    expect(getApiBaseUrl()).toBe("/api");
  });

  it("returns remote base URL when meta tag is present", () => {
    setApiBaseUrlMeta("https://api.example.com");
    expect(getApiBaseUrl()).toBe("https://api.example.com/api");
  });

  it("does not double-add /api when the origin has no trailing slash", () => {
    setApiBaseUrlMeta("https://api.example.com");
    expect(getApiBaseUrl()).toBe("https://api.example.com/api");
  });
});

describe("getApiWsBase", () => {
  it("converts https origin to wss", () => {
    setApiBaseUrlMeta("https://api.example.com");
    expect(getApiWsBase()).toBe("wss://api.example.com");
  });

  it("converts http origin to ws", () => {
    setApiBaseUrlMeta("http://api.example.com");
    expect(getApiWsBase()).toBe("ws://api.example.com");
  });

  it("falls back to window.location when no meta tag is present", () => {
    // jsdom defaults to http://localhost
    const result = getApiWsBase();
    expect(result).toMatch(/^ws:\/\//);
    expect(result).toContain("localhost");
  });
});
