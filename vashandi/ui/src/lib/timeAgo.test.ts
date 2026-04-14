import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { timeAgo } from "./timeAgo";

describe("timeAgo", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2024-01-01T12:00:00.000Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns 'just now' for times under a minute", () => {
    const now = new Date("2024-01-01T12:00:00.000Z");
    expect(timeAgo(now)).toBe("just now");

    const fiftyNineSecondsAgo = new Date(now.getTime() - 59 * 1000);
    expect(timeAgo(fiftyNineSecondsAgo)).toBe("just now");
  });

  it("returns minutes ago for times under an hour", () => {
    const now = new Date("2024-01-01T12:00:00.000Z");

    const exactlyOneMinuteAgo = new Date(now.getTime() - 60 * 1000);
    expect(timeAgo(exactlyOneMinuteAgo)).toBe("1m ago");

    const fiftyNineMinutesAgo = new Date(now.getTime() - 59 * 60 * 1000);
    expect(timeAgo(fiftyNineMinutesAgo)).toBe("59m ago");
  });

  it("returns hours ago for times under a day", () => {
    const now = new Date("2024-01-01T12:00:00.000Z");

    const exactlyOneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    expect(timeAgo(exactlyOneHourAgo)).toBe("1h ago");

    const twentyThreeHoursAgo = new Date(now.getTime() - 23 * 60 * 60 * 1000);
    expect(timeAgo(twentyThreeHoursAgo)).toBe("23h ago");
  });

  it("returns days ago for times under a week", () => {
    const now = new Date("2024-01-01T12:00:00.000Z");

    const exactlyOneDayAgo = new Date(now.getTime() - 24 * 60 * 60 * 1000);
    expect(timeAgo(exactlyOneDayAgo)).toBe("1d ago");

    const sixDaysAgo = new Date(now.getTime() - 6 * 24 * 60 * 60 * 1000);
    expect(timeAgo(sixDaysAgo)).toBe("6d ago");
  });

  it("returns weeks ago for times under a month (30 days)", () => {
    const now = new Date("2024-01-01T12:00:00.000Z");

    const exactlyOneWeekAgo = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
    expect(timeAgo(exactlyOneWeekAgo)).toBe("1w ago");

    const fourWeeksAgo = new Date(now.getTime() - 29 * 24 * 60 * 60 * 1000); // 29 days is 4 weeks
    expect(timeAgo(fourWeeksAgo)).toBe("4w ago");
  });

  it("returns months ago for times over 30 days", () => {
    const now = new Date("2024-01-01T12:00:00.000Z");

    const thirtyDaysAgo = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);
    expect(timeAgo(thirtyDaysAgo)).toBe("1mo ago");

    const sixtyDaysAgo = new Date(now.getTime() - 60 * 24 * 60 * 60 * 1000);
    expect(timeAgo(sixtyDaysAgo)).toBe("2mo ago");

    const oneYearAgo = new Date(now.getTime() - 365 * 24 * 60 * 60 * 1000);
    expect(timeAgo(oneYearAgo)).toBe("12mo ago");
  });

  it("accepts string dates", () => {
    expect(timeAgo("2024-01-01T11:59:00.000Z")).toBe("1m ago");
    expect(timeAgo("2024-01-01T11:00:00.000Z")).toBe("1h ago");
    expect(timeAgo("2023-12-31T12:00:00.000Z")).toBe("1d ago");
  });
});
