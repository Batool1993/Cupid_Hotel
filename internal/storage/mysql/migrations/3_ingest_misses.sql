-- 3_ingest_misses.sql — track failed fetches per property/reason

CREATE TABLE IF NOT EXISTS ingest_misses (
    id          BIGINT       NOT NULL,                 -- property id that failed
    http_status INT          NOT NULL,
    reason      VARCHAR(255) NOT NULL,                 -- e.g., 'not found', 'inactive', 'reviews', 'i18n:fr'
    seen_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id, reason)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
