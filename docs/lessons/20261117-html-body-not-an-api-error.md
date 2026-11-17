# An HTML body in a client error means the request never reached New API

Signal: a customer integrating the site as an upstream channel reports
`bad response status code 405, body: <html>`, or "获取不到模型" (can't fetch
models), while `/v1/models` looks fine in the app logs.

New API answers JSON for every error it produces. An HTML body therefore comes
from the web server, and the request never entered `/v1/`, `/api/`, `/v1beta/`,
`/mj/`, `/pg/` or `/doubao/`.

## Check

1. Reproduce against the site, not the code:
   - `curl -X POST https://<site>/index.html` → `405 text/html` with
     `<title>405 Not Allowed</title>` and an nginx footer;
   - `curl https://<site>/index.html/v1/models` → `200 text/html` (the SPA
     fallback serves `index.html`);
   - `curl -X POST https://<site>/v1/models` → `404 application/json`;
   - `curl https://<site>/v1/models` (no key) → `401 application/json`.
2. nginx access log is key=value: grep `" status=405 "`, never `" 405 "`. The
   tell is `upstream_addr=-` (no backend was contacted) and
   `http_user_agent="Go-http-client/2.0"` / `undici` for gateway-style callers.
3. The application log has no matching request id at all — do not conclude the
   app accepted and rejected it.

## Why it looks like "can't fetch models"

`location / { try_files $uri $uri/ /index.html; }` turns any unknown GET path
into `200 text/html`. A client whose base URL is a page URL (for example
`https://<site>/index.html`, or any prefix that lands on `location /`) receives
HTML where it expects `{"data":[...]}` and reports a parse/no-models failure
instead of a 404.

## Fix

Tell the caller to use the bare origin (`https://token.shanedit.com`) with no
`/index.html`, and verify with
`curl -s https://<site>/v1/models -H "Authorization: Bearer sk-..."` returning
JSON. If the caller appends `/v1/...` itself, do not include `/v1` in the base
URL.

Server-side hardening (nginx only, no rebuild): answer non-GET requests on SPA
routes with a JSON 404 instead of `405 Not Allowed`, and skip the `index.html`
fallback when the request carries `Accept: application/json`.