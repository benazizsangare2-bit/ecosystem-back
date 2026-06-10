# Backend Implementation Review

**Project:** Ecosystem / EnvTrack — Go + Gin + PostgreSQL + Docker  
**Purpose:** Confirm scope, assumptions, and open questions before implementation.  
**Status:** Awaiting your approval — **do not implement until you reply “proceed” (or equivalent).**

---

## 1. My Understanding of the User Story

| Step | Flow                                              | Backend responsibility                                               |
| ---- | ------------------------------------------------- | -------------------------------------------------------------------- |
| 1    | User registers with email (no phone verification) | `POST /api/register` — hash password, send Mailjet verification link |
| 2    | User verifies email via link                      | Verify token → set `is_email_verified = true`                        |
| 3    | User logs in                                      | `POST /api/login` → JWT (Bearer)                                     |
| 4    | User submits report                               | Multipart upload: photo, description, lat/lng, category → `pending`  |
| 5    | Duplicate check                                   | Three approaches (see §6) — configurable or layered                  |
| 6    | Report persisted                                  | DB + local `./uploads/`                                              |
| 7    | Admin reviews                                     | Approve / reject / duplicate / resolve + notes                       |
| 8    | User sees own reports + status                    | `GET /api/reports/user`                                              |
| 9    | Public feed + social                              | Approved reports, likes, comments, visible status                    |

**Additional (your message):** Forgot-password + reset-password + change-password while logged in.

---

## 2. Current Codebase State (as inspected)

### Already present

| Area                     | Status         | Notes                                                                        |
| ------------------------ | -------------- | ---------------------------------------------------------------------------- |
| `handlers/auth.go`       | Partial        | Register, VerifyEmail (POST + DB token), Login, JWT helpers, Mailjet send    |
| `database/connection.go` | OK             | Docker `DB_HOST=postgres` vs local — keep as-is per your instruction         |
| `database/schema.go`     | Partial        | Users, reports, upvotes, comments, notifications, refresh_tokens, audit_logs |
| `models/models.go`       | Partial        | Auth request/response types only                                             |
| `main.go`                | Needs refactor | Routes + placeholder `authMiddleware()` still here                           |
| `routes/routes.go`       | Empty          | All routes should move here                                                  |
| Docker                   | Basic          | `postgres` + `backend`; **no `./uploads` volume yet**                        |
| `handlers/profile.go`    | Stub           | Placeholder JSON                                                             |

### Missing (expected to create)

- `middleware/auth.go` — real JWT validation, role checks
- `handlers/report.go`, `handlers/admin.go`
- `handlers/` helpers: standardized responses, upload utilities
- Password reset / change-password handlers
- Report CRUD, admin, public, social endpoints
- Static file serving for uploads
- `.gitignore` entry for `uploads/` (recommended)

### Issues to fix during implementation

1. **`schema.go` line 38** — SQL typo: `''illegal_dumping'` (broken `CHECK` constraint) — migrations will fail until fixed. // I manually fixed this SQL typo, proceed with other task.
2. **Email verification** — Today: random hex token stored in `email_verification_token`. You asked for **JWT-only (no DB storage)** and link `http://localhost:3000/verify?token={jwt}`. Readme says `GET /api/verify-email?token=`. Current handler is **POST** with `{email, token}` body.
3. **Verify link path** — Code uses `/verify-email?email=...&token=...`; you specified `/verify?token=...` only.
4. **Register fields** — Code: `first_name`, `last_name`, `phone_number` (optional). Readme snippet: `{email, password, name}` — need alignment. // use first_name, last_name (optional), phone_number (required), email (required)
5. **Reports `title`** — Schema requires `title`; user story emphasizes description + photo — clarify if title is required or auto-generated. // title can be auto-generated, then proceed with description + photo etc
6. **`admin_notes`** — Referenced in readme; **not in `reports` table** — add column or use audit_logs only? // add column admin_notes
7. **API response format** — Handlers return ad-hoc `gin.H{"error": ...}`; target: `{success, data, message}` / `{success, false, error, code}`.
8. **`main.go` order** — `InitDatabase()` runs after some routes are registered (works but messy); routes package should register after DB init. // implement the right way

---

## 3. Architecture & File Layout (agreed structure)

