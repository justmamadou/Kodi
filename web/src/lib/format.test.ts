import { describe, expect, it } from "vitest";
import { date, fcfa } from "./format";
import { scaledSize } from "./offline/photos";

describe("formats", () => {
  it("écrit les FCFA sans décimale, avec un séparateur de milliers", () => {
    expect(fcfa(1155000)).toBe("1 155 000");
    expect(fcfa(0)).toBe("0");
    expect(fcfa(-25000)).toBe("-25 000");
  });

  it("écrit les dates en JJ/MM/AAAA", () => {
    expect(date("2026-11-03")).toBe("03/11/2026");
    expect(date("2026-11-03T10:00:00Z")).toBe("03/11/2026");
    expect(date(null)).toBe("—");
  });
});

describe("compression des photos", () => {
  it("ramène le plus grand côté à 1600 px sans déformer", () => {
    expect(scaledSize(4000, 3000)).toEqual({ width: 1600, height: 1200 });
    expect(scaledSize(3000, 4000)).toEqual({ width: 1200, height: 1600 });
  });

  it("n'agrandit jamais une petite image", () => {
    expect(scaledSize(800, 600)).toEqual({ width: 800, height: 600 });
  });
});
