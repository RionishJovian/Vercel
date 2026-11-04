import { describe, expect, it } from "vitest";
import { ApiError } from "./api";
import { errorText } from "./errors";

describe("errorText", () => {
  it("shows what the API wrote for people", () => {
    expect(errorText(new ApiError("that name is taken", 409, { error: "that name is taken" }), "failed")).toBe("that name is taken");
    expect(errorText(new ApiError("http-409", 409, { reason: "router is offline", error: "conflict" }), "failed")).toBe("router is offline");
  });
  it("never shows a code error", () => {
    const crash = new TypeError("Cannot read properties of undefined (reading 'pending_confirm')");
    expect(errorText(crash, "failed")).toBe("failed");
    expect(errorText(new Error("router-read-timeout"), "failed")).toBe("failed");
    expect(errorText("boom", "failed")).toBe("failed");
    expect(errorText(null, "failed")).toBe("failed");
  });
  it("replaces the client's own technical codes", () => {
    expect(errorText(new ApiError("http-500", 500), "failed")).toBe("failed");
    expect(errorText(new ApiError("network-unreachable"), "failed")).toBe("failed");
    expect(errorText(new ApiError("unauthorized", 401), "failed")).toBe("failed");
  });
  it("ignores error fields that are not text", () => {
    expect(errorText(new ApiError("http-400", 400, { error: { code: 3 } }), "failed")).toBe("failed");
  });
});
