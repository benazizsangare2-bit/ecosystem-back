package database

func GetTableQueries() []string {
	return []string{

`-- Users table (unified with phone verification)
CREATE TABLE IF NOT EXISTS users (
    user_id                   SERIAL PRIMARY KEY,
    email                     TEXT UNIQUE NOT NULL,
    phone_number              TEXT UNIQUE,
    first_name                TEXT NOT NULL,
    last_name                 TEXT NOT NULL,
    password_hash             TEXT NOT NULL,
    role                      TEXT NOT NULL DEFAULT 'citizen' CHECK (role IN ('citizen', 'admin', 'authority')),
    is_email_verified         BOOLEAN DEFAULT FALSE,
    is_phone_verified         BOOLEAN DEFAULT FALSE,
    email_verification_token  TEXT,
    email_verification_expiry TIMESTAMP WITH TIME ZONE,
    reset_token               TEXT,
    reset_token_expiry        TIMESTAMP WITH TIME ZONE,
    reputation_score          INTEGER DEFAULT 0,
    total_reports             INTEGER DEFAULT 0,
    total_upvotes_received    INTEGER DEFAULT 0,
    accept_notifications      BOOLEAN DEFAULT TRUE,
    last_login                TIMESTAMP WITH TIME ZONE,
    created_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    status                    TEXT DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'banned', 'deleted')),
    deleted_at                TIMESTAMP WITH TIME ZONE  -- Soft delete
)`,

`-- Reports table (also add for bad roads, )
CREATE TABLE IF NOT EXISTS reports (
    report_id                 SERIAL PRIMARY KEY,
    user_id                   INTEGER NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    title                     TEXT NOT NULL,
    description               TEXT NOT NULL,
    category                  TEXT NOT NULL CHECK (category IN ('illegal_dumping', 'overflowing_waste', 'air_pollution', 'water_contamination', 'noise_pollution', 'deforestation', 'bad_roads', 'other')),
    latitude                  DECIMAL(10, 8) NOT NULL,
    longitude                 DECIMAL(11, 8) NOT NULL,
    address                   TEXT,
    photo_urls                TEXT[],
    photo_hashes              TEXT[],
    status                    TEXT DEFAULT 'pending' CHECK (status IN ('pending', 'under_review', 'investigating', 'resolved', 'rejected', 'duplicate')),
    duplicate_of              INTEGER REFERENCES reports(report_id) ON DELETE SET NULL,
    resolved_by               INTEGER REFERENCES users(user_id),
    resolved_at               TIMESTAMP WITH TIME ZONE,
    assigned_to               INTEGER REFERENCES users(user_id),
    upvote_count              INTEGER DEFAULT 0,
    view_count                INTEGER DEFAULT 0,
    created_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    admin_notes               TEXT,
    duplicate_warning         BOOLEAN DEFAULT FALSE,
    thumbnail_urls            TEXT[]
)`,

`-- Revoked JWT identifiers (logout / security)
CREATE TABLE IF NOT EXISTS token_blacklist (
    jti                       TEXT PRIMARY KEY,
    expires_at                TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW()
)`,

`-- Upvotes table (many-to-many)
CREATE TABLE IF NOT EXISTS report_upvotes (
    upvote_id                 SERIAL PRIMARY KEY,
    report_id                 INTEGER NOT NULL REFERENCES reports(report_id) ON DELETE CASCADE,
    user_id                   INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    created_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(report_id, user_id)
)`,

`-- Report comments
CREATE TABLE IF NOT EXISTS report_comments (
    comment_id                SERIAL PRIMARY KEY,
    report_id                 INTEGER NOT NULL REFERENCES reports(report_id) ON DELETE CASCADE,
    user_id                   INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    comment                   TEXT NOT NULL,
    is_official_response      BOOLEAN DEFAULT FALSE,
    created_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW()
)`,

`-- Notifications
CREATE TABLE IF NOT EXISTS notifications (
    notification_id           SERIAL PRIMARY KEY,
    user_id                   INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    report_id                 INTEGER REFERENCES reports(report_id) ON DELETE CASCADE,
    title                     TEXT NOT NULL,
    message                   TEXT NOT NULL,
    is_read                   BOOLEAN DEFAULT FALSE,
    created_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW()
)`,


`-- Refresh tokens for JWT
CREATE TABLE IF NOT EXISTS refresh_tokens (
    token_id                  SERIAL PRIMARY KEY,
    user_id                   INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    token                     TEXT UNIQUE NOT NULL,
    expires_at                TIMESTAMP WITH TIME ZONE NOT NULL,
    revoked                   BOOLEAN DEFAULT FALSE,
    created_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW()
)`,

`-- Audit logs for admin actions (accountability)
CREATE TABLE IF NOT EXISTS audit_logs (
    log_id                    SERIAL PRIMARY KEY,
    admin_id                  INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    action                    TEXT NOT NULL,
    target_type               TEXT,
    target_id                 INTEGER,
    old_data                  JSONB,
    new_data                  JSONB,
    ip_address                INET,
    user_agent                TEXT,
    created_at                TIMESTAMP WITH TIME ZONE DEFAULT NOW()
)`,


`-- Indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_reports_user_id ON reports(user_id);
CREATE INDEX IF NOT EXISTS idx_reports_status ON reports(status);
CREATE INDEX IF NOT EXISTS idx_reports_category ON reports(category);
CREATE INDEX IF NOT EXISTS idx_reports_created_at ON reports(created_at);
CREATE INDEX IF NOT EXISTS idx_reports_location ON reports(latitude, longitude);
CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_is_read ON notifications(is_read);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_token ON refresh_tokens(token);
CREATE INDEX IF NOT EXISTS idx_token_blacklist_expires ON token_blacklist(expires_at);
`,

`-- Report history/timeline
CREATE TABLE IF NOT EXISTS report_history (
    history_id            SERIAL PRIMARY KEY,
    report_id             INTEGER NOT NULL REFERENCES reports(report_id) ON DELETE CASCADE,
    status                TEXT NOT NULL,
    changed_by            INTEGER REFERENCES users(user_id),
    changed_by_name       TEXT,
    notes                 TEXT,
    created_at            TIMESTAMP WITH TIME ZONE DEFAULT NOW()
)
`,

`-- Migrations for existing databases
ALTER TABLE reports ADD COLUMN IF NOT EXISTS admin_notes TEXT;
ALTER TABLE reports ADD COLUMN IF NOT EXISTS duplicate_warning BOOLEAN DEFAULT FALSE;
ALTER TABLE reports ADD COLUMN IF NOT EXISTS thumbnail_urls TEXT[];
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE;
`,

`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'suspended', 'banned', 'deleted'));
`,
`CREATE EXTENSION IF NOT EXISTS pg_trgm;`,
//CREATE INDEX idx_reports_description_trgm ON reports USING gin(description gin_trgm_ops);

`CREATE INDEX IF NOT EXISTS idx_report_history_report_id ON report_history(report_id)`,
`CREATE INDEX IF NOT EXISTS idx_report_history_created_at ON report_history(created_at)`,
	}
}