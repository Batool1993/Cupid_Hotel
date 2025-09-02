package app_test

import (
	"context"
	"testing"
	"time"

	"cupid_hotel/internal/app"
	"cupid_hotel/internal/domain"
)

// ---- fakes ----

type fakeRepo struct {
	hv domain.HotelView
	rp domain.ReviewsPage
}

func (f *fakeRepo) UpsertProperty(ctx context.Context, h domain.Hotel) error    { return nil }
func (f *fakeRepo) UpsertI18n(ctx context.Context, i domain.HotelI18n) error    { return nil }
func (f *fakeRepo) UpsertReviews(ctx context.Context, rs []domain.Review) error { return nil }
func (f *fakeRepo) GetHotel(ctx context.Context, id int64, lang string) (domain.HotelView, error) {
	return f.hv, nil
}
func (f *fakeRepo) ListHotels(ctx context.Context, q domain.HotelsQuery) (domain.HotelsPage, error) {
	return domain.HotelsPage{}, nil
}
func (f *fakeRepo) ListReviews(ctx context.Context, id int64, pg domain.PageQuery) (domain.ReviewsPage, error) {
	return f.rp, nil
}
func (f *fakeRepo) LogMiss(ctx context.Context, id int64, status int, reason string) error {
	// no-op for tests
	return nil
}

type fakeCache struct {
	store map[string]any
}

func (c *fakeCache) Get(ctx context.Context, key string, dst any) (bool, error) {
	if c.store == nil {
		return false, nil
	}
	v, ok := c.store[key]
	if !ok {
		return false, nil
	}
	switch d := dst.(type) {
	case *domain.HotelView:
		*d = v.(domain.HotelView)
	case *domain.ReviewsPage:
		*d = v.(domain.ReviewsPage)
	}
	return true, nil
}
func (c *fakeCache) Set(ctx context.Context, key string, v any, ttlSec int) error {
	if c.store == nil {
		c.store = map[string]any{}
	}
	c.store[key] = v
	return nil
}
func (c *fakeCache) Del(ctx context.Context, key string) error { return nil }

// ---- tests ----

func TestGetHotel_CacheMissThenHit(t *testing.T) {
	repo := &fakeRepo{
		hv: domain.HotelView{ID: 42, Language: "fr", Name: ptr("Hôtel Test")},
	}
	cache := &fakeCache{}
	q := app.NewQueryService(repo, cache, 10*time.Minute)

	// Miss (first time, populates cache)
	h, err := q.GetHotel(context.Background(), 42, "fr")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if h.ID != 42 || h.Language != "fr" || h.Name == nil || *h.Name != "Hôtel Test" {
		t.Fatalf("unexpected hotel: %+v", h)
	}

	// Mutate repo to ensure second read indeed comes from cache
	repo.hv.Name = ptr("SHOULD NOT SEE THIS")

	// Hit (served from cache)
	h2, err := q.GetHotel(context.Background(), 42, "fr")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if *h2.Name != "Hôtel Test" {
		t.Fatalf("expected cached name, got %s", deref(h2.Name))
	}
}

func TestListReviews_Cache(t *testing.T) {
	repo := &fakeRepo{
		rp: domain.ReviewsPage{Items: []domain.Review{
			{PropertyID: 1, Author: ptr("Ana"), Rating: pfloat(9.0)},
		}},
	}
	cache := &fakeCache{}
	q := app.NewQueryService(repo, cache, 10*time.Minute)

	out, err := q.ListReviews(context.Background(), 1, domain.PageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out.Items) != 1 || deref(out.Items[0].Author) != "Ana" {
		t.Fatalf("unexpected reviews: %+v", out.Items)
	}

	// Change repo, call again -> should come from cache
	repo.rp.Items[0].Author = ptr("Changed")
	out2, _ := q.ListReviews(context.Background(), 1, domain.PageQuery{Limit: 10})
	if deref(out2.Items[0].Author) != "Ana" {
		t.Fatalf("expected cached author Ana, got %s", deref(out2.Items[0].Author))
	}
}

func ptr[T any](v T) *T { return &v }
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func pfloat(f float64) *float64 { return &f }
