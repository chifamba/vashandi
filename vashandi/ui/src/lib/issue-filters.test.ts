import { describe, expect, it } from "vitest";
import {
  defaultIssueFilterState,
  issueFilterLabel,
  issueFilterArraysEqual,
  toggleIssueFilterValue,
  applyIssueFilters,
  countActiveIssueFilters,
} from "./issue-filters";
import type { Issue } from "@paperclipai/shared";

describe("issue-filters", () => {
  describe("issueFilterLabel", () => {
    it("replaces underscores with spaces and capitalizes words", () => {
      expect(issueFilterLabel("in_progress")).toBe("In Progress");
      expect(issueFilterLabel("todo")).toBe("Todo");
      expect(issueFilterLabel("in_review")).toBe("In Review");
      expect(issueFilterLabel("some_very_long_status")).toBe("Some Very Long Status");
    });
  });

  describe("issueFilterArraysEqual", () => {
    it("returns true for identical arrays", () => {
      expect(issueFilterArraysEqual(["a", "b"], ["a", "b"])).toBe(true);
    });

    it("returns true for arrays with same elements in different order", () => {
      expect(issueFilterArraysEqual(["a", "b"], ["b", "a"])).toBe(true);
    });

    it("returns false for arrays of different lengths", () => {
      expect(issueFilterArraysEqual(["a"], ["a", "b"])).toBe(false);
    });

    it("returns false for arrays with different elements", () => {
      expect(issueFilterArraysEqual(["a", "b"], ["a", "c"])).toBe(false);
    });

    it("returns true for empty arrays", () => {
      expect(issueFilterArraysEqual([], [])).toBe(true);
    });
  });

  describe("toggleIssueFilterValue", () => {
    it("adds a value if it does not exist in the array", () => {
      expect(toggleIssueFilterValue(["a", "b"], "c")).toEqual(["a", "b", "c"]);
      expect(toggleIssueFilterValue([], "a")).toEqual(["a"]);
    });

    it("removes a value if it exists in the array", () => {
      expect(toggleIssueFilterValue(["a", "b", "c"], "b")).toEqual(["a", "c"]);
      expect(toggleIssueFilterValue(["a"], "a")).toEqual([]);
    });
  });

  describe("applyIssueFilters", () => {
    const createIssue = (overrides: Partial<Issue>): Issue => {
      return {
        id: "issue-1",
        status: "todo",
        priority: "medium",
        ...overrides,
      } as Issue;
    };

    it("returns all issues when no filters are active", () => {
      const issues = [createIssue({ id: "1" }), createIssue({ id: "2" })];
      const result = applyIssueFilters(issues, defaultIssueFilterState);
      expect(result).toHaveLength(2);
      expect(result).toEqual(issues);
    });

    it("filters out routine_execution when enableRoutineVisibilityFilter is true and showRoutineExecutions is false", () => {
      const issues = [
        createIssue({ id: "1", originKind: "routine_execution" }),
        createIssue({ id: "2", originKind: "user" }),
      ];
      const result = applyIssueFilters(issues, defaultIssueFilterState, null, true);
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("2");
    });

    it("includes routine_execution when enableRoutineVisibilityFilter is true and showRoutineExecutions is true", () => {
      const issues = [
        createIssue({ id: "1", originKind: "routine_execution" }),
        createIssue({ id: "2", originKind: "user" }),
      ];
      const result = applyIssueFilters(
        issues,
        { ...defaultIssueFilterState, showRoutineExecutions: true },
        null,
        true
      );
      expect(result).toHaveLength(2);
    });

    it("filters by status", () => {
      const issues = [
        createIssue({ id: "1", status: "todo" }),
        createIssue({ id: "2", status: "in_progress" }),
      ];
      const result = applyIssueFilters(issues, { ...defaultIssueFilterState, statuses: ["in_progress"] });
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("2");
    });

    it("filters by priority", () => {
      const issues = [
        createIssue({ id: "1", priority: "high" }),
        createIssue({ id: "2", priority: "low" }),
      ];
      const result = applyIssueFilters(issues, { ...defaultIssueFilterState, priorities: ["high"] });
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("1");
    });

    it("filters by assignees (specific agent)", () => {
      const issues = [
        createIssue({ id: "1", assigneeAgentId: "agent-1" }),
        createIssue({ id: "2", assigneeAgentId: "agent-2" }),
      ];
      const result = applyIssueFilters(issues, { ...defaultIssueFilterState, assignees: ["agent-1"] });
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("1");
    });

    it("filters by assignees (__unassigned)", () => {
      const issues = [
        createIssue({ id: "1" }), // unassigned
        createIssue({ id: "2", assigneeAgentId: "agent-1" }),
        createIssue({ id: "3", assigneeUserId: "user-1" }),
      ];
      const result = applyIssueFilters(issues, { ...defaultIssueFilterState, assignees: ["__unassigned"] });
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("1");
    });

    it("filters by assignees (__me)", () => {
      const issues = [
        createIssue({ id: "1", assigneeUserId: "user-1" }),
        createIssue({ id: "2", assigneeUserId: "user-2" }),
      ];
      const result = applyIssueFilters(
        issues,
        { ...defaultIssueFilterState, assignees: ["__me"] },
        "user-1"
      );
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("1");
    });

    it("filters by labels", () => {
      const issues = [
        createIssue({ id: "1", labelIds: ["label-1", "label-2"] }),
        createIssue({ id: "2", labelIds: ["label-3"] }),
        createIssue({ id: "3" }),
      ];
      const result = applyIssueFilters(issues, { ...defaultIssueFilterState, labels: ["label-1"] });
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("1");
    });

    it("filters by projects", () => {
      const issues = [
        createIssue({ id: "1", projectId: "proj-1" }),
        createIssue({ id: "2", projectId: "proj-2" }),
      ];
      const result = applyIssueFilters(issues, { ...defaultIssueFilterState, projects: ["proj-2"] });
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("2");
    });

    it("handles multiple filters (AND logic)", () => {
      const issues = [
        createIssue({ id: "1", status: "todo", priority: "high" }),
        createIssue({ id: "2", status: "todo", priority: "low" }),
        createIssue({ id: "3", status: "in_progress", priority: "high" }),
      ];
      const result = applyIssueFilters(issues, {
        ...defaultIssueFilterState,
        statuses: ["todo"],
        priorities: ["high"],
      });
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("1");
    });
  });

  describe("countActiveIssueFilters", () => {
    it("returns 0 for default state", () => {
      expect(countActiveIssueFilters(defaultIssueFilterState)).toBe(0);
    });

    it("counts each active filter array", () => {
      const state = {
        ...defaultIssueFilterState,
        statuses: ["todo"],
        priorities: ["high", "low"],
        labels: ["label-1"],
      };
      expect(countActiveIssueFilters(state)).toBe(3);
    });

    it("counts routine executions filter when enabled and active", () => {
      const state = {
        ...defaultIssueFilterState,
        showRoutineExecutions: true,
      };
      expect(countActiveIssueFilters(state, true)).toBe(1);
    });

    it("does not count routine executions filter when not enabled", () => {
      const state = {
        ...defaultIssueFilterState,
        showRoutineExecutions: true,
      };
      expect(countActiveIssueFilters(state, false)).toBe(0);
    });

    it("counts everything together", () => {
      const state = {
        statuses: ["todo"],
        priorities: ["high"],
        assignees: ["agent-1"],
        labels: ["label-1"],
        projects: ["proj-1"],
        showRoutineExecutions: true,
      };
      expect(countActiveIssueFilters(state, true)).toBe(6);
      expect(countActiveIssueFilters(state, false)).toBe(5);
    });
  });
});
