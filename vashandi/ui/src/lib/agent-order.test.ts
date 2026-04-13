import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import {
  getAgentOrderStorageKey,
  readAgentOrder,
  writeAgentOrder,
  AGENT_ORDER_UPDATED_EVENT,
} from "./agent-order";

describe("agent-order utility functions", () => {
  describe("getAgentOrderStorageKey", () => {
    it("returns storage key with companyId and resolved userId", () => {
      expect(getAgentOrderStorageKey("company123", "user456")).toBe("paperclip.agentOrder:company123:user456");
    });

    it("resolves null/undefined user to anonymous", () => {
      expect(getAgentOrderStorageKey("company123", null)).toBe("paperclip.agentOrder:company123:anonymous");
      expect(getAgentOrderStorageKey("company123", undefined)).toBe("paperclip.agentOrder:company123:anonymous");
    });

    it("resolves empty string or whitespace user to anonymous", () => {
      expect(getAgentOrderStorageKey("company123", "")).toBe("paperclip.agentOrder:company123:anonymous");
      expect(getAgentOrderStorageKey("company123", "   ")).toBe("paperclip.agentOrder:company123:anonymous");
    });
  });

  describe("normalizeIdList (indirectly through writeAgentOrder and readAgentOrder)", () => {
    const storageKey = "testKey";

    beforeEach(() => {
      vi.stubGlobal("localStorage", {
        getItem: vi.fn(),
        setItem: vi.fn(),
      });
      vi.stubGlobal("window", {
        dispatchEvent: vi.fn(),
      });
      vi.stubGlobal("CustomEvent", class CustomEvent {
        constructor(type: string, options: any) {
          return { type, ...options };
        }
      });
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    it("filters out invalid entries on write and stringifies", () => {
      const mockSetItem = vi.fn();
      vi.stubGlobal("localStorage", { setItem: mockSetItem });

      const inputIds = ["valid1", "", "valid2", null, undefined, 123, "valid3"] as unknown as string[];
      writeAgentOrder(storageKey, inputIds);

      expect(mockSetItem).toHaveBeenCalledWith(
        storageKey,
        JSON.stringify(["valid1", "valid2", "valid3"])
      );
    });

    it("handles non-array inputs gracefully in readAgentOrder by returning empty array", () => {
      vi.stubGlobal("localStorage", {
        getItem: vi.fn().mockReturnValue(JSON.stringify("not an array")),
      });

      expect(readAgentOrder(storageKey)).toEqual([]);
    });

    it("filters invalid entries read from localStorage", () => {
      vi.stubGlobal("localStorage", {
        getItem: vi.fn().mockReturnValue(JSON.stringify(["valid1", "", 123, "valid2"])),
      });

      expect(readAgentOrder(storageKey)).toEqual(["valid1", "valid2"]);
    });
  });
});

describe("storage functions", () => {
  const storageKey = "testStorageKey";

  beforeEach(() => {
    vi.stubGlobal("localStorage", {
      getItem: vi.fn(),
      setItem: vi.fn(),
    });
    vi.stubGlobal("window", {
      dispatchEvent: vi.fn(),
    });
    vi.stubGlobal("CustomEvent", class CustomEvent {
      constructor(type: string, options: any) {
        return { type, ...options };
      }
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe("readAgentOrder", () => {
    it("returns empty array if nothing in localStorage", () => {
      vi.stubGlobal("localStorage", {
        getItem: vi.fn().mockReturnValue(null),
      });

      expect(readAgentOrder(storageKey)).toEqual([]);
    });

    it("returns empty array if JSON parse fails", () => {
      vi.stubGlobal("localStorage", {
        getItem: vi.fn().mockReturnValue("invalid json"),
      });

      expect(readAgentOrder(storageKey)).toEqual([]);
    });

    it("returns normalized parsed array from localStorage", () => {
      vi.stubGlobal("localStorage", {
        getItem: vi.fn().mockReturnValue(JSON.stringify(["agent1", "agent2"])),
      });

      expect(readAgentOrder(storageKey)).toEqual(["agent1", "agent2"]);
    });
  });

  describe("writeAgentOrder", () => {
    it("writes stringified normalized array to localStorage", () => {
      const mockSetItem = vi.fn();
      vi.stubGlobal("localStorage", { setItem: mockSetItem });

      writeAgentOrder(storageKey, ["agent1", "agent2"]);

      expect(mockSetItem).toHaveBeenCalledWith(
        storageKey,
        JSON.stringify(["agent1", "agent2"])
      );
    });

    it("dispatches custom event with updated order on window", () => {
      const mockDispatchEvent = vi.fn();
      vi.stubGlobal("window", { dispatchEvent: mockDispatchEvent });

      writeAgentOrder(storageKey, ["agent1", "agent2"]);

      expect(mockDispatchEvent).toHaveBeenCalledTimes(1);
      const eventArg = mockDispatchEvent.mock.calls[0][0];
      expect(eventArg.type).toBe(AGENT_ORDER_UPDATED_EVENT);
      expect(eventArg.detail).toEqual({
        storageKey,
        orderedIds: ["agent1", "agent2"],
      });
    });

    it("ignores localStorage write failures silently", () => {
      const mockSetItem = vi.fn().mockImplementation(() => {
        throw new Error("Quota Exceeded");
      });
      vi.stubGlobal("localStorage", { setItem: mockSetItem });
      const mockDispatchEvent = vi.fn();
      vi.stubGlobal("window", { dispatchEvent: mockDispatchEvent });

      expect(() => {
        writeAgentOrder(storageKey, ["agent1"]);
      }).not.toThrow();

      // Should still dispatch event even if storage fails
      expect(mockDispatchEvent).toHaveBeenCalledTimes(1);
    });
  });
});

import type { Agent } from "@paperclipai/shared";
import { sortAgentsByDefaultSidebarOrder, sortAgentsByStoredOrder } from "./agent-order";

describe("sorting functions", () => {
  const createMockAgent = (id: string, name: string, reportsTo: string | null = null): Agent => {
    return {
      id,
      companyId: "comp1",
      name,
      urlKey: id,
      role: "agent",
      title: null,
      icon: null,
      status: "active",
      reportsTo,
      capabilities: null,
      adapterType: "openai",
      adapterConfig: {},
      runtimeConfig: {},
      budgetMonthlyCents: 0,
      spentMonthlyCents: 0,
      pauseReason: null,
      pausedAt: null,
      permissions: { canCreateAgents: false },
      lastHeartbeatAt: null,
      metadata: null,
      createdAt: new Date(),
      updatedAt: new Date(),
    } as unknown as Agent;
  };

  describe("sortAgentsByDefaultSidebarOrder", () => {
    it("returns empty array when given empty array", () => {
      expect(sortAgentsByDefaultSidebarOrder([])).toEqual([]);
    });

    it("sorts top-level agents alphabetically", () => {
      const agents = [
        createMockAgent("3", "Charlie"),
        createMockAgent("1", "Alice"),
        createMockAgent("2", "Bob"),
      ];

      const sorted = sortAgentsByDefaultSidebarOrder(agents);
      expect(sorted.map(a => a.name)).toEqual(["Alice", "Bob", "Charlie"]);
    });

    it("groups children under their parents, sorting alphabetically within groups", () => {
      const parent2 = createMockAgent("p2", "Zebra");
      const parent1 = createMockAgent("p1", "Alpha");
      const child2a = createMockAgent("c2a", "Delta", "p2");
      const child1b = createMockAgent("c1b", "Gamma", "p1");
      const child1a = createMockAgent("c1a", "Beta", "p1");

      const agents = [parent2, child2a, child1b, parent1, child1a];
      const sorted = sortAgentsByDefaultSidebarOrder(agents);

      // Top level: Alpha, Zebra
      // Alpha children: Beta, Gamma
      // Zebra children: Delta
      // Note: The algorithm does breadth-first or parent-then-children?
      // Let's check algorithm:
      // queue = [...topLevel]
      // while queue > 0:
      //   agent = queue.shift()
      //   push agent to sorted
      //   queue.push(...childrenOf(agent))
      // It's BFS order!
      // Top level: [Alpha, Zebra] -> push Alpha, push Zebra. queue: [Beta, Gamma, Delta] -> push Beta, push Gamma, push Delta

      // Let's actually run it to see the expected array order.
      expect(sorted.map(a => a.name)).toEqual(["Alpha", "Zebra", "Beta", "Gamma", "Delta"]);
    });
  });

  describe("sortAgentsByStoredOrder", () => {
    it("returns empty array when given empty array", () => {
      expect(sortAgentsByStoredOrder([], ["1", "2"])).toEqual([]);
    });

    it("returns default sorted array when orderedIds is empty", () => {
      const agents = [
        createMockAgent("3", "Charlie"),
        createMockAgent("1", "Alice"),
        createMockAgent("2", "Bob"),
      ];

      const sorted = sortAgentsByStoredOrder(agents, []);
      expect(sorted.map(a => a.name)).toEqual(["Alice", "Bob", "Charlie"]);
    });

    it("sorts according to orderedIds first, then default sorted for the rest", () => {
      const parent2 = createMockAgent("p2", "Zebra");
      const parent1 = createMockAgent("p1", "Alpha");
      const child2a = createMockAgent("c2a", "Delta", "p2");
      const child1b = createMockAgent("c1b", "Gamma", "p1");
      const child1a = createMockAgent("c1a", "Beta", "p1");

      const agents = [parent2, child2a, child1b, parent1, child1a];

      // Assume user dragged Delta to the top, and Gamma to the second.
      // orderedIds: ["c2a", "c1b"]
      // Expected rest: default sorted excluding c2a, c1b.
      // default: Alpha, Zebra, Beta, Gamma, Delta
      // rest: Alpha, Zebra, Beta
      // total expected: Delta, Gamma, Alpha, Zebra, Beta

      const sorted = sortAgentsByStoredOrder(agents, ["c2a", "c1b"]);
      expect(sorted.map(a => a.name)).toEqual(["Delta", "Gamma", "Alpha", "Zebra", "Beta"]);
    });

    it("ignores orderedIds that are not in the agents list", () => {
      const agents = [
        createMockAgent("3", "Charlie"),
        createMockAgent("1", "Alice"),
        createMockAgent("2", "Bob"),
      ];

      const sorted = sortAgentsByStoredOrder(agents, ["missing", "2", "ghost"]);
      // orderedIds valid: Bob
      // rest (default excluding Bob): Alice, Charlie
      // total: Bob, Alice, Charlie
      expect(sorted.map(a => a.name)).toEqual(["Bob", "Alice", "Charlie"]);
    });
  });
});
