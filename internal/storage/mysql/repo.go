package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"cupid_hotel/internal/domain"
)

func valStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
func valInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
func valInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
func valF64(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
func valJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

type Repo struct{ db *sql.DB }

func New(db *sql.DB) *Repo { return &Repo{db: db} }

func (r *Repo) UpsertProperty(ctx context.Context, h domain.Hotel) error {
	amen, _ := json.Marshal(h.Amenities)
	imgs, _ := json.Marshal(h.Images)
	_, err := r.db.ExecContext(ctx, upsertPropertySQL,
		h.ID,
		valInt64(h.BrandID),
		valInt(h.Stars),
		valF64(h.Lat),
		valF64(h.Lon),
		valStr(h.Country),
		valStr(h.City),
		valStr(h.AddressRaw),
		string(amen),
		string(imgs),
		string(h.RawJSON),
	)
	return err
}

func (r *Repo) UpsertI18n(ctx context.Context, i domain.HotelI18n) error {
	_, err := r.db.ExecContext(ctx, upsertI18nSQL,
		i.PropertyID,
		i.Lang, // string in your domain
		i.Name,
		i.Description,
		i.Policies,
		i.Address,
		string(i.ExtrasJSON),
	)
	return err
}

func (r *Repo) UpsertReviews(ctx context.Context, rs []domain.Review) error {
	if len(rs) == 0 {
		return nil
	}
	values := make([]string, 0, len(rs))
	args := make([]any, 0, len(rs)*11) // 11 params per row (includes 'aspects')
	for _, rv := range rs {
		// Columns (from insertReviewsPrefix):
		// (property_id, source_id, author, rating, lang, title, `text`, aspects, created_at, source, raw)
		// created_at value is COALESCE(?, CURRENT_TIMESTAMP) to allow "unknown" timestamps.
		values = append(values, "(?,?,?,?,?,?,?,?,COALESCE(?, CURRENT_TIMESTAMP),?,?)")
		args = append(args,
			rv.PropertyID,          // property_id
			valStr(rv.SourceID),    // source_id
			valStr(rv.Author),      // author
			valF64(rv.Rating),      // rating
			valStr(rv.Lang),        // lang
			valStr(rv.Title),       // title
			valStr(rv.Text),        // text
			string(rv.AspectsJSON), // aspects (JSON text or "")
			nil,                    // created_at param to COALESCE
			valStr(rv.Source),      // source
			string(rv.RawJSON),     // raw
		)
	}
	sqlStr := insertReviewsPrefix + strings.Join(values, ",") + insertReviewsOnDup
	_, err := r.db.ExecContext(ctx, sqlStr, args...)
	return err
}

func (r *Repo) LogMiss(ctx context.Context, id int64, status int, reason string) error {
	_, err := r.db.ExecContext(ctx, insertMissSQL, id, status, reason)
	return err
}

func (r *Repo) GetHotel(ctx context.Context, id int64, lang string) (domain.HotelView, error) {
	// Use the shared SELECT with both base and i18n address columns
	row := r.db.QueryRowContext(ctx, getHotelSQL, lang, id)

	var hv domain.HotelView
	var brandID sql.NullInt64 // present in the SELECT, but not used directly in the view here
	var stars sql.NullInt64
	var lat, lon sql.NullFloat64
	var country, city sql.NullString
	var amenitiesJSON, imagesJSON []byte
	var name, desc, pol sql.NullString
	var baseAddr, i18nAddr sql.NullString

	if err := row.Scan(
		&hv.ID,
		&brandID,
		&stars,
		&lat, &lon,
		&country, &city,
		&baseAddr,
		&amenitiesJSON, &imagesJSON,
		&name, &desc, &pol,
		&i18nAddr,
	); err != nil {
		if err == sql.ErrNoRows {
			return domain.HotelView{}, domain.ErrNotFound
		}
		return domain.HotelView{}, err
	}

	if stars.Valid {
		s := int(stars.Int64)
		hv.Stars = &s
	}
	if lat.Valid && lon.Valid {
		hv.Coords = &domain.Coords{Lat: lat.Float64, Lon: lon.Float64}
	}
	if country.Valid {
		cs := country.String
		hv.Country = &cs
	}
	if city.Valid {
		cy := city.String
		hv.City = &cy
	}

	// Prefer localized address when present; otherwise fallback to base address_raw
	if i18nAddr.Valid && strings.TrimSpace(i18nAddr.String) != "" {
		addr := i18nAddr.String
		hv.Address = &addr
	} else if baseAddr.Valid && strings.TrimSpace(baseAddr.String) != "" {
		addr := baseAddr.String
		hv.Address = &addr
	}

	_ = json.Unmarshal(amenitiesJSON, &hv.Amenities)
	_ = json.Unmarshal(imagesJSON, &hv.Images)
	if name.Valid {
		ns := name.String
		hv.Name = &ns
	}
	if desc.Valid {
		ds := desc.String
		hv.Description = &ds
	}
	if pol.Valid {
		ps := pol.String
		hv.Policies = &ps
	}
	hv.Language = lang
	return hv, nil
}

func (r *Repo) ListHotels(ctx context.Context, q domain.HotelsQuery) (domain.HotelsPage, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT p.id, p.stars, p.lat, p.lon, p.country, p.city, i.name
FROM properties p
LEFT JOIN property_i18n i ON i.property_id = p.id AND i.lang = ?
ORDER BY p.id
LIMIT ?`, q.Lang, q.Limit)
	if err != nil {
		return domain.HotelsPage{}, err
	}
	defer rows.Close()

	var out []domain.HotelView
	for rows.Next() {
		var hv domain.HotelView
		var stars sql.NullInt64
		var lat, lon sql.NullFloat64
		var country, city, name sql.NullString
		if err := rows.Scan(&hv.ID, &stars, &lat, &lon, &country, &city, &name); err != nil {
			return domain.HotelsPage{}, err
		}
		if stars.Valid {
			s := int(stars.Int64)
			hv.Stars = &s
		}
		if lat.Valid && lon.Valid {
			hv.Coords = &domain.Coords{Lat: lat.Float64, Lon: lon.Float64}
		}
		if country.Valid {
			cs := country.String
			hv.Country = &cs
		}
		if city.Valid {
			cy := city.String
			hv.City = &cy
		}
		if name.Valid {
			ns := name.String
			hv.Name = &ns
		}
		out = append(out, hv)
	}
	return domain.HotelsPage{Items: out}, nil
}

func (r *Repo) ListReviews(ctx context.Context, id int64, pg domain.PageQuery) (domain.ReviewsPage, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT
		   id,
		   property_id,
		   source_id,
		   author,
		   rating,
		   lang,
		   title,
		   text,
		   aspects,
		   created_at,
		   source,
		   raw
		 FROM reviews
		 WHERE property_id=?
		 ORDER BY created_at DESC, id DESC
		 LIMIT ?`,
		id, pg.Limit,
	)
	if err != nil {
		return domain.ReviewsPage{}, err
	}
	defer rows.Close()

	var out []domain.Review
	for rows.Next() {
		var rv domain.Review
		var (
			sourceID         sql.NullString
			author           sql.NullString
			rating           sql.NullFloat64
			lang             sql.NullString
			title            sql.NullString
			text             sql.NullString
			aspectsRaw, rawB sql.RawBytes
			createdAt        sql.NullTime
			source           sql.NullString
		)
		if err := rows.Scan(
			&rv.ID,
			&rv.PropertyID,
			&sourceID,
			&author,
			&rating,
			&lang,
			&title,
			&text,
			&aspectsRaw,
			&createdAt, // ignored if your domain.Review has no CreatedAt field
			&source,
			&rawB,
		); err != nil {
			return domain.ReviewsPage{}, err
		}

		if sourceID.Valid {
			s := sourceID.String
			rv.SourceID = &s
		}
		if author.Valid {
			s := author.String
			rv.Author = &s
		}
		if rating.Valid {
			f := rating.Float64
			rv.Rating = &f
		}
		if lang.Valid {
			s := lang.String
			rv.Lang = &s
		}
		if title.Valid {
			s := title.String
			rv.Title = &s
		}
		if text.Valid {
			s := text.String
			rv.Text = &s
		}
		if len(aspectsRaw) > 0 {
			rv.AspectsJSON = append([]byte(nil), aspectsRaw...)
		}
		if source.Valid {
			s := source.String
			rv.Source = &s
		}
		if len(rawB) > 0 {
			rv.RawJSON = append([]byte(nil), rawB...)
		}

		out = append(out, rv)
	}
	if err := rows.Err(); err != nil {
		return domain.ReviewsPage{}, err
	}
	return domain.ReviewsPage{Items: out}, nil
}
