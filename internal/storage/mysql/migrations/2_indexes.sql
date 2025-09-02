-- 2_indexes.sql — idempotent index & generated column setup (MySQL 8.0 safe)

-- FULLTEXT on property_i18n(name, description)
SET @exists := (
  SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME   = 'property_i18n'
    AND INDEX_NAME   = 'ftx_i18n_name_desc'
);
SET @sql := IF(
  @exists = 0,
  'ALTER TABLE property_i18n ADD FULLTEXT INDEX ftx_i18n_name_desc (name, description)',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- Simple index on property_i18n(lang)
SET @exists := (
  SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME   = 'property_i18n'
    AND INDEX_NAME   = 'idx_i18n_lang'
);
SET @sql := IF(
  @exists = 0,
  'ALTER TABLE property_i18n ADD INDEX idx_i18n_lang (lang)',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- Generated column has_spa derived from amenities JSON
SET @col_exists := (
  SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME   = 'properties'
    AND COLUMN_NAME  = 'has_spa'
);
SET @sql := IF(
  @col_exists = 0,
  'ALTER TABLE properties ADD COLUMN has_spa TINYINT AS (JSON_CONTAINS(amenities, JSON_QUOTE(''spa''))) STORED',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- Index on has_spa
SET @idx_exists := (
  SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME   = 'properties'
    AND INDEX_NAME   = 'idx_has_spa'
);
SET @sql := IF(
  @idx_exists = 0,
  'ALTER TABLE properties ADD INDEX idx_has_spa (has_spa)',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
