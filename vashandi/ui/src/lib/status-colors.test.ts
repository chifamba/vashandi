import { describe, expect, it } from "vitest";
import {
  issueStatusIcon,
  issueStatusIconDefault,
  issueStatusText,
  issueStatusTextDefault,
  statusBadge,
  statusBadgeDefault,
  agentStatusDot,
  agentStatusDotDefault,
  priorityColor,
  priorityColorDefault,
} from "./status-colors";

describe("status-colors", () => {
  describe("issueStatusIcon", () => {
    it("should export colors for all defined issue statuses", () => {
      expect(issueStatusIcon.backlog).toBeDefined();
      expect(issueStatusIcon.todo).toBeDefined();
      expect(issueStatusIcon.in_progress).toBeDefined();
      expect(issueStatusIcon.in_review).toBeDefined();
      expect(issueStatusIcon.done).toBeDefined();
      expect(issueStatusIcon.cancelled).toBeDefined();
      expect(issueStatusIcon.blocked).toBeDefined();
    });

    it("should have a default fallback color of type string", () => {
      expect(issueStatusIconDefault).toBeDefined();
      expect(typeof issueStatusIconDefault).toBe("string");
    });
  });

  describe("issueStatusText", () => {
    it("should export text colors for all defined issue statuses", () => {
      expect(issueStatusText.backlog).toBeDefined();
      expect(issueStatusText.todo).toBeDefined();
      expect(issueStatusText.in_progress).toBeDefined();
      expect(issueStatusText.in_review).toBeDefined();
      expect(issueStatusText.done).toBeDefined();
      expect(issueStatusText.cancelled).toBeDefined();
      expect(issueStatusText.blocked).toBeDefined();
    });

    it("should have a default fallback text color of type string", () => {
      expect(issueStatusTextDefault).toBeDefined();
      expect(typeof issueStatusTextDefault).toBe("string");
    });
  });

  describe("statusBadge", () => {
    it("should export colors for agent statuses", () => {
      expect(statusBadge.active).toBeDefined();
      expect(statusBadge.running).toBeDefined();
      expect(statusBadge.paused).toBeDefined();
      expect(statusBadge.idle).toBeDefined();
      expect(statusBadge.archived).toBeDefined();
    });

    it("should export colors for goal statuses", () => {
      expect(statusBadge.planned).toBeDefined();
      expect(statusBadge.achieved).toBeDefined();
      expect(statusBadge.completed).toBeDefined();
    });

    it("should export colors for run statuses", () => {
      expect(statusBadge.failed).toBeDefined();
      expect(statusBadge.timed_out).toBeDefined();
      expect(statusBadge.succeeded).toBeDefined();
      expect(statusBadge.error).toBeDefined();
      expect(statusBadge.terminated).toBeDefined();
      expect(statusBadge.pending).toBeDefined();
    });

    it("should export colors for approval statuses", () => {
      expect(statusBadge.pending_approval).toBeDefined();
      expect(statusBadge.revision_requested).toBeDefined();
      expect(statusBadge.approved).toBeDefined();
      expect(statusBadge.rejected).toBeDefined();
    });

    it("should export colors for issue statuses", () => {
      expect(statusBadge.backlog).toBeDefined();
      expect(statusBadge.todo).toBeDefined();
      expect(statusBadge.in_progress).toBeDefined();
      expect(statusBadge.in_review).toBeDefined();
      expect(statusBadge.blocked).toBeDefined();
      expect(statusBadge.done).toBeDefined();
      expect(statusBadge.cancelled).toBeDefined();
    });

    it("should have a default fallback color of type string", () => {
      expect(statusBadgeDefault).toBeDefined();
      expect(typeof statusBadgeDefault).toBe("string");
    });
  });

  describe("agentStatusDot", () => {
    it("should export colors for agent dot statuses", () => {
      expect(agentStatusDot.running).toBeDefined();
      expect(agentStatusDot.active).toBeDefined();
      expect(agentStatusDot.paused).toBeDefined();
      expect(agentStatusDot.idle).toBeDefined();
      expect(agentStatusDot.pending_approval).toBeDefined();
      expect(agentStatusDot.error).toBeDefined();
      expect(agentStatusDot.archived).toBeDefined();
    });

    it("should have a default fallback color of type string", () => {
      expect(agentStatusDotDefault).toBeDefined();
      expect(typeof agentStatusDotDefault).toBe("string");
    });
  });

  describe("priorityColor", () => {
    it("should export colors for priorities", () => {
      expect(priorityColor.critical).toBeDefined();
      expect(priorityColor.high).toBeDefined();
      expect(priorityColor.medium).toBeDefined();
      expect(priorityColor.low).toBeDefined();
    });

    it("should have a default fallback priority color of type string", () => {
      expect(priorityColorDefault).toBeDefined();
      expect(typeof priorityColorDefault).toBe("string");
    });
  });
});
