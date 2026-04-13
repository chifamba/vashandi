import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import {
  getRecentAssigneeIds,
  trackRecentAssignee,
  sortAgentsByRecency,
} from "./recent-assignees";

const STORAGE_KEY = "paperclip:recent-assignees";

describe("recent-assignees", () => {
  let localStorageMock: Record<string, string> = {};

  beforeEach(() => {
    localStorageMock = {};
    const mockGetItem = vi.fn((key: string) => localStorageMock[key] || null);
    const mockSetItem = vi.fn((key: string, value: string) => {
      localStorageMock[key] = value.toString();
    });

    vi.stubGlobal("localStorage", {
      getItem: mockGetItem,
      setItem: mockSetItem,
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  describe("getRecentAssigneeIds", () => {
    it("returns an empty array if no data is in localStorage", () => {
      expect(getRecentAssigneeIds()).toEqual([]);
    });

    it("returns an empty array if invalid JSON is in localStorage", () => {
      localStorageMock[STORAGE_KEY] = "{ invalid json ";
      expect(getRecentAssigneeIds()).toEqual([]);
    });

    it("returns an empty array if parsed JSON is not an array", () => {
      localStorageMock[STORAGE_KEY] = JSON.stringify({ not: "an array" });
      expect(getRecentAssigneeIds()).toEqual([]);
    });

    it("returns parsed array of IDs", () => {
      localStorageMock[STORAGE_KEY] = JSON.stringify(["id1", "id2"]);
      expect(getRecentAssigneeIds()).toEqual(["id1", "id2"]);
    });
  });

  describe("trackRecentAssignee", () => {
    it("does nothing if agentId is empty", () => {
      trackRecentAssignee("");
      expect(localStorageMock[STORAGE_KEY]).toBeUndefined();
    });

    it("adds a new agentId to the beginning of the list", () => {
      localStorageMock[STORAGE_KEY] = JSON.stringify(["id1", "id2"]);
      trackRecentAssignee("newId");
      expect(JSON.parse(localStorageMock[STORAGE_KEY])).toEqual(["newId", "id1", "id2"]);
    });

    it("moves an existing agentId to the beginning of the list", () => {
      localStorageMock[STORAGE_KEY] = JSON.stringify(["id1", "id2", "id3"]);
      trackRecentAssignee("id2");
      expect(JSON.parse(localStorageMock[STORAGE_KEY])).toEqual(["id2", "id1", "id3"]);
    });

    it("respects MAX_RECENT limit (10)", () => {
      const initialList = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "10"];
      localStorageMock[STORAGE_KEY] = JSON.stringify(initialList);
      trackRecentAssignee("11");
      expect(JSON.parse(localStorageMock[STORAGE_KEY])).toEqual([
        "11", "1", "2", "3", "4", "5", "6", "7", "8", "9"
      ]);
    });
  });

  describe("sortAgentsByRecency", () => {
    const agents = [
      { id: "A", name: "Alice" },
      { id: "B", name: "Bob" },
      { id: "C", name: "Charlie" },
      { id: "D", name: "David" },
    ];

    it("sorts by recentIds order correctly", () => {
      const recentIds = ["C", "A"];
      const sorted = sortAgentsByRecency(agents, recentIds);
      expect(sorted.map((a) => a.id)).toEqual(["C", "A", "B", "D"]);
    });

    it("sorts agents not in recentIds alphabetically by name", () => {
      const recentIds = ["D"];
      const sorted = sortAgentsByRecency(agents, recentIds);
      // D is recent, so it comes first.
      // Then Alice (A), Bob (B), Charlie (C) alphabetically
      expect(sorted.map((a) => a.id)).toEqual(["D", "A", "B", "C"]);
    });

    it("handles empty recentIds by sorting alphabetically", () => {
      const recentIds: string[] = [];
      const sorted = sortAgentsByRecency(agents, recentIds);
      expect(sorted.map((a) => a.id)).toEqual(["A", "B", "C", "D"]);
    });

    it("handles recent IDs not present in agents list", () => {
      const recentIds = ["X", "B", "Y", "A"];
      const sorted = sortAgentsByRecency(agents, recentIds);
      // B is at index 1, A is at index 3 in recentIds. B comes before A.
      // C and D are not in recentIds, so they come after A, sorted alphabetically.
      expect(sorted.map((a) => a.id)).toEqual(["B", "A", "C", "D"]);
    });
  });
});
