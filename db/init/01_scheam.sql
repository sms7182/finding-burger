

CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE IF NOT EXISTS vendors (
    id           SERIAL PRIMARY KEY,
    name         TEXT                     NOT NULL,
    service_zone geometry(Polygon, 4326)  NOT NULL,
    base_load    INTEGER                  NOT NULL DEFAULT 0,
    current_load INTEGER                  NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ              NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ              NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_vendors_service_zone
    ON vendors USING GIST (service_zone);
