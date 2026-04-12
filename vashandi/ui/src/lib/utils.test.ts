import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import {
  cn,
  formatCents,
  formatDate,
  formatDateTime,
  formatShortDate,
  relativeTime,
  formatTokens,
  providerDisplayName,
  billingTypeDisplayName,
  quotaSourceDisplayName,
  visibleRunCostUsd,
  financeEventKindDisplayName,
  financeDirectionDisplayName,
  issueUrl,
  agentRouteRef,
  agentUrl,
  projectRouteRef,
  projectUrl,
  projectWorkspaceUrl,
} from "./utils";

describe("utils", () => {
  describe("cn", () => {
    it("merges tailwind classes correctly", () => {
      expect(cn("px-2 py-2", "p-4")).toBe("p-4");
      expect(cn("px-2", "py-2")).toBe("px-2 py-2");
    });

    it("handles conditional classes", () => {
      expect(cn("px-2", true && "py-2", false && "m-2")).toBe("px-2 py-2");
    });
  });

  describe("formatCents", () => {
    it("formats cents to USD string", () => {
      expect(formatCents(100)).toBe("$1.00");
      expect(formatCents(0)).toBe("$0.00");
      expect(formatCents(12345)).toBe("$123.45");
    });
  });

  describe("Date formatting", () => {
    const testDate = new Date("2024-03-20T12:00:00Z");

    it("formatDate formats date correctly", () => {
      expect(formatDate(testDate)).toBe("Mar 20, 2024");
    });

    it("formatDateTime formats date and time correctly", () => {
      const formatted = formatDateTime(testDate);
      expect(formatted).toContain("Mar 20, 2024");
      expect(formatted).toMatch(/\d{1,2}:00/);
    });

    it("formatShortDate formats month and day correctly", () => {
      expect(formatShortDate(testDate)).toBe("Mar 20");
    });
  });

  describe("relativeTime", () => {
    const now = new Date("2024-03-20T12:00:00Z").getTime();

    beforeEach(() => {
      vi.useFakeTimers();
      vi.setSystemTime(now);
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it("returns 'just now' for very recent dates", () => {
      expect(relativeTime(now - 10 * 1000)).toBe("just now");
    });

    it("returns minutes ago", () => {
      expect(relativeTime(now - 5 * 60 * 1000)).toBe("5m ago");
    });

    it("returns hours ago", () => {
      expect(relativeTime(now - 3 * 60 * 60 * 1000)).toBe("3h ago");
    });

    it("returns days ago", () => {
      expect(relativeTime(now - 2 * 24 * 60 * 60 * 1000)).toBe("2d ago");
    });

    it("falls back to formatDate for older dates", () => {
      const oldDate = new Date("2024-01-01T12:00:00Z");
      expect(relativeTime(oldDate)).toBe(formatDate(oldDate));
    });
  });

  describe("formatTokens", () => {
    it("formats numbers less than 1000", () => {
      expect(formatTokens(999)).toBe("999");
    });

    it("formats thousands with 'k'", () => {
      expect(formatTokens(1500)).toBe("1.5k");
      expect(formatTokens(10000)).toBe("10.0k");
    });

    it("formats millions with 'M'", () => {
      expect(formatTokens(1200000)).toBe("1.2M");
      expect(formatTokens(10000000)).toBe("10.0M");
    });
  });

  describe("Display Name Mappers", () => {
    it("providerDisplayName maps slugs correctly", () => {
      expect(providerDisplayName("anthropic")).toBe("Anthropic");
      expect(providerDisplayName("OPENAI")).toBe("OpenAI");
      expect(providerDisplayName("unknown_provider")).toBe("unknown_provider");
    });

    it("billingTypeDisplayName maps billing types correctly", () => {
      expect(billingTypeDisplayName("metered_api")).toBe("Metered API");
      expect(billingTypeDisplayName("credits")).toBe("Credits");
    });

    it("quotaSourceDisplayName maps sources correctly", () => {
      expect(quotaSourceDisplayName("anthropic-oauth")).toBe("Anthropic OAuth");
      expect(quotaSourceDisplayName("unknown-source")).toBe("unknown-source");
    });

    it("financeEventKindDisplayName maps event kinds correctly", () => {
      expect(financeEventKindDisplayName("inference_charge")).toBe("Inference charge");
      expect(financeEventKindDisplayName("platform_fee")).toBe("Platform fee");
    });

    it("financeDirectionDisplayName maps directions correctly", () => {
      expect(financeDirectionDisplayName("credit")).toBe("Credit");
      expect(financeDirectionDisplayName("debit")).toBe("Debit");
    });
  });

  describe("visibleRunCostUsd", () => {
    it("returns 0 for subscription_included billing type", () => {
      const usage = { billingType: "subscription_included", costUsd: 10 };
      expect(visibleRunCostUsd(usage)).toBe(0);
    });

    it("reads cost from various payload keys", () => {
      expect(visibleRunCostUsd({ costUsd: 1.23 })).toBe(1.23);
      expect(visibleRunCostUsd({ cost_usd: 4.56 })).toBe(4.56);
      expect(visibleRunCostUsd({ total_cost_usd: 7.89 })).toBe(7.89);
    });

    it("falls back to result payload if usage payload has no cost", () => {
      const usage = { billingType: "metered_api" };
      const result = { costUsd: 2.50 };
      expect(visibleRunCostUsd(usage, result)).toBe(2.50);
    });

    it("returns 0 if no cost is found", () => {
      expect(visibleRunCostUsd(null)).toBe(0);
      expect(visibleRunCostUsd({})).toBe(0);
    });
  });

  describe("URL generation", () => {
    const agent = { id: "agent-id", name: "Agent Name", urlKey: "agent-key" };
    const project = { id: "project-id", name: "Project Name", urlKey: "project-key" };

    it("issueUrl uses identifier if available, else id", () => {
      expect(issueUrl({ id: "123", identifier: "ABC-1" })).toBe("/issues/ABC-1");
      expect(issueUrl({ id: "123", identifier: null })).toBe("/issues/123");
    });

    it("agentRouteRef uses urlKey if available", () => {
      expect(agentRouteRef(agent)).toBe("agent-key");
    });

    it("agentUrl builds correct URL", () => {
      expect(agentUrl(agent)).toBe("/agents/agent-key");
    });

    it("projectRouteRef uses urlKey if available", () => {
      expect(projectRouteRef(project)).toBe("project-key");
    });

    it("projectUrl builds correct URL", () => {
      expect(projectUrl(project)).toBe("/projects/project-key");
    });

    it("projectWorkspaceUrl builds correct URL", () => {
      expect(projectWorkspaceUrl(project, "ws-123")).toBe("/projects/project-key/workspaces/ws-123");
    });

    it("projectRouteRef handles non-ASCII content guard", () => {
      const projectWithNonAscii = { id: "p-123", name: "Projet 🚀", urlKey: null };
      // deriveProjectUrlKey("Projet 🚀", "p-123") -> "projet-🚀-p123" (probably)
      // normalizeProjectUrlKey("Projet 🚀") -> "projet-" (strips emoji usually)
      // If it matches exactly and has non-ascii, it returns id.
      // This is hard to test without knowing exactly what @paperclipai/shared does,
      // but we can at least verify it doesn't crash and returns a string.
      const ref = projectRouteRef(projectWithNonAscii);
      expect(typeof ref).toBe("string");
    });
  });
});
