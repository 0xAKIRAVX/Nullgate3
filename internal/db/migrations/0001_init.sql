-- NullGate 3.0 initial schema (PostgreSQL 14+)

CREATE TABLE IF NOT EXISTS settings (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS admins (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      text UNIQUE NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS clients (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    quota      bigint NOT NULL DEFAULT 0,          -- bytes, 0 = unlimited
    expire_at  timestamptz,                        -- NULL = never
    protocols  jsonb NOT NULL DEFAULT '[]'::jsonb, -- per-user overrides; [] = follow global
    note       text NOT NULL DEFAULT '',
    sub_token  text UNIQUE NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS client_usage (
    client_id  uuid PRIMARY KEY REFERENCES clients(id) ON DELETE CASCADE,
    up         bigint NOT NULL DEFAULT 0,
    down       bigint NOT NULL DEFAULT 0,
    reset_at   timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS inbounds (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tag        text UNIQUE NOT NULL,
    name       text NOT NULL,
    protocol   text NOT NULL,
    network    text NOT NULL,
    security   text NOT NULL DEFAULT 'none',
    port       int,
    lport      int,
    path       text NOT NULL DEFAULT '',
    svc        text NOT NULL DEFAULT '',
    sni        text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_clients_created ON clients (created_at DESC);
