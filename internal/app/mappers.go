package app

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"strconv"
	"strings"

	"cupid_hotel/internal/domain"
)

/********** alias registries (single source of truth) **********/

var reviewAliases = map[string][]string{
	"author":       {"author", "name", "userName", "reviewer", "reviewer.name"},
	"author_first": {"first_name", "firstname", "user.first_name", "user.firstName"},
	"author_last":  {"last_name", "lastname", "user.last_name", "user.lastName"},
	"title":        {"title", "review_title", "headline", "summary"},
	"text":         {"text", "review_text", "review", "comment", "content", "body", "message"},
	"lang":         {"lang", "language", "language_code", "languageCode", "locale"},
	"source":       {"source", "platform", "provider", "site", "origin"},
	"source_id":    {"id", "review_id", "reviewId"},
	"rating":       {"rating", "rate", "score", "rating.value", "scores.overall", "overall_score", "average_score"},
}

var i18nAliases = map[string][]string{
	"name":        {"name", "hotel_name", "translations.name"},
	"description": {"description", "markdown_description", "translations.description", "description_long"},
	"policies":    {"policies", "important_info", "translations.policies"},
	"address": {
		"address", "address.line", "address_raw", "full_address",
		"address1", "address_line1", "location.address",
		"street", "street_address",
	},
}

/********** tiny helpers **********/

// lookupAny: safe nested lookup with dot paths on maps.
func lookupAny(m map[string]any, path string) any {
	cur := any(m)
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		v, ok := obj[part]
		if !ok {
			return nil
		}
		cur = v
	}
	return cur
}

// lookupStr returns string at path or "".
func lookupStr(m map[string]any, path string) string {
	if v := lookupAny(m, path); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// firstNonEmptyAlias: first non-empty string for a named alias set.
func firstNonEmptyAlias(m map[string]any, aliases map[string][]string, key string) *string {
	for _, p := range aliases[key] {
		if s := lookupStr(m, p); s != "" {
			return &s
		}
	}
	return nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptrStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, " ")
}

// getFloatFlexible: number from several paths (float64/int/string like "8,0").
func getFloatFlexible(m map[string]any, paths ...string) *float64 {
	for _, k := range paths {
		switch v := lookupAny(m, k).(type) {
		case float64:
			f := v
			return &f
		case int:
			f := float64(v)
			return &f
		case string:
			s := strings.TrimSpace(strings.ReplaceAll(v, ",", "."))
			if s == "" {
				continue
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return &f
			}
		}
	}
	return nil
}

// firstInt64Flexible: int64 from several paths (float64/int/string).
func firstInt64Flexible(m map[string]any, paths ...string) *int64 {
	for _, k := range paths {
		switch v := lookupAny(m, k).(type) {
		case float64:
			x := int64(v)
			return &x
		case int:
			x := int64(v)
			return &x
		case int64:
			x := v
			return &x
		case string:
			s := strings.TrimSpace(v)
			if s == "" {
				continue
			}
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return &n
			}
		}
	}
	return nil
}

