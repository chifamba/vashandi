import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  PROJECT_ORDER_UPDATED_EVENT,
  getProjectOrderStorageKey,
  readProjectOrder,
  writeProjectOrder,
  sortProjectsByStoredOrder,
} from "./project-order";
import type { Project } from "@paperclipai/shared";

// Setup browser globals for tests since they don't seem to be configured by default
const mockStorage = {
  getItem: vi.fn(),
  setItem: vi.fn(),
};

global.localStorage = mockStorage as any;
global.window = {
  dispatchEvent: vi.fn(),
} as any;
global.CustomEvent = class CustomEvent {
  type: string;
  detail: any;
  constructor(type: string, options: any) {
    this.type = type;
    this.detail = options?.detail;
  }
} as any;

describe("project-order", () => {
  describe("getProjectOrderStorageKey", () => {
    it("returns correct key with valid companyId and userId", () => {
      expect(getProjectOrderStorageKey("company-1", "user-1")).toBe("paperclip.projectOrder:company-1:user-1");
    });

    it("falls back to anonymous when userId is null", () => {
      expect(getProjectOrderStorageKey("company-1", null)).toBe("paperclip.projectOrder:company-1:anonymous");
    });

    it("falls back to anonymous when userId is undefined", () => {
      expect(getProjectOrderStorageKey("company-1", undefined)).toBe("paperclip.projectOrder:company-1:anonymous");
    });

    it("falls back to anonymous when userId is empty string", () => {
      expect(getProjectOrderStorageKey("company-1", "")).toBe("paperclip.projectOrder:company-1:anonymous");
    });

    it("falls back to anonymous when userId is whitespace", () => {
      expect(getProjectOrderStorageKey("company-1", "   ")).toBe("paperclip.projectOrder:company-1:anonymous");
    });

    it("trims userId", () => {
      expect(getProjectOrderStorageKey("company-1", " user-1 ")).toBe("paperclip.projectOrder:company-1:user-1");
    });
  });

  describe("readProjectOrder", () => {
    beforeEach(() => {
      vi.clearAllMocks();
    });

    it("returns array of strings when valid JSON exists", () => {
      mockStorage.getItem.mockReturnValue(JSON.stringify(["proj-1", "proj-2"]));
      expect(readProjectOrder("test-key")).toEqual(["proj-1", "proj-2"]);
      expect(mockStorage.getItem).toHaveBeenCalledWith("test-key");
    });

    it("returns empty array when key does not exist", () => {
      mockStorage.getItem.mockReturnValue(null);
      expect(readProjectOrder("test-key")).toEqual([]);
    });

    it("returns empty array when JSON is invalid", () => {
      mockStorage.getItem.mockReturnValue("{ invalid json");
      expect(readProjectOrder("test-key")).toEqual([]);
    });

    it("filters out non-strings from JSON array", () => {
      mockStorage.getItem.mockReturnValue(JSON.stringify(["proj-1", 123, null, "proj-2", {}]));
      expect(readProjectOrder("test-key")).toEqual(["proj-1", "proj-2"]);
    });

    it("filters out empty strings from JSON array", () => {
      mockStorage.getItem.mockReturnValue(JSON.stringify(["proj-1", "", "proj-2"]));
      expect(readProjectOrder("test-key")).toEqual(["proj-1", "proj-2"]);
    });

    it("returns empty array if JSON is not an array", () => {
      mockStorage.getItem.mockReturnValue(JSON.stringify({ "proj-1": true }));
      expect(readProjectOrder("test-key")).toEqual([]);
    });
  });

  describe("writeProjectOrder", () => {
    beforeEach(() => {
      vi.clearAllMocks();
    });

    it("saves normalized IDs to localStorage", () => {
      writeProjectOrder("test-key", ["proj-1", "", "proj-2", "proj-1"]);
      expect(mockStorage.setItem).toHaveBeenCalledWith("test-key", JSON.stringify(["proj-1", "proj-2", "proj-1"]));
    });

    it("dispatches PROJECT_ORDER_UPDATED_EVENT", () => {
      writeProjectOrder("test-key", ["proj-1"]);

      expect(global.window.dispatchEvent).toHaveBeenCalledTimes(1);
      const event = (global.window.dispatchEvent as any).mock.calls[0][0];
      expect(event.type).toBe(PROJECT_ORDER_UPDATED_EVENT);
      expect(event.detail).toEqual({
        storageKey: "test-key",
        orderedIds: ["proj-1"],
      });
    });

    it("ignores localStorage.setItem errors", () => {
      mockStorage.setItem.mockImplementation(() => {
        throw new Error("QuotaExceededError");
      });

      expect(() => {
        writeProjectOrder("test-key", ["proj-1"]);
      }).not.toThrow();

      expect(global.window.dispatchEvent).toHaveBeenCalledTimes(1);
    });

    it("handles non-string/empty strings properly in writeProjectOrder", () => {
      // simulate ts ignore or untyped call
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      writeProjectOrder("test-key", ["proj-1", 123 as any, "", "proj-2"]);
      expect(mockStorage.setItem).toHaveBeenCalledWith("test-key", JSON.stringify(["proj-1", "proj-2"]));
    });
  });

  describe("sortProjectsByStoredOrder", () => {
    const projects = [
      { id: "proj-1", name: "Project 1" } as Project,
      { id: "proj-2", name: "Project 2" } as Project,
      { id: "proj-3", name: "Project 3" } as Project,
    ];

    it("returns empty array when projects array is empty", () => {
      expect(sortProjectsByStoredOrder([], ["proj-1"])).toEqual([]);
    });

    it("returns original projects when orderedIds is empty", () => {
      expect(sortProjectsByStoredOrder(projects, [])).toEqual(projects);
    });

    it("sorts correctly with orderedIds, pushing unlisted projects to the end", () => {
      const result = sortProjectsByStoredOrder(projects, ["proj-2", "proj-1"]);
      expect(result).toEqual([
        { id: "proj-2", name: "Project 2" },
        { id: "proj-1", name: "Project 1" },
        { id: "proj-3", name: "Project 3" },
      ]);
    });

    it("ignores orderedIds that do not correspond to any project", () => {
      const result = sortProjectsByStoredOrder(projects, ["proj-999", "proj-3", "proj-1"]);
      expect(result).toEqual([
        { id: "proj-3", name: "Project 3" },
        { id: "proj-1", name: "Project 1" },
        { id: "proj-2", name: "Project 2" },
      ]);
    });

    it("preserves ordering of unlisted projects", () => {
       const result = sortProjectsByStoredOrder(projects, ["proj-3"]);
       expect(result).toEqual([
         { id: "proj-3", name: "Project 3" },
         { id: "proj-1", name: "Project 1" },
         { id: "proj-2", name: "Project 2" },
       ]);
    });
  });
});
