import { describe, expect, it } from "vitest";
import { groupBy } from "./groupBy";

describe("groupBy", () => {
  it("should return an empty object for an empty array", () => {
    expect(groupBy([], (item) => String(item))).toEqual({});
  });

  it("should group items by a simple object property", () => {
    const items = [
      { id: 1, category: "A" },
      { id: 2, category: "B" },
      { id: 3, category: "A" },
    ];
    expect(groupBy(items, (item) => item.category)).toEqual({
      A: [
        { id: 1, category: "A" },
        { id: 3, category: "A" },
      ],
      B: [{ id: 2, category: "B" }],
    });
  });

  it("should group items by a computed string", () => {
    const items = [1, 2, 3, 4, 5];
    expect(groupBy(items, (item) => (item % 2 === 0 ? "even" : "odd"))).toEqual({
      odd: [1, 3, 5],
      even: [2, 4],
    });
  });

  it("should handle elements that produce the same key", () => {
    const items = ["apple", "banana", "apricot", "blueberry"];
    expect(groupBy(items, (item) => item[0])).toEqual({
      a: ["apple", "apricot"],
      b: ["banana", "blueberry"],
    });
  });
});
