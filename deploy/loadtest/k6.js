// Test de charge ciblé (CF-032) sur les écrans les plus sollicités : tableau de bord, file de
// validation, liste des dépenses. Seuils alignés sur l'exigence P95 < 700 ms du cahier des charges.
//
//   k6 run -e BASE_URL=http://localhost:8080 -e LOGIN=... -e PASSWORD=... deploy/loadtest/k6.js
import http from "k6/http";
import { check, group } from "k6";

const BASE = __ENV.BASE_URL || "http://localhost:8080";

export const options = {
  scenarios: {
    lecture: { executor: "ramping-vus", stages: [
      { duration: "10s", target: 20 }, { duration: "30s", target: 20 }, { duration: "5s", target: 0 },
    ] },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    "http_req_duration{name:dashboard}": ["p(95)<700"],
    "http_req_duration{name:inbox}": ["p(95)<700"],
    "http_req_duration{name:expenses}": ["p(95)<700"],
  },
};

export function setup() {
  const res = http.post(`${BASE}/api/v1/auth/login`, JSON.stringify({ login: __ENV.LOGIN, password: __ENV.PASSWORD }), {
    headers: { "Content-Type": "application/json" },
  });
  check(res, { "connexion réussie": (r) => r.status === 200 });
  return { session: res.cookies.cf_session[0].value };
}

export default function (data) {
  const params = (name) => ({ tags: { name }, cookies: { cf_session: data.session } });
  group("écrans", () => {
    check(http.get(`${BASE}/api/v1/dashboard`, params("dashboard")), { "dashboard 200": (r) => r.status === 200 });
    check(http.get(`${BASE}/api/v1/expenses?status=pending&limit=50`, params("inbox")), { "inbox 200": (r) => r.status === 200 });
    check(http.get(`${BASE}/api/v1/expenses?limit=50`, params("expenses")), { "dépenses 200": (r) => r.status === 200 });
  });
}
