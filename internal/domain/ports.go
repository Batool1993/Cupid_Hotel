package domain

import "context"

type HotelRepository interface {
	// Write paths
	UpsertProperty(ctx context.Context, h Hotel) error
	UpsertI18n(ctx context.Context, i HotelI18n) error
	UpsertReviews(ctx context.Context, rs []Review) error
	LogMiss(ctx context.Context, id int64, status int, reason string) error

	// Read paths
	GetHotel(ctx context.Context, id int64, lang string) (HotelView, error)
	ListHotels(ctx context.Context, q HotelsQuery) (HotelsPage, error)
	ListReviews(ctx context.Context, id int64, pg PageQuery) (ReviewsPage, error)
}

type CupidClient interface {
	GetProperty(ctx context.Context, id int64) (map[string]any, error)
	GetTranslation(ctx context.Context, id int64, lang string) (map[string]any, error)
	GetReviews(ctx context.Context, id int64, count int) ([]map[string]any, error)
}

type Cache interface {
	Get(ctx context.Context, key string, dst any) (bool, error)
	Set(ctx context.Context, key string, v any, ttlSec int) error
	Del(ctx context.Context, key string) error
}

// Read models & queries
type HotelView struct {
	ID          int64
	Stars       *int
	Coords      *Coords
	Country     *string
	City        *string
	Address     *string // <-- add this
	Name        *string
	Description *string
	Policies    *string
	Amenities   []string
	Images      []string
	Language    string
}

type Coords struct{ Lat, Lon float64 }

type HotelsQuery struct {
	Lang          string
	Q             *string
	Country, City *string
	Stars         *int
	Amenity       *string
	Limit         int
	Cursor        *string
}

type PageQuery struct {
	Limit  int
	Cursor *string
	Sort   string
}

type HotelsPage struct {
	Items      []HotelView
	NextCursor *string
}

type ReviewsPage struct {
	Items      []Review
	NextCursor *string
}
