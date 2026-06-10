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
