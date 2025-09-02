package domain

type Review struct {
	ID          int64
	PropertyID  int64
	SourceID    *string
	Author      *string
	Rating      *float64
	Lang        *string
	Title       *string
	Text        *string
	AspectsJSON []byte // {"pros":[...],"cons":[...]} — optional
	Source      *string
	RawJSON     []byte
}
