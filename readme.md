## steps to use docker on this project

# first install docker compose:

sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose

# Make it executable

sudo chmod +x /usr/local/bin/docker-compose

# Verify installation

docker-compose --version

# create docker, docker-compose.yml files and .dockerignore files

###### Daily codes for work

# Connect to PostgreSQL inside Docker:
docker exec -it ecosystem-postgres psql -U postgres -d ecosystem

# START working (run in background):

docker-compose up -d

# STOP working:

docker-compose down

# REBUILD after code changes:

docker-compose up -d --build

# VIEW LOGS (live):

docker-compose logs -f

# Restart backend service:

docker-compose restart backend

//////////

## API Endpoints

All success responses: `{ "success": true, "data": {...}, "message": "..." }`  
All error responses: `{ "success": false, "error": "message", "code": 400 }`

**API documentation (Swagger UI):** [http://localhost:3030/api/docs](http://localhost:3030/api/docs)  
Raw OpenAPI YAML: `GET /api/docs/openapi.yaml`

### Auth

| Method | Path                       | Body / Params                                                  |
| ------ | -------------------------- | -------------------------------------------------------------- |
| POST   | `/api/register`            | `{ email, name, phone_number, password }` — DRC phone required |
| GET    | `/api/verify-email?token=` | JWT verification token                                         |
| POST   | `/api/verify-email`        | `{ token }`                                                    |
| POST   | `/api/login`               | `{ email, password }` → JWT + user                             |
| POST   | `/api/logout`              | Bearer token (blacklists JWT)                                  |
| POST   | `/api/forgot-password`     | `{ email }`                                                    |
| POST   | `/api/reset-password`      | `{ token, new_password }`                                      |
| POST   | `/api/change-password`     | Bearer — `{ old_password, new_password }`                      |

### Reports (authenticated)

| Method | Path                | Notes                                                                                      |
| ------ | ------------------- | ------------------------------------------------------------------------------------------ |
| POST   | `/api/reports`      | Multipart: `photo`, `description`, `latitude`, `longitude`, `category`, optional `address` |
| GET    | `/api/reports/user` | Own reports only                                                                           |
| GET    | `/api/reports/:id`  | Own reports only                                                                           |
| PUT    | `/api/reports/:id`  | Pending only; optional `title`, `description`, etc.                                        |
| DELETE | `/api/reports/:id`  | Pending only                                                                               |

### Public / social

| Method | Path                             | Notes                       |
| ------ | -------------------------------- | --------------------------- |
| GET    | `/api/reports/public?page&limit` | Status `investigating` only |
| POST   | `/api/reports/:id/like`          | Toggle like (auth)          |
| POST   | `/api/reports/:id/comments`      | `{ content }` (auth)        |
| GET    | `/api/reports/:id/comments`      | Public                      |
| GET    | `/uploads/*`                     | Static photos & thumbnails  |

### Admin (`admin` or `authority` role)

| Method | Path                            | Notes                                                        |
| ------ | ------------------------------- | ------------------------------------------------------------ |
| GET    | `/api/admin/reports`            | Filters: `status`, `category`, `from`, `to`, `page`, `limit` |
| PUT    | `/api/admin/reports/:id/status` | `{ status, admin_notes, duplicate_of? }`                     |
| GET    | `/api/admin/stats`              | Dashboard stats                                              |

### Environment variables

| Variable                | Default                 | Description                             |
| ----------------------- | ----------------------- | --------------------------------------- |
| `DUPLICATE_MODE`        | `auto_flag`             | `manual`, `auto_flag`, or `auto_reject` |
| `DUPLICATE_LAT_EPSILON` | `0.00072`               | ~80m proximity box (DRC urban)          |
| `UPLOAD_DIR`            | `./uploads`             | Photo storage                           |
| `FRONTEND_URL`          | `http://localhost:3000` | Email links                             |
| `JWT_SECRET`            | —                       | Required in production                  |



/// Ajouter sur le dashboard de l'utilisateur, le motif de rejet de son report. 
Ne pas afficher le id du report qui est dupliquer sur le dashboard de l'utilisateur.
Dans la partie admin, ajouter le numero de telephone et l'email de la personne qui as appliquer. 
Ajouter soft delete pour un soft delete pour utilisateur et admin et l'admin peut effacer un utilisateur. 
Pouvoir imprimer un rapport et ajouter ce que on veut dedans avant d'imprimer. 


//*Enable the ability to print reports and add optional information before printing. = last one




# Printable Reports Feature

Analyze the existing report pages and the newly added backend endpoints before making changes.

Available endpoints:

/api/reports/:id/history-	GET	Get full timeline/audit trail of status changes
/api/reports/:id/attachments-	GET	Get all photos, videos, documents for a report
/api/reports/:id/printable-	GET	Complete report data with metadata, timeline, statistics, and attachments (for print preview)
/api/reports/:id/print-preview-	POST	Generate customizable preview with recipient, purpose, and toggle options (images/statistics/timeline)
/api/admin/reports/:id/pdf-	GET	Download official PDF document with watermark, page numbers, QR code, and signatures

Goal:

Allow users and administrators to print professional reports suitable for authorities and official records.

Requirements

## Print Button

Add a Print Report button on the report details page.

Clicking it should open a Print Configuration Modal.

Do not print immediately.

---

## Print Configuration Modal

Allow users to optionally provide:

* Recipient
* Purpose
* Additional Notes

Provide checkboxes:

* Include Images
* Include Statistics
* Include Charts
* Include Admin Notes
* Include Rejection Reason
* Include Duplicate Information
* Include Reporter Information (admin only)

These values should only affect printing and should not be stored permanently.

---

## Print Preview Page

Create a dedicated print preview page.

Display:

* Report title
* Reference number
* Status
* Category
* Severity
* Location
* Description
* Evidence
* Timeline information (if available)
* Statistics from GET /api/reports/:id/statistics
* Rejection reason
* Admin notes
* Duplicate information
* Recipient
* Purpose
* Additional notes

Design:

* A4 format
* White background
* Black text
* Professional appearance
* Proper margins
* Page breaks where needed

---

## Printing

Use browser printing.

Optimize with print CSS.

Hide buttons and navigation when printing.

---

## Statistics

Display charts and trend information if enabled.

Charts should remain printable.

---

## Error Handling

Handle:

* missing report data
* missing statistics
* empty images
* network failures

Prevent runtime crashes and provide loading states.

---

Deliverables

1. New pages created.
2. Components added.
3. Endpoints used.
4. Print CSS implemented.
5. Any backend limitations discovered.
