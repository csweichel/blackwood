import { describe, it, expect } from "vitest";
import { stableSortedArray } from "./stableArray";

describe("stableSortedArray", () => {
  it("returns prev when contents are identical", () => {
    const prev = ["a", "b", "c"];
    const next = ["a", "b", "c"];
    const result = stableSortedArray(prev, next);
    expect(result).toBe(prev); // referential equality
  });

  it("returns prev when contents are identical but differently ordered", () => {
    const prev = ["a", "b", "c"];
    const next = ["c", "a", "b"];
    const result = stableSortedArray(prev, next);
    expect(result).toBe(prev); // referential equality — same elements
  });

  it("returns new sorted array when contents differ", () => {
    const prev = ["a", "b"];
    const next = ["a", "b", "c"];
    const result = stableSortedArray(prev, next);
    expect(result).not.toBe(prev);
    expect(result).toEqual(["a", "b", "c"]);
  });

  it("returns new sorted array when an element is removed", () => {
    const prev = ["a", "b", "c"];
    const next = ["a", "c"];
    const result = stableSortedArray(prev, next);
    expect(result).not.toBe(prev);
    expect(result).toEqual(["a", "c"]);
  });

  it("returns new sorted array when elements are replaced", () => {
    const prev = ["a", "b"];
    const next = ["x", "y"];
    const result = stableSortedArray(prev, next);
    expect(result).not.toBe(prev);
    expect(result).toEqual(["x", "y"]);
  });

  it("handles empty arrays", () => {
    const prev: string[] = [];
    const next: string[] = [];
    const result = stableSortedArray(prev, next);
    expect(result).toBe(prev);
  });

  it("handles transition from empty to non-empty", () => {
    const prev: string[] = [];
    const next = ["a"];
    const result = stableSortedArray(prev, next);
    expect(result).not.toBe(prev);
    expect(result).toEqual(["a"]);
  });

  it("returns sorted output regardless of input order", () => {
    const prev = ["z", "a"];
    const next = ["m", "b", "z"];
    const result = stableSortedArray(prev, next);
    expect(result).toEqual(["b", "m", "z"]);
  });
});
