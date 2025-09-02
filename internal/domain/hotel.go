package domain

type Hotel struct {
	ID         int64
	BrandID    *int64
	Stars      *int
	Lat, Lon   *float64
	Country    *string
	City       *string
	AddressRaw *string
	Amenities  []string
	Images     []string
	RawJSON    []byte // full Cupid property payload
}

type HotelI18n struct {
	PropertyID  int64
	Lang        string // en|fr|es
	Name        *string
	Description *string
	Policies    *string
	Address     *string
	ExtrasJSON  []byte // full localized payload for future fields
}
