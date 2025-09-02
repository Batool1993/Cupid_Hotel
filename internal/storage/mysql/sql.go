package mysql

const upsertPropertySQL = `
INSERT INTO properties
  (id, brand_id, stars, lat, lon, country, city, address_raw, amenities, images, raw)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  brand_id    = VALUES(brand_id),
  stars       = VALUES(stars),
  lat         = VALUES(lat),
  lon         = VALUES(lon),
  country     = VALUES(country),
  city        = VALUES(city),
  address_raw = VALUES(address_raw),
  amenities   = VALUES(amenities),
  images      = VALUES(images),
  raw         = VALUES(raw),
  updated_at  = CURRENT_TIMESTAMP
`

const upsertI18nSQL = `
INSERT INTO property_i18n
  (property_id, lang, name, description, policies, address, extras)
VALUES
  (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  name        = VALUES(name),
  description = VALUES(description),
  policies    = VALUES(policies),
  address     = VALUES(address),
  extras      = VALUES(extras)
`

// Note: `text` is reserved; keep it quoted everywhere.
const insertReviewsPrefix = "INSERT INTO reviews\n  (property_id, source_id, author, rating, lang, title, `text`, aspects, created_at, source, raw)\nVALUES "

// Use VALUES(col) for broad compatibility; COALESCE keeps old value if new is NULL.
const insertReviewsOnDup = " ON DUPLICATE KEY UPDATE\n" +
	"  author     = COALESCE(VALUES(author), reviews.author),\n" +
	"  rating     = COALESCE(VALUES(rating), reviews.rating),\n" +
	"  lang       = COALESCE(VALUES(lang), reviews.lang),\n" +
	"  title      = COALESCE(VALUES(title), reviews.title),\n" +
	"  `text`     = COALESCE(VALUES(`text`), reviews.`text`),\n" +
	"  aspects    = COALESCE(VALUES(aspects), reviews.aspects),\n" +
	"  created_at = COALESCE(VALUES(created_at), reviews.created_at),\n" +
	"  source     = COALESCE(VALUES(source), reviews.source),\n" +
	"  raw        = COALESCE(VALUES(raw), reviews.raw)\n"

const insertMissSQL = `
INSERT INTO ingest_misses (id, http_status, reason)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE seen_at = CURRENT_TIMESTAMP
`

// -----------------------------------------------------------------------------
// READ QUERIES
// -----------------------------------------------------------------------------

// Returns a single property joined with i18n for the requested lang.
// We include BOTH i.address (localized) and p.address_raw (base); in the repo,
// prefer i.address when not NULL, else fallback to p.address_raw.
const getHotelSQL = `
SELECT
  p.id,
  p.brand_id,
  p.stars,
  p.lat,
  p.lon,
  p.country,
  p.city,
  p.address_raw,          -- base address (fallback)
  p.amenities,
  p.images,
  i.name,
  i.description,
  i.policies,
  i.address               -- localized address (preferred when not NULL)
FROM properties p
LEFT JOIN property_i18n i
  ON i.property_id = p.id AND i.lang = ?
WHERE p.id = ?
`