```
environment/
├── main.go                 # bootstrap only: env, DB, CORS, routes.Setup(), Run
├── routes/routes.go        # ALL HTTP routes
├── middleware/auth.go      # JWT + optional AdminMiddleware
├── handlers/
│   ├── auth.go             # register, verify, login, logout, forgot/reset/change password
│   ├── report.go           # CRUD, public, upload
│   ├── admin.go            # admin reports, status, stats
│   └── profile.go          # profile (extend as needed)
├── models/models.go        # all structs + request/response DTOs
├── database/
│   ├── connection.go       # unchanged behavior
│   └── schema.go           # migrations + any new columns
├── uploads/                # created at runtime; Docker volume ./uploads:/app/uploads
└── utils/ (optional)       # response helpers, image validation — only if keeps handlers clean
```

---

## 4. Proposed Implementation Phases

### Phase 1 — Auth (complete + align with your spec)

- [ ] Standardize API response helpers
- [ ] **Register:** validate email uniqueness; bcrypt password; **JWT verification token (24h, claim: `user_id`, `purpose: email_verify`)** — remove DB token storage _or_ keep both during transition (your call in §8)
- [ ] **Verify email:** `GET /api/verify-email?token=` (and/or support frontend `POST` if needed)
- [ ] Mailjet link: `http://localhost:3000/verify?token={jwt}` (confirm frontend route)
- [ ] **Login:** require `is_email_verified`; return JWT + user object in standard envelope
- [ ] **Logout (bonus):** blacklist `jti` in DB table or `refresh_tokens` / new `token_blacklist` — clarify storage (§8)
- [ ] **Forgot password:** `POST /api/forgot-password` → JWT 1h (`purpose: password_reset`)
- [ ] **Reset password:** `POST /api/reset-password` `{token, new_password}`
- [ ] **Change password:** `POST /api/change-password` (auth) `{old_password, new_password}`

### Phase 2 — Auth middleware

- [ ] Create `middleware/auth.go`
- [ ] Parse `Authorization: Bearer <token>`, validate signature + expiry
- [ ] Set context: `user_id`, `email`, `role`
- [ ] Reject unverified users on protected routes? (only login blocked today — confirm for reports)
- [ ] `AdminMiddleware()` — `role == 'admin'` (and/or `'authority'`?)
- [ ] Remove placeholder from `main.go`

### Phase 3 — Report CRUD + uploads

- [ ] `POST /api/reports` — multipart: photo, description, lat, lng, category (+ title if required)
- [ ] Save to `./uploads/{uuid}.{ext}`; store path in `photo_urls` (array with one or more)
- [ ] Validation: max 5MB; `.jpg`, `.jpeg`, `.png`; min 200×200; sanitize filename → UUID only
- [ ] Optional: thumbnail generation (e.g. `uploads/thumbs/`)
- [ ] Static route: e.g. `GET /uploads/*filepath` or `/api/static/...`
- [ ] `GET /api/reports/user`, `GET /api/reports/:id`, `PUT /api/reports/:id`, `DELETE /api/reports/:id` — only if `status = 'pending'`
- [ ] Docker: volume `./uploads:/app/uploads` in `docker-compose.yml`

### Phase 4 — Duplicate prevention (three approaches)

| Approach                   | Behavior                                                                                                   | Implementation sketch                                               |
| -------------------------- | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| **1 — Manual admin**       | All reports `pending`; admin sets `status = 'duplicate'`, `duplicate_of = <id>`; notify reporter           | Default; notifications row + optional email                         |
| **2 — DB proximity check** | On create: query reports within lat/lng box (±ε), last 7 days, `status != 'rejected'`, different `user_id` | Flag as `potential_duplicate` or block/warn — **behavior TBD (§8)** |
| **3 — ?**                  | **Not described in your message**                                                                          | See §8 — propose hybrid or third mode                               |

**Proposed default wiring (pending your approval):**

- Always run Approach 2 as **non-blocking warning** in create response (`possible_duplicates: [...]`) unless you want hard reject.
- Approach 1 remains source of truth for final `duplicate` status.
- Approach 3: **config flag** `DUPLICATE_MODE=manual|auto_flag|auto_reject` — implement all three behaviors behind env — _only if you confirm_. // Confirmed

### Phase 5 — Admin

