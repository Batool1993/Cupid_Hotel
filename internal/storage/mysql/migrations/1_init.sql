-- Fresh schema for Cupid Hotel assignment
SET NAMES utf8mb4;
SET time_zone = '+00:00';

-- -----------------------------------------------------------------------------
-- properties: base hotel data (language-agnostic)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS properties (
    id          BIGINT        NOT NULL,                         -- Cupid/property id
    brand_id    BIGINT        NULL,
    stars       INT           NULL,
    lat         DOUBLE        NULL,
    lon         DOUBLE        NULL,
    country     VARCHAR(64)   NULL,
    city        VARCHAR(128)  NULL,
    address_raw VARCHAR(512)  NULL,                             -- composed or raw base address
    amenities   JSON          NULL,
    images      JSON          NULL,
    raw         JSON          NULL,                             -- full original payload
    created_at  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- -----------------------------------------------------------------------------
-- property_i18n: localized fields per language
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS property_i18n (
                                             property_id  BIGINT        NOT NULL,
                                             lang         VARCHAR(10)   NOT NULL,                        -- e.g. 'en', 'fr', 'es'
    name         VARCHAR(255)  NULL,
    description  TEXT          NULL,
    policies     TEXT          NULL,                            -- TEXT to tolerate non-JSON / empty values
    address      VARCHAR(512)  NULL,                            -- localized address if provider supplies it
    extras       JSON          NULL,                            -- leftover localized fields
    created_at   TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (property_id, lang),
    CONSTRAINT fk_i18n_property FOREIGN KEY (property_id)
    REFERENCES properties(id) ON DELETE CASCADE
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- -----------------------------------------------------------------------------
-- reviews: user reviews per property
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS reviews (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    property_id  BIGINT          NOT NULL,
    source_id    VARCHAR(191)    NULL,
    author       VARCHAR(255)    NULL,
    rating       DECIMAL(4,2)    NULL,
    lang         VARCHAR(10)     NULL,
    title        VARCHAR(255)    NULL,
    `text`       TEXT            NULL,
    aspects      TEXT            NULL,   -- TEXT to tolerate empty string from provider
    created_at   TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    source       VARCHAR(64)     NULL,
    raw          JSON            NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_reviews_natural (property_id, source, source_id),
    KEY idx_reviews_prop_created (property_id, created_at),
    CONSTRAINT fk_reviews_property FOREIGN KEY (property_id)
    REFERENCES properties(id) ON DELETE CASCADE
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
