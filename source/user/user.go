package user

type User struct {
	XMLName     xml.Name `xml:"User" json:"-"`
	ID          string   `xml:"id" json:"user_id" validate:"required"`
	Type        string   `xml:"type" json:"user_type" validate:"required,oneof=human service external"`
	Organisatie string   `xml:"organisatie" json:"organisatie" validate:"required"`
	Datum       string   `xml:"datum" json:"@timestamp" validate:"required"`
}

