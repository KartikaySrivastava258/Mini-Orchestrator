import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, createPod, getReadiness, listPods, request } from "./client";

function mockFetch(status: number, body: unknown, raw = false) {
  const fn = vi.fn().mockResolvedValue(
    new Response(raw ? (body as string) : JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("request", () => {
  it("returns list items", async () => {
    mockFetch(200, { items: [{ name: "web" }], count: 1 });
    await expect(listPods()).resolves.toEqual([{ name: "web" }]);
  });

  it("turns the error envelope into an ApiError with code, field and request ID", async () => {
    mockFetch(400, { error: { code: "VALIDATION_ERROR", message: "image is required", field: "image" }, requestId: "r-1" });
    const err = await createPod({ name: "web", image: "", restartPolicy: "Always" }).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 400, code: "VALIDATION_ERROR", field: "image", requestId: "r-1", message: "image is required" });
  });

  it("handles non-JSON error bodies", async () => {
    mockFetch(502, "<html>bad gateway</html>", true);
    await expect(request("/x")).rejects.toMatchObject({ status: 502, code: "HTTP_502" });
  });

  it("reports network failures with an actionable message", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));
    const err = (await request("/x").catch((e: unknown) => e)) as ApiError;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.isNetworkError).toBe(true);
    expect(err.message).toMatch(/Cannot reach the API/);
  });

  it("sends JSON bodies with the right method and content type", async () => {
    const fn = mockFetch(201, { name: "web" });
    await createPod({ name: "web", image: "nginx", restartPolicy: "Never" });
    const [url, init] = fn.mock.calls[0];
    expect(url).toMatch(/\/api\/v1\/pods$/);
    expect(init.method).toBe("POST");
    expect(init.headers).toEqual({ "Content-Type": "application/json" });
    expect(JSON.parse(init.body)).toEqual({ name: "web", image: "nginx", restartPolicy: "Never" });
  });
});

describe("getReadiness", () => {
  it("treats 503 with a readiness body as a result, not an error", async () => {
    mockFetch(503, { status: "not ready", checks: { etcd: "unavailable" } });
    await expect(getReadiness()).resolves.toEqual({ status: "not ready", checks: { etcd: "unavailable" } });
  });
});