- [ ] `GET /api/admin/reports` — filters: status, category, date range, pagination
- [ ] `PUT /api/admin/reports/:id/status` — `{status, admin_notes}`; write `audit_logs`
- [ ] `GET /api/admin/stats` — counts by status, category, recent activity
- [ ] Valid statuses: `pending`, `under_review`, `investigating`, `resolved`, `rejected`, `duplicate` (per schema)

### Phase 6 — Social + reputation

- [ ] `GET /api/reports/public?page&limit` — only **approved** reports (define: `resolved` only or also `under_review`? — §8)
- [ ] `POST /api/reports/:id/like` — toggle upvote (`report_upvotes`)
- [ ] `POST /api/reports/:id/comments`, `GET /api/reports/:id/comments`
- [ ] Reputation: increment on approved report / upvotes received (schema has `reputation_score`, `total_upvotes_received`)

---

## 5. Endpoint Checklist (from readme + your additions)

| Method | Path                            | Auth              | Phase | Notes             |
| ------ | ------------------------------- | ----------------- | ----- | ----------------- |
| POST   | `/api/register`                 | Public            | 1     | Align body fields |
| GET    | `/api/verify-email`             | Public            | 1     | Query `token`     |
| POST   | `/api/login`                    | Public            | 1     |                   |
| POST   | `/api/logout`                   | Bearer            | 1     | Bonus blacklist   |
| POST   | `/api/forgot-password`          | Public            | 1     |                   |
| POST   | `/api/reset-password`           | Public            | 1     |                   |
| POST   | `/api/change-password`          | Bearer            | 1     |                   |
| POST   | `/api/reports`                  | Bearer            | 3     | Multipart         |
| GET    | `/api/reports/user`             | Bearer            | 3     |                   |
| GET    | `/api/reports/:id`              | Bearer? / Public? | 3     | §8 — own vs any   |
| PUT    | `/api/reports/:id`              | Bearer            | 3     | pending only      |
| DELETE | `/api/reports/:id`              | Bearer            | 3     | pending only      |
| GET    | `/api/admin/reports`            | Admin             | 5     |                   |
| PUT    | `/api/admin/reports/:id/status` | Admin             | 5     |                   |
| GET    | `/api/admin/stats`              | Admin             | 5     |                   |
| GET    | `/api/reports/public`           | Public            | 6     | paginated         |
| POST   | `/api/reports/:id/like`         | Bearer            | 6     |                   |
| POST   | `/api/reports/:id/comments`     | Bearer            | 6     |                   |
| GET    | `/api/reports/:id/comments`     | Public            | 6     |                   |
| GET    | `/api/profile`                  | Bearer            | 2     | Implement fully   |
| GET    | `/api/health`                   | Public            | —     | Keep              |
| GET    | `/uploads/*`                    | Public            | 3     | Static files      |

---

## 6. Technical Decisions (planned unless you override)

| Topic                  | Plan                                                                       |
| ---------------------- | -------------------------------------------------------------------------- |
| JWT secret             | `JWT_SECRET` from env (already in `.env`)                                  |
| Login JWT expiry       | `JWT_EXPIRY_HOURS` (default 24h)                                           |
| Email verify JWT       | 24h, claim `purpose: "email_verify"`                                       |
| Password reset JWT     | 1h, claim `purpose: "password_reset"`                                      |
| Password hashing       | bcrypt (existing)                                                          |
| Photo storage          | Local `./uploads/`, DB stores relative path                                |
| CORS                   | Keep permissive `*` for dev (existing)                                     |
| Phone verification     | **Not implemented** (per user story) — `phone_number` optional on register |
| DB migrations          | Continue `InitDatabase()` on startup (existing pattern)                    |
| Duplicate notification | Insert into `notifications` table; email optional                          |

---

## 7. Docker & Environment (no change to connection logic)

- Local dev: `.env` with `DB_HOST` empty → `localhost` (your `connection.go` behavior).
- Docker Compose: override `DB_HOST=postgres` (already in `docker-compose.yml`).
- **Add:** volume mount for uploads; pass Mailjet + JWT vars into backend service (currently only DB vars in compose — Mailjet may be missing in container unless copied via `.env` in Dockerfile). // do not add mailjet to container, just read from `.env`

---

## 8. Open Questions — Please Comment Below Each Item

