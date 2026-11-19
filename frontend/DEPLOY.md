# Deploying on Render

Two static sites, one per app, built from this folder. This guide assumes the repository has `backend/` and `frontend/` side by side at its root, and that the API is a Render web service.

## 1. Names

| Site            | Recommended name     | URL                              |
| --------------- | -------------------- | -------------------------------- |
| Main app        | `endosoflow-app`     | `endosoflow-app.onrender.com`    |
| Customer portal | `endosoflow-portal`  | `endosoflow-portal.onrender.com` |
| API (existing)  | whatever it is today |                                  |

- **`endosoflow-portal`**: keep it. It says what it is, it is what the code and the data model already call it (`portalStatus`), and customers recognise the word.
- **`endosoflow-workspace` → `endosoflow-staff`**: "workspace" collides with _npm workspaces_, which this repository now uses, so `workspace` would end up meaning two things in commands and docs. "Staff" also names the _audience_, which matches "portal" (customers), and it is accurate: `admin` would be wrong because advisors and managers use the site too, and `system_admin` is one role among three.
- The code is already named `apps/web` and `@endosoflow/web`. Render's service name is independent of the folder name, so using another one later costs nothing in the code.
- **Availability:** `onrender.com` subdomains are shared by every Render customer, and a name already taken gets a random suffix. Create the services and read the URL Render gives you; if you do not get the exact one, it is not a problem (see 2).
- **Long term, use your own domain** (for example `app.<yourdomain>` and `portal.<yourdomain>`). Customers will type or bookmark this address and see it in emails, and a Render subdomain says nothing about Grupo Pana. Render supports custom domains on static sites.

Neither name is a secret and neither protects anything: login does. Both sites ship `noindex` (a meta tag and `robots.txt`) so search engines do not list them.

## 2. The decision that affects login: how the browser reaches the API

The API signs people in with a **refresh cookie** (`credentials: "include"`). Browsers only send that cookie freely when the site and the API are the _same site_.

`onrender.com` is on the Public Suffix List, so `endosoflow-app.onrender.com`, `endosoflow-portal.onrender.com` and `endosoflow-api.onrender.com` are **three different sites** to a browser. A cookie set by the API is then a third-party cookie: modern browsers block it, and login appears to work until the page is refreshed. Render's own support gives two ways out:

**A. A rewrite rule on each static site (works on `onrender.com`, recommended now).** The site proxies `/api/*` to the API, so the browser only ever talks to its own address and the cookie is first-party. The code is already written for this: `VITE_API_BASE_URL` defaults to `/api/v1`, the same relative path the dev proxy uses.

```
Source:      /api/*
Destination: https://<your-api-service>.onrender.com/api/*
Action:      Rewrite
```

The destination must be the API's **public** URL (the internal address is not accepted by static-site rules). Put this rule **above** the `/*` → `/index.html` rule, because rules apply in order.

**B. Your own domain for all three** (`app.`, `portal.`, `api.` under one registrable domain). Then they are the same site. This also needs CORS on the API with the two explicit origins and `credentials: true`, and the cookie without a `Domain` attribute problem. More to configure than A, so choose it only if you want the API on its own public address anyway.

With A, each site has its own cookie on its own host: **a staff session never exists on the customers' site and vice versa.**

Backend checklist for the refresh cookie behind a rewrite: `HttpOnly`, `Secure`, `SameSite=Lax` (or `Strict`), **no `Domain` attribute** (host-only), `Path` covering `/api/v1/auth`.

## 3. Settings for each site

Create each as **New → Static Site** from the repository. Leave **Root Directory empty**: with a root directory set, Render only redeploys when files under it change, and the shared packages live outside each app's folder. Use build filters instead.

