package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cupid_hotel/internal/domain"
)

type IngestionService struct {
	cupid domain.CupidClient
	repo  domain.HotelRepository
	cache domain.Cache
}

func NewIngestionService(c domain.CupidClient, r domain.HotelRepository, cache domain.Cache) *IngestionService {
	return &IngestionService{cupid: c, repo: r, cache: cache}
}

func (s *IngestionService) IngestHotel(ctx context.Context, id int64, reviewCount int) error {
	// 1) Fetch property (parent first). Handle known 404/401/403 as "misses".
	p, err := s.cupid.GetProperty(ctx, id)
	if err != nil {
		low := strings.ToLower(err.Error())

		// 404: property not found -> record miss, clear caches, and stop gracefully.
		if errors.Is(err, domain.ErrNotFound) || strings.Contains(low, "not found") {
			_ = s.repo.LogMiss(ctx, id, 404, "not found")
			// Evict any stale caches so we don't keep serving an old snapshot.
			if s.cache != nil {
				s.invalidateHotelAllLangs(ctx, id)
				s.invalidateReviews(ctx, id)
			}
			return nil
		}

		// 401/403: unauthorized/forbidden/inactive -> record miss, evict caches, stop.
		if strings.Contains(low, "403") || strings.Contains(low, "forbidden") ||
			strings.Contains(low, "401") || strings.Contains(low, "unauthorized") {
			_ = s.repo.LogMiss(ctx, id, 403, "inactive")
			if s.cache != nil {
				s.invalidateHotelAllLangs(ctx, id)
				s.invalidateReviews(ctx, id)
			}
			return nil
		}

		// Anything else is unexpected (network/5xx/JSON/etc.) -> bubble up.
		return err
	}

	// Parent upsert first to satisfy FK for i18n/reviews.
	if err := s.repo.UpsertProperty(ctx, mapProperty(p)); err != nil {
		return err
	}

	// Property change affects all languages -> invalidate all hotel caches.
	if s.cache != nil {
		s.invalidateHotelAllLangs(ctx, id)
	}

	// 2) Reviews: best-effort. We don't fail ingestion on 404/401/403,
	// but we do bubble up other errors. We always invalidate the reviews cache
	// after a successful call (even if the list is empty) to avoid stale cache.
	if revs, rerr := s.cupid.GetReviews(ctx, id, reviewCount); rerr != nil {
		low := strings.ToLower(rerr.Error())
		switch {
		case errors.Is(rerr, domain.ErrNotFound) || strings.Contains(low, "not found"):
			_ = s.repo.LogMiss(ctx, id, 404, "reviews")
			if s.cache != nil {
				s.invalidateReviews(ctx, id)
			}
		case strings.Contains(low, "403") || strings.Contains(low, "forbidden") ||
			strings.Contains(low, "401") || strings.Contains(low, "unauthorized"):
			_ = s.repo.LogMiss(ctx, id, 403, "reviews")
			if s.cache != nil {
				s.invalidateReviews(ctx, id)
			}
		default:
			return rerr
		}
	} else {
		// success: even if zero reviews, invalidate cache to drop any stale entries
		if len(revs) > 0 {
			if err := s.repo.UpsertReviews(ctx, mapReviews(id, revs)); err != nil {
				// IMPORTANT: do not swallow this; surface so we know inserts failed
				return fmt.Errorf("upsert reviews failed for %d: %w", id, err)
			}
		}
		if s.cache != nil {
			s.invalidateReviews(ctx, id)
		}
	}

	// 3) Translations: try en, fr, es; log misses per-language; continue on 404/401/403.
	for _, lang := range []string{"en", "fr", "es"} {
		tr, terr := s.cupid.GetTranslation(ctx, id, lang)
		if terr != nil {
			low := strings.ToLower(terr.Error())

			if errors.Is(terr, domain.ErrNotFound) || strings.Contains(low, "not found") {
				_ = s.repo.LogMiss(ctx, id, 404, "i18n:"+lang)
				// Invalidate this language cache so we don't serve a stale cached translation.
				if s.cache != nil {
					s.invalidateHotelLang(ctx, id, lang)
				}
				continue
			}
			if strings.Contains(low, "403") || strings.Contains(low, "forbidden") ||
				strings.Contains(low, "401") || strings.Contains(low, "unauthorized") {
				_ = s.repo.LogMiss(ctx, id, 403, "i18n:"+lang)
				if s.cache != nil {
					s.invalidateHotelLang(ctx, id, lang)
				}
				continue
			}

			// Unknown/unexpected error: surface it.
			return terr
		}

		// Upsert this language and evict only that language's hotel cache.
		if err := s.repo.UpsertI18n(ctx, mapI18n(id, lang, tr)); err != nil {
			return err
		}
		if s.cache != nil {
			s.invalidateHotelLang(ctx, id, lang)
		}
	}

	return nil
}

// invalidate hotel caches
func (s *IngestionService) invalidateHotelAllLangs(ctx context.Context, id int64) {
	for _, l := range []string{"en", "fr", "es"} {
		s.invalidateHotelLang(ctx, id, l)
	}
}

func (s *IngestionService) invalidateHotelLang(ctx context.Context, id int64, lang string) {
	_ = s.cache.Del(ctx, fmt.Sprintf("hotel:%d:%s", id, strings.ToLower(lang)))
}

// invalidate the most common review cache variants
func (s *IngestionService) invalidateReviews(ctx context.Context, id int64) {
	// Your API default is limit=50, sort=-created_at. Invalidate that first.
	_ = s.cache.Del(ctx, fmt.Sprintf("reviews:%d:%d:%s", id, 50, "-created_at"))
	// Optionally clear a couple more common limits to be safe:
	for _, lim := range []int{100, 200} {
		_ = s.cache.Del(ctx, fmt.Sprintf("reviews:%d:%d:%s", id, lim, "-created_at"))
	}
}