### Q1 — Email verification mechanism

**Your recommendation:** JWT only, no DB column.
**Current code:** Random token in DB.  
**Proposed:** Switch to JWT; drop writes to `email_verification_token` / `expiry` (columns can remain unused).  
**Your comment:**

```
[ yes] Approve JWT-only
[ ] Keep DB token instead
[ ] Other: _______________________________
```

### Q2 — Verify endpoint contract

- `GET /api/verify-email?token=` only, or also `POST` for SPA?
- Frontend URL: `/verify` vs `/verify-email`?  
  **Your comment:**

```
implement the better way
```

### Q3 — Register payload

Use `first_name` + `last_name`, single `name`, or both? Is `phone_number` required?  
**Your comment:**

```
use as single name. and phone_number is required but match the format of DRC phone number
```

### Q4 — Report `title` field

Required from client, optional, or auto-truncate from description (first N chars)?  
**Your comment:**

```
auto-truncate from description and possibility to edit
```

### Q5 — “Approved” public reports

Which statuses appear on main page? e.g. only `resolved`, or `under_review` + `investigating` + `resolved`?  
**Your comment:**

```
investigating
```

### Q6 — Duplicate Approach 3

You asked for **three** approaches; only 1 and 2 were described. Options:

- A) Hybrid: auto-flag + mandatory admin confirmation
- B) Auto-reject on proximity match
- C) Image hash similarity (future)  
  **Your comment:**

```
leave the future one. (complicated)
```

### Q7 — Approach 2 on create — block or warn?

When proximity match found: reject create, allow with `pending` + warning, or set `under_review` automatically?  
**Your comment:**

```
warn and let the admin know it was marked as warned
```

### Q8 — Logout / token blacklist

Store revoked `jti` in new table, reuse `refresh_tokens`, or skip logout (client-only delete)?  
**Your comment:**

```
use the best way to keep security at max.
```

### Q9 — `admin_notes` storage

Add `admin_notes TEXT` on `reports`, or only in `audit_logs`?  
**Your comment:**

```
add on reports.
```

### Q10 — Roles

Is `authority` role same as admin for API access, or admin-only for `/api/admin/*`?  
**Your comment:**

```
yes admin and authority is the same.
```

### Q11 — GET `/api/reports/:id` visibility

Can any authenticated user view any report, or only owner + admin + public if approved?  
**Your comment:**

```
authenticated user view only their reports in their accounts. But in the main page (social pages) everyone views all resolved reports.
```

### Q12 — Reputation rules

Exact formula? e.g. +10 approved report, +1 per upvote, -5 duplicate?  
**Your comment:**

```
yes correct.
```

### Q13 — Thumbnails

Implement in Phase 3 (recommended) or defer?  
**Your comment:**

```
approved
```

### Q14 — Latitude/longitude duplicate epsilon

Default ~50m? (e.g. ±0.0005 degrees) — provide preferred radius.  
**Your comment:**

```
use the best one based on DRC (democratic republic of congo) data.
```

---

## 9. Out of Scope (unless you say otherwise)

- Phone/SMS verification
- Frontend implementation (links point to `localhost:3000`)
- S3/cloud storage
- Automated image-ML duplicate detection
- Rate limiting / CAPTCHA
- Full OpenAPI/Swagger docs // implement that.

---

## 10. Risks & Dependencies

| Risk                               | Mitigation                                     |
| ---------------------------------- | ---------------------------------------------- | ---------------------------------------------- |
| Mailjet not configured in Docker   | Pass env vars in `docker-compose.yml`          | // read mailjet info from .env when on docker. |
| Schema typo blocks startup         | Fix `category` CHECK first                     |
| Uploads lost on container recreate | Bind mount `./uploads`                         |
| Secrets in `.env` committed        | Ensure `.gitignore`; do not commit credentials |

---

## 11. Approval

When you are satisfied with this document:

1. Fill in comments in §8 (or reply inline in chat).
2. Reply with **“proceed”** (and any phase limits, e.g. “Phases 1–3 only first”).

I will then implement in phase order, run `go build`, fix schema, update Docker compose, and align readme endpoints with actual behavior.

---

**Prepared by:** Cursor Agent (review only — no code changes made for this task).  
**Date:** 2026-05-30
