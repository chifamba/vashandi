import { describe, expect, it } from "vitest";
import {
  buildExecutionPolicy,
  principalFromSelectionValue,
  selectionValueFromPrincipal,
  stageParticipantValues,
} from "./issue-execution-policy";
import type { IssueExecutionPolicy, IssueExecutionStageParticipant } from "@paperclipai/shared";

describe("issue-execution-policy", () => {
  describe("principalFromSelectionValue", () => {
    it("returns agent principal for agent prefix", () => {
      expect(principalFromSelectionValue("agent:agent-123")).toEqual({
        type: "agent",
        agentId: "agent-123",
        userId: null,
      });
    });

    it("returns user principal for user prefix", () => {
      expect(principalFromSelectionValue("user:user-456")).toEqual({
        type: "user",
        userId: "user-456",
        agentId: null,
      });
    });

    it("returns null for invalid/empty value", () => {
      expect(principalFromSelectionValue("")).toBeNull();
    });
  });

  describe("selectionValueFromPrincipal", () => {
    it("returns correctly formatted string for agent principal", () => {
      expect(
        selectionValueFromPrincipal({
          type: "agent",
          agentId: "agent-123",
          userId: null,
        }),
      ).toBe("agent:agent-123");
    });

    it("returns correctly formatted string for user principal", () => {
      expect(
        selectionValueFromPrincipal({
          type: "user",
          userId: "user-456",
          agentId: null,
        }),
      ).toBe("user:user-456");
    });
  });

  describe("stageParticipantValues", () => {
    it("returns empty array for null/undefined policy", () => {
      expect(stageParticipantValues(null, "review")).toEqual([]);
      expect(stageParticipantValues(undefined, "approval")).toEqual([]);
    });

    it("returns empty array when stage type is not found", () => {
      const policy: IssueExecutionPolicy = {
        mode: "normal",
        commentRequired: true,
        stages: [
          {
            id: "stage-1",
            type: "review",
            approvalsNeeded: 1,
            participants: [
              { id: "p1", type: "user", userId: "user-1", agentId: null },
            ],
          },
        ],
      };
      expect(stageParticipantValues(policy, "approval")).toEqual([]);
    });

    it("returns mapped participant selection values for existing stage", () => {
      const policy: IssueExecutionPolicy = {
        mode: "normal",
        commentRequired: true,
        stages: [
          {
            id: "stage-1",
            type: "approval",
            approvalsNeeded: 1,
            participants: [
              { id: "p1", type: "user", userId: "user-1", agentId: null },
              { id: "p2", type: "agent", agentId: "agent-1", userId: null },
            ],
          },
        ],
      };
      expect(stageParticipantValues(policy, "approval")).toEqual([
        "user:user-1",
        "agent:agent-1",
      ]);
    });
  });

  describe("buildExecutionPolicy", () => {
    it("returns null when no participants are provided", () => {
      expect(
        buildExecutionPolicy({
          reviewerValues: [],
          approverValues: [],
        }),
      ).toBeNull();
    });

    it("creates policy with review stage", () => {
      const policy = buildExecutionPolicy({
        reviewerValues: ["user:user-1"],
        approverValues: [],
      });
      expect(policy).not.toBeNull();
      expect(policy?.mode).toBe("normal");
      expect(policy?.commentRequired).toBe(true);
      expect(policy?.stages).toHaveLength(1);
      expect(policy?.stages[0].type).toBe("review");
      expect(policy?.stages[0].participants).toHaveLength(1);
      expect(policy?.stages[0].participants[0].type).toBe("user");
      expect(policy?.stages[0].participants[0].userId).toBe("user-1");
      expect(policy?.stages[0].participants[0].id).toBeDefined();
    });

    it("creates policy with approval stage", () => {
      const policy = buildExecutionPolicy({
        reviewerValues: [],
        approverValues: ["agent:agent-1"],
      });
      expect(policy).not.toBeNull();
      expect(policy?.stages).toHaveLength(1);
      expect(policy?.stages[0].type).toBe("approval");
      expect(policy?.stages[0].participants).toHaveLength(1);
      expect(policy?.stages[0].participants[0].type).toBe("agent");
      expect(policy?.stages[0].participants[0].agentId).toBe("agent-1");
    });

    it("creates policy with both review and approval stages", () => {
      const policy = buildExecutionPolicy({
        reviewerValues: ["user:user-1"],
        approverValues: ["agent:agent-1"],
      });
      expect(policy?.stages).toHaveLength(2);
      expect(policy?.stages.map((s) => s.type)).toContain("review");
      expect(policy?.stages.map((s) => s.type)).toContain("approval");
    });

    it("preserves IDs of existing participants and mode", () => {
      const existingPolicy: IssueExecutionPolicy = {
        mode: "normal",
        commentRequired: true,
        stages: [
          {
            id: "stage-1",
            type: "review",
            approvalsNeeded: 1,
            participants: [
              { id: "existing-p-id", type: "user", userId: "user-1", agentId: null },
            ],
          },
        ],
      };

      const policy = buildExecutionPolicy({
        existingPolicy,
        reviewerValues: ["user:user-1", "agent:agent-2"],
        approverValues: [],
      });

      expect(policy?.mode).toBe("strict");
      expect(policy?.stages[0].id).toBe("stage-1");

      const user1 = policy?.stages[0].participants.find(p => p.userId === "user-1");
      expect(user1?.id).toBe("existing-p-id");

      const agent2 = policy?.stages[0].participants.find(p => p.agentId === "agent-2");
      expect(agent2?.id).toBeDefined();
      expect(agent2?.id).not.toBe("existing-p-id");
    });

    it("ignores empty or invalid selection values", () => {
      const policy = buildExecutionPolicy({
        reviewerValues: ["", "user:user-1"],
        approverValues: [],
      });
      expect(policy?.stages[0].participants).toHaveLength(1);
      expect(policy?.stages[0].participants[0].userId).toBe("user-1");
    });
  });
});
