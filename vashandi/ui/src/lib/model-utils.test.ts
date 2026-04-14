import { describe, it, expect } from "vitest";
import {
  extractProviderId,
} from "./model-utils";

describe("model-utils", () => {
  describe("extractProviderId", () => {
    it("extracts the provider from a standard modelId", () => {
      expect(extractProviderId("openai/gpt-4")).toBe("openai");
    });

    it("returns null if there is no slash", () => {
      expect(extractProviderId("gpt-4")).toBeNull();
    });

    it("handles whitespace around the provider and model", () => {
      expect(extractProviderId("  anthropic / claude-3 ")).toBe("anthropic");
    });

    it("returns null if provider is empty before the slash", () => {
      expect(extractProviderId("/gpt-4")).toBeNull();
      expect(extractProviderId("  /gpt-4")).toBeNull();
    });
  });
});