| Field                | Main App                                                                                              | Portal                                                                                                   |
| -------------------- | ----------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Name                 | `endosoflow-app`                                                                                      | `endosoflow-portal`                                                                                      |
| Build Command        | `cd frontend && npm ci && npm run build:web`                                                          | `cd frontend && npm ci && npm run build:portal`                                                          |
| Publish Directory    | `frontend/apps/web/dist`                                                                              | `frontend/apps/portal/dist`                                                                              |
| Environment          | `NODE_VERSION=20`                                                                                     | `NODE_VERSION=20`                                                                                        |
| Build filter (paths) | `frontend/apps/web/**`, `frontend/packages/**`, `frontend/package-lock.json`, `frontend/package.json` | `frontend/apps/portal/**`, `frontend/packages/**`, `frontend/package-lock.json`, `frontend/package.json` |

**Redirects/Rewrites** (app and portal, in this order):

| #   | Source   | Destination                                     | Action  |
| --- | -------- | ----------------------------------------------- | ------- |
| 1   | `/api/*` | `https://<your-api-service>.onrender.com/api/*` | Rewrite |
| 2   | `/*`     | `/index.html`                                   | Rewrite |

Rule 2 is what makes a refresh on `/sales/123` work: the sites use client-side routing, so every address that is not a file must serve `index.html`.

Environment variables on a static site exist only while building. Neither site needs one: `VITE_API_BASE_URL` already defaults to `/api/v1`. Set it only if you choose option B (`https://api.<yourdomain>/api/v1`).

## 4. The same thing as a Blueprint

Merge this into the `render.yaml` at the **repository root** (it must live there; add the API service next to it as you already have it). Rewrite destinations cannot reference other services, so the API URL is written out.

```yaml
services:
  - type: web
    name: endosoflow-app
    runtime: static
    buildCommand: cd frontend && npm ci && npm run build:web
    staticPublishPath: ./frontend/apps/web/dist
    envVars:
      - key: NODE_VERSION
        value: "20"
    buildFilter:
      paths:
        - frontend/apps/web/**
        - frontend/packages/**
        - frontend/package.json
        - frontend/package-lock.json
    routes:
      - type: rewrite
        source: /api/*
        destination: https://<your-api-service>.onrender.com/api/*
      - type: rewrite
        source: /*
        destination: /index.html
    headers:
      - path: /*
        name: X-Frame-Options
        value: DENY
      - path: /*
        name: X-Content-Type-Options
        value: nosniff
      - path: /*
        name: Referrer-Policy
        value: strict-origin-when-cross-origin
      - path: /*
        name: X-Robots-Tag
        value: noindex, nofollow
      - path: /assets/*
        name: Cache-Control
        value: public, max-age=31536000, immutable

  - type: web
    name: endosoflow-portal
    runtime: static
    buildCommand: cd frontend && npm ci && npm run build:portal
    staticPublishPath: ./frontend/apps/portal/dist
    envVars:
      - key: NODE_VERSION
        value: "20"
    buildFilter:
      paths:
        - frontend/apps/portal/**
        - frontend/packages/**
        - frontend/package.json
        - frontend/package-lock.json
    routes:
      - type: rewrite
        source: /api/*
        destination: https://<your-api-service>.onrender.com/api/*
      - type: rewrite
        source: /*
        destination: /index.html
    headers:
      - path: /*
        name: X-Frame-Options
        value: DENY
      - path: /*
        name: X-Content-Type-Options
        value: nosniff
      - path: /*
        name: Referrer-Policy
        value: strict-origin-when-cross-origin
      - path: /*
        name: X-Robots-Tag
        value: noindex, nofollow
      - path: /assets/*
        name: Cache-Control
        value: public, max-age=31536000, immutable
```

## 5. After the first deploy, check

1. Open a deep link directly (`/sales`, or any address on the portal) and refresh: it must not 404.
2. Sign in on the main app, then **refresh the page**: you must stay signed in. If you are sent back to the login, the refresh cookie is being blocked: re-read section 2.
3. `/robots.txt` answers with `Disallow: /`.
4. Push a change that touches only `apps/portal`: only the portal redeploys. Change something in `packages/ui`: both do.

## 6. Local development

`npm run dev:web` (5173) and `npm run dev:portal` (5174) proxy `/api` and `/media` to the Go server (`VITE_API_PROXY_TARGET`, default `http://localhost:8080`), which is the same shape the rewrite gives in production: the browser talks to its own origin.
