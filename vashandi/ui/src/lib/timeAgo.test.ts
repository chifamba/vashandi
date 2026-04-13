import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { timeAgo } from "./timeAgo";

const MINUTE = 60 * 1000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;
const MONTH = 30 * DAY;

describe("timeAgo", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    // Set a fixed point in time to run the tests against
    vi.setSystemTime(new Date("2024-01-01T12:00:00.000Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("should return 'just now' for times less than a minute ago", () => {
    const now = Date.now();
    expect(timeAgo(new Date(now - 10 * 1000))).toBe("just now");
    expect(timeAgo(new Date(now - 59 * 1000))).toBe("just now");
  });

  it("should return minutes ago for times less than an hour ago", () => {
    const now = Date.now();
    expect(timeAgo(new Date(now - 1 * MINUTE))).toBe("1m ago");
    expect(timeAgo(new Date(now - 45 * MINUTE))).toBe("45m ago");
    expect(timeAgo(new Date(now - 59 * MINUTE))).toBe("59m ago");
  });

  it("should return hours ago for times less than a day ago", () => {
    const now = Date.now();
    expect(timeAgo(new Date(now - 1 * HOUR))).toBe("1h ago");
    expect(timeAgo(new Date(now - 12 * HOUR))).toBe("12h ago");
    expect(timeAgo(new Date(now - 23 * HOUR))).toBe("23h ago");
  });

  it("should return days ago for times less than a week ago", () => {
    const now = Date.now();
    expect(timeAgo(new Date(now - 1 * DAY))).toBe("1d ago");
    expect(timeAgo(new Date(now - 3 * DAY))).toBe("3d ago");
    expect(timeAgo(new Date(now - 6 * DAY))).toBe("6d ago");
  });

  it("should return weeks ago for times less than a month ago", () => {
    const now = Date.now();
    expect(timeAgo(new Date(now - 1 * WEEK))).toBe("1w ago");
    expect(timeAgo(new Date(now - 2 * WEEK))).toBe("2w ago");
    expect(timeAgo(new Date(now - 4 * WEEK))).toBe("4w ago"); // 28 days
  });

  it("should return months ago for times a month or more ago", () => {
    const now = Date.now();
    expect(timeAgo(new Date(now - 1 * MONTH))).toBe("1mo ago");
    expect(timeAgo(new Date(now - 6 * MONTH))).toBe("6mo ago");
    expect(timeAgo(new Date(now - 12 * MONTH))).toBe("12mo ago");
  });

  it("should handle string inputs correctly", () => {
    const now = Date.now();
    const threeDaysAgo = new Date(now - 3 * DAY).toISOString();
    expect(timeAgo(threeDaysAgo)).toBe("3d ago");
  });
});
