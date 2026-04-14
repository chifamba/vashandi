import { describe, expect, it, vi, afterEach } from "vitest";
import { getWorktreeUiBranding } from "./worktree-branding";

describe("getWorktreeUiBranding", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function mockMeta(tags: Record<string, string>) {
    const querySelector = vi.fn((selector: string) => {
      const match = selector.match(/meta\[name="(.*?)"\]/);
      if (!match) return null;
      const name = match[1];
      if (tags[name] !== undefined) {
        return {
          getAttribute: (attr: string) => attr === "content" ? tags[name] : null,
        };
      }
      return null;
    });

    vi.stubGlobal("document", { querySelector });
  }

  it("returns null when document is undefined", () => {
    expect(getWorktreeUiBranding()).toBeNull();
  });

  it("returns null when not enabled", () => {
    mockMeta({
      "paperclip-worktree-enabled": "false",
    });
    expect(getWorktreeUiBranding()).toBeNull();
  });

  it("returns null when name is missing", () => {
    mockMeta({
      "paperclip-worktree-enabled": "true",
      "paperclip-worktree-color": "#ffffff",
    });
    expect(getWorktreeUiBranding()).toBeNull();
  });

  it("returns null when color is missing", () => {
    mockMeta({
      "paperclip-worktree-enabled": "true",
      "paperclip-worktree-name": "Test",
    });
    expect(getWorktreeUiBranding()).toBeNull();
  });

  it("returns branding with generated text color (dark text for light background)", () => {
    mockMeta({
      "paperclip-worktree-enabled": "true",
      "paperclip-worktree-name": "Test",
      "paperclip-worktree-color": "#ffffff",
    });
    const branding = getWorktreeUiBranding();
    expect(branding).toEqual({
      enabled: true,
      name: "Test",
      color: "#ffffff",
      textColor: "#111827", // dark text for light background
    });
  });

  it("returns branding with generated text color (light text for dark background)", () => {
    mockMeta({
      "paperclip-worktree-enabled": "true",
      "paperclip-worktree-name": "Test",
      "paperclip-worktree-color": "#000000",
    });
    const branding = getWorktreeUiBranding();
    expect(branding).toEqual({
      enabled: true,
      name: "Test",
      color: "#000000",
      textColor: "#f8fafc", // light text for dark background
    });
  });

  it("returns branding with explicit text color from meta tag", () => {
    mockMeta({
      "paperclip-worktree-enabled": "true",
      "paperclip-worktree-name": "Test",
      "paperclip-worktree-color": "#ffffff",
      "paperclip-worktree-text-color": "#ff0000",
    });
    const branding = getWorktreeUiBranding();
    expect(branding).toEqual({
      enabled: true,
      name: "Test",
      color: "#ffffff",
      textColor: "#ff0000",
    });
  });

  it("handles empty meta content gracefully", () => {
    const querySelector = vi.fn((selector: string) => {
      return {
        getAttribute: (attr: string) => attr === "content" ? "   " : null,
      };
    });
    vi.stubGlobal("document", { querySelector });

    expect(getWorktreeUiBranding()).toBeNull();
  });

  it("handles missing meta tag gracefully", () => {
    const querySelector = vi.fn((selector: string) => {
      return null;
    });
    vi.stubGlobal("document", { querySelector });

    expect(getWorktreeUiBranding()).toBeNull();
  });

  it("normalizes short hex colors", () => {
    mockMeta({
      "paperclip-worktree-enabled": "true",
      "paperclip-worktree-name": "Test",
      "paperclip-worktree-color": "#fff",
    });
    const branding = getWorktreeUiBranding();
    expect(branding?.color).toBe("#ffffff");
  });

  it("normalizes hex colors without hash", () => {
    mockMeta({
      "paperclip-worktree-enabled": "true",
      "paperclip-worktree-name": "Test",
      "paperclip-worktree-color": "000000",
    });
    const branding = getWorktreeUiBranding();
    expect(branding?.color).toBe("#000000");
  });

  it("returns null for invalid color formats", () => {
    mockMeta({
      "paperclip-worktree-enabled": "true",
      "paperclip-worktree-name": "Test",
      "paperclip-worktree-color": "red",
    });
    const branding = getWorktreeUiBranding();
    expect(branding).toBeNull();
  });
});
