import { describe, it, expect } from "vitest";
import { pluralRu } from "./plural";

const forms: [string, string, string] = ["задание", "задания", "заданий"];

describe("pluralRu", () => {
  it.each([
    [1, "задание"], [21, "задание"], [101, "задание"],
    [2, "задания"], [3, "задания"], [4, "задания"], [22, "задания"],
    [0, "заданий"], [5, "заданий"], [11, "заданий"], [12, "заданий"],
    [14, "заданий"], [19, "заданий"], [100, "заданий"], [111, "заданий"],
  ])("%d → %s", (n, want) => {
    expect(pluralRu(n as number, forms)).toBe(want);
  });
});