// firstSliceStrings: accept []any with either strings or {url/src/name}.
func firstSliceStrings(m map[string]any, paths ...string) []string {
	for _, k := range paths {
		if raw, ok := lookupAny(m, k).([]any); ok {
			out := make([]string, 0, len(raw))
			for _, it := range raw {
				switch t := it.(type) {
				case string:
					if t != "" {
						out = append(out, t)
					}
				case map[string]any:
					if u, ok := t["url"].(string); ok && u != "" {
						out = append(out, u)
						continue
					}
					if u, ok := t["src"].(string); ok && u != "" {
						out = append(out, u)
						continue
					}
					if n, ok := t["name"].(string); ok && n != "" {
						out = append(out, n)
						continue
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

// topLevelKnownFromAliases builds a set of top-level keys to exclude from extras.
func topLevelKnownFromAliases(aliases map[string][]string, keys ...string) map[string]struct{} {
	set := make(map[string]struct{}, 16)
	for _, k := range keys {
		for _, path := range aliases[k] {
			top := path
			if i := strings.IndexByte(top, '.'); i >= 0 {
				top = top[:i]
			}
			set[top] = struct{}{}
		}
	}
	return set
}

/********** property mapper **********/

func mapProperty(p map[string]any) domain.Hotel {
	id := int64(0)
	if v := firstInt64Flexible(p, "hotel_id", "cupid_id", "id"); v != nil {
		id = *v
	}

	raw, err := json.Marshal(p)
	if err != nil {
		log.Error().Err(err).
			Str("context", "mapProperty").
			Msg("failed to marshal property to JSON")
	}

	return domain.Hotel{
		ID:      id,
		BrandID: firstInt64Flexible(p, "chain_id", "brand_id"),
		Stars: func() *int {
			// int from rating-ish fields
			if f := getFloatFlexible(p, "stars", "rating.stars", "rating"); f != nil {
				x := int(*f)
				return &x
			}
			return nil
		}(),
		Lat: getFloatFlexible(p, "latitude", "lat", "location.lat"),
		Lon: getFloatFlexible(p, "longitude", "lon", "lng", "location.lon", "location.lng"),
		Country: func() *string {
			return firstNonEmptyAlias(p, map[string][]string{"country": {"address.country", "country", "countryCode", "country_code"}}, "country")
		}(),
		City: func() *string {
			return firstNonEmptyAlias(p, map[string][]string{"city": {"address.city", "city", "locality", "town"}}, "city")
		}(),
		AddressRaw: func() *string {
			// 1) Try known single-field aliases first
			if s := firstNonEmptyAlias(p, map[string][]string{
				"addr": {
					"address_raw",
					"address",
					"address.line",
					"full_address",
					"location.address",
					"formatted_address",
				},
			}, "addr"); s != nil && *s != "" {
				return s
			}

			// 2) Compose from components if no single field is present
			parts := []string{
				lookupStr(p, "address.addressLine1"),
				lookupStr(p, "address.addressLine2"),
				lookupStr(p, "address.street"),
				lookupStr(p, "address.district"),
				lookupStr(p, "address.city"),
				lookupStr(p, "address.state"),
				lookupStr(p, "address.postcode"),
				lookupStr(p, "address.zip"),
				lookupStr(p, "address.country"),
				// sometimes flattened at the root:
				lookupStr(p, "street"),
				lookupStr(p, "city"),
				lookupStr(p, "postcode"),
				lookupStr(p, "zip"),
				lookupStr(p, "country"),
			}
			nonEmpty := make([]string, 0, len(parts))
			for _, part := range parts {
				if t := strings.TrimSpace(part); t != "" {
					nonEmpty = append(nonEmpty, t)
				}
			}
			if len(nonEmpty) > 0 {
				composed := strings.Join(nonEmpty, ", ")
				return &composed
			}
			return nil
		}(),
		Amenities: firstSliceStrings(p, "facilities", "amenities"),
		Images:    firstSliceStrings(p, "photos", "images"),
		RawJSON:   raw,
	}
}

/********** reviews mapper **********/

func mapReviews(propertyID int64, in []map[string]any) []domain.Review {
	out := make([]domain.Review, 0, len(in))
	for _, r := range in {
		var rv domain.Review
		rv.PropertyID = propertyID

		// Author → prefer single field; fallback to first + last.
		if s := firstNonEmptyAlias(r, reviewAliases, "author"); s != nil {
			rv.Author = s
		} else {
			first := firstNonEmptyAlias(r, reviewAliases, "author_first")
			last := firstNonEmptyAlias(r, reviewAliases, "author_last")
			if first != nil || last != nil {
				full := strings.TrimSpace(joinNonEmpty(deref(first), deref(last)))
				if full != "" {
					rv.Author = &full
				}
			}
		}

		// Title
		if s := firstNonEmptyAlias(r, reviewAliases, "title"); s != nil {
			rv.Title = s
		}

		// Text → fallback compose from pros/cons.
		if s := firstNonEmptyAlias(r, reviewAliases, "text"); s != nil {
			rv.Text = s
		} else {
			pros, cons := lookupStr(r, "pros"), lookupStr(r, "cons")
			if pros != "" || cons != "" {
				joined := strings.TrimSpace(strings.Join([]string{
					strings.TrimSpace("Pros: " + pros),
					strings.TrimSpace("Cons: " + cons),
				}, "\n"))
				if joined != "" {
					rv.Text = &joined
				}
			}
		}

		// Lang
		if s := firstNonEmptyAlias(r, reviewAliases, "lang"); s != nil {
			rv.Lang = s
		}

		// Rating
		if f := getFloatFlexible(r, reviewAliases["rating"]...); f != nil {
			rv.Rating = f
		}

		// Source
		if s := firstNonEmptyAlias(r, reviewAliases, "source"); s != nil {
			rv.Source = s
		}

		// SourceID → prefer explicit; else synthesize stable hash.
		if s := firstNonEmptyAlias(r, reviewAliases, "source_id"); s != nil {
			rv.SourceID = s
		} else {
			a, t, b, l := deref(rv.Author), deref(rv.Title), deref(rv.Text), deref(rv.Lang)
			r := ""
			if rv.Rating != nil {
				r = fmt.Sprintf("%.3f", *rv.Rating)
			}
			sig := strings.Join([]string{a, t, b, l, r}, "|")
			sum := sha1.Sum([]byte(sig))
			id := hex.EncodeToString(sum[:])
			rv.SourceID = &id
		}

		// -------- NEW: structured pros/cons into AspectsJSON --------
		{
			aspects := map[string]any{}

			// Prefer arrays if present
			prosArr := firstSliceStrings(r, "pros", "review.pros", "positives")
			consArr := firstSliceStrings(r, "cons", "review.cons", "negatives")

			// Fallback to single string keys if arrays missing
			if len(prosArr) == 0 {
				if p := strings.TrimSpace(lookupStr(r, "pros")); p != "" {
					prosArr = []string{p}
				} else if p := strings.TrimSpace(lookupStr(r, "positives")); p != "" {
					prosArr = []string{p}
				}
			}
			if len(consArr) == 0 {
				if c := strings.TrimSpace(lookupStr(r, "cons")); c != "" {
					consArr = []string{c}
				} else if c := strings.TrimSpace(lookupStr(r, "negatives")); c != "" {
					consArr = []string{c}
				}
			}

			if len(prosArr) > 0 {
				aspects["pros"] = prosArr
			}
			if len(consArr) > 0 {
				aspects["cons"] = consArr
			}
			if len(aspects) > 0 {
				if b, err := json.Marshal(aspects); err == nil {
					rv.AspectsJSON = b
				} else {
					log.Error().Err(err).Str("context", "mapReviews").Msg("marshal aspects failed")
				}
			}
		}
		// -------- END NEW --------

		// Raw
		if raw, err := json.Marshal(r); err == nil {
			rv.RawJSON = raw
		} else {
			log.Error().Err(err).Str("context", "mapReviews").Msg("marshal review failed")
		}

		out = append(out, rv)
	}
	return out
}

/********** i18n mapper **********/

func mapI18n(propertyID int64, lang string, payload map[string]any) domain.HotelI18n {
	name := deref(firstNonEmptyAlias(payload, i18nAliases, "name"))
	desc := deref(firstNonEmptyAlias(payload, i18nAliases, "description"))
	pol := deref(firstNonEmptyAlias(payload, i18nAliases, "policies"))
	addr := deref(firstNonEmptyAlias(payload, i18nAliases, "address"))

	// Build known set from alias registry (top-level only).
	known := topLevelKnownFromAliases(i18nAliases, "name", "description", "policies", "address")

	extras := make(map[string]any, 8)
	for k, v := range payload {
		if _, ok := known[k]; ok {
			continue
		}
		extras[k] = v
	}
	extrasJSON, err := json.Marshal(extras)
	if err != nil {
		log.Error().Err(err).Str("context", "mapI18n").Msg("marshal extras failed")
	}

	return domain.HotelI18n{
		PropertyID:  propertyID,
		Lang:        lang,
		Name:        ptrStr(name),
		Description: ptrStr(desc),
		Policies:    ptrStr(pol),
		Address:     ptrStr(addr),
		ExtrasJSON:  extrasJSON,
	}
}
