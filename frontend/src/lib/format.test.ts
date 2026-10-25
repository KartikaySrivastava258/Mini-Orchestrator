import { describe, expect, it } from "vitest";
import type { Node, Pod } from "./api/types";
import { age, countPhases, needsAttention, nodeHealth, parseTime, relativeTime } from "./format";
import { validateImage, validateName, validateReplicas } from "./validation";

const now = Date.parse("2026-03-01T12:00:00Z");

describe("time formatting", () => {
  it("treats Go's zero time as missing", () => {
    expect(parseTime("0001-01-01T00:00:00Z")).toBeNull();
    expect(relativeTime("0001-01-01T00:00:00Z", now)).toBe("—");
  });
  it("formats relative times", () => {
    expect(relativeTime("2026-03-01T11:59:30Z", now)).toBe("30s ago");
    expect(relativeTime("2026-03-01T11:55:00Z", now)).toBe("5m ago");
    expect(age("2026-02-27T12:00:00Z", now)).toBe("2d");
  });
});

const node = (ready: boolean, reason: string, heartbeat: string): Node => ({
  name: "w1",
  uid: "u",
  registeredAt: heartbeat,
  spec: { unschedulable: false },
  status: { ready, reason, address: "", maxPods: 10, runtimeReady: ready, lastHeartbeatTime: heartbeat, lastTransitionTime: heartbeat },
});

describe("nodeHealth", () => {
  it("distinguishes ready, stale, runtime-unavailable and not-ready", () => {
    expect(nodeHealth(node(true, "Ready", "2026-03-01T11:59:55Z"), 15, now)).toBe("Ready");
    expect(nodeHealth(node(true, "Ready", "2026-03-01T11:59:00Z"), 15, now)).toBe("StaleHeartbeat");
    expect(nodeHealth(node(false, "RuntimeUnavailable", "2026-03-01T11:59:55Z"), 15, now)).toBe("RuntimeUnavailable");
    expect(nodeHealth(node(false, "HeartbeatTimeout", "2026-03-01T11:00:00Z"), 15, now)).toBe("NotReady");
  });
});

const pod = (phase: Pod["status"]["phase"], reason?: string): Pod => ({
  name: "p",
  uid: "u",
  createdAt: "2026-03-01T11:00:00Z",
  spec: { image: "nginx", restartPolicy: "Always" },
  status: { phase, reason, restartCount: 0, lastTransitionTime: "", updatedAt: "" },
});

describe("pod summaries", () => {
  it("counts phases from real data only", () => {
    const c = countPhases([pod("Running"), pod("Running"), pod("Failed")]);
    expect(c.Running).toBe(2);
    expect(c.Failed).toBe(1);
    expect(c.Pending).toBe(0);
  });
  it("flags pods that need attention", () => {
    expect(needsAttention(pod("Failed"))).toBe(true);
    expect(needsAttention(pod("Pending", "AwaitingScheduling"))).toBe(false);
    expect(needsAttention(pod("Pending", "NoEligibleNodes"))).toBe(true);
    expect(needsAttention(pod("Running", "RestartBackOff"))).toBe(true);
    expect(needsAttention(pod("Running", "ContainerStarted"))).toBe(false);
  });
});

describe("validation mirrors the API", () => {
  it("validates names", () => {
    expect(validateName("web-1", 63)).toBeNull();
    expect(validateName("Web", 63)).not.toBeNull();
    expect(validateName("a".repeat(64), 63)).not.toBeNull();
  });
  it("rejects flag-like images", () => {
    expect(validateImage("nginx:stable")).toBeNull();
    expect(validateImage("--privileged")).not.toBeNull();
  });
  it("validates replica counts", () => {
    expect(validateReplicas("3")).toBeNull();
    expect(validateReplicas("-1")).not.toBeNull();
    expect(validateReplicas("2.5")).not.toBeNull();
    expect(validateReplicas("")).not.toBeNull();
  });
});
