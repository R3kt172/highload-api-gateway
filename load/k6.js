import http from "k6/http";
import { check } from "k6";

export const options = {
  scenarios: {
    steady: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.RPS || 100),
      timeUnit: "1s",
      duration: __ENV.DURATION || "30s",
      preAllocatedVUs: 50,
      maxVUs: 500,
    },
  },
  thresholds: {
    http_req_duration: ["p(95)<100", "p(99)<250"],
  },
};

export default function () {
  const response = http.get(`${__ENV.BASE_URL || "http://localhost:8080"}/api/ping`, {
    headers: {
      Authorization: `Bearer ${__ENV.TOKEN}`,
      "X-Trace-ID": `k6-${__VU}-${__ITER}`,
    },
  });
  check(response, {
    "success or rate-limited": (r) => r.status === 200 || r.status === 429,
    "trace id returned": (r) => Boolean(r.headers["X-Trace-Id"]),
  });
}
