package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cupid_hotel/internal/domain"
)

type QueryService struct {
	repo     domain.HotelRepository
	cache    domain.Cache
	cacheTTL time.Duration
}

func NewQueryService(r domain.HotelRepository, c domain.Cache, ttl time.Duration) *QueryService {
	return &QueryService{repo: r, cache: c, cacheTTL: ttl}
}

func (s *QueryService) GetHotel(ctx context.Context, id int64, lang string) (domain.HotelView, error) {
	key := fmt.Sprintf("hotel:%d:%s", id, lang)
	var hv domain.HotelView
	if ok, _ := s.cache.Get(ctx, key, &hv); ok {
		return hv, nil
	}
	h, err := s.repo.GetHotel(ctx, id, lang)
	if err != nil {
		return domain.HotelView{}, err
	}
	_ = s.cache.Set(ctx, key, h, int(s.cacheTTL.Seconds()))
	return h, nil
}

func (s *QueryService) ListReviews(ctx context.Context, id int64, pg domain.PageQuery) (domain.ReviewsPage, error) {
	key := fmt.Sprintf("reviews:%d:%d:%s", id, pg.Limit, pg.Sort)
	var out domain.ReviewsPage
	if ok, _ := s.cache.Get(ctx, key, &out); ok {
		return out, nil
	}

	rs, err := s.repo.ListReviews(ctx, id, pg)
	if err != nil {
		return domain.ReviewsPage{}, err
	}

	// copy slice to avoid aliasing the repo's backing array (prevents tests from mutating cached value)
	copyRS := deepCopyReviewsPage(rs)

	// optional size guard
	if b, _ := json.Marshal(copyRS); len(b) < 1_000_000 {
		_ = s.cache.Set(ctx, key, copyRS, int(s.cacheTTL.Seconds()))
	}
	return copyRS, nil
}

func deepCopyReviewsPage(in domain.ReviewsPage) domain.ReviewsPage {
	out := domain.ReviewsPage{NextCursor: in.NextCursor}
	if n := len(in.Items); n > 0 {
		out.Items = make([]domain.Review, n)
		copy(out.Items, in.Items)
	}
	return out
}
