package enamad

type SearchFilter struct {
	DomainAddress       string
	PersianName         string
	ProductServiceIDEnc string
	Rating              string
	ProvinceIDEnc       string
	CityIDEnc           string
}

type CaptchaResponse struct {
	CaptchaBase64 string `json:"captha"`
	CptToken      string `json:"cptToken"`
}

type ApplicantDomain struct {
	ID            int    `json:"id"`
	Code          string `json:"code"`
	DomainAddress string `json:"domain_address"`
	PersianName   string `json:"persian_name"`
	Province      string `json:"province"`
	City          string `json:"city"`
	Rating        int    `json:"rating"`
}

type DomainResponse struct {
	Count                int               `json:"count"`
	Page                 int               `json:"page"`
	Result               int               `json:"result"`
	ResultMsg            string            `json:"result_msg"`
	ApplicantDomainsList []ApplicantDomain `json:"applicantDomainsList"`
}

type TrustsealDetails struct {
	OwnerName     string `json:"owner_name"`
	Phone         string `json:"phone"`
	IsMobile      bool   `json:"is_mobile"`
	Address       string `json:"address"`
	Email         string `json:"email"`
	ResponseHours string `json:"response_hours"`
}

type BusinessInfo struct {
	DomainAddress  string            `json:"domain_address"`
	PersianName    string            `json:"persian_name"`
	Province       string            `json:"province"`
	City           string            `json:"city"`
	Rating         int               `json:"rating"`
	TrustsealURL   string            `json:"trustseal_url"`
	Trustseal      *TrustsealDetails `json:"trustseal_details,omitempty"`
	SiteMobiles    []string          `json:"site_mobiles,omitempty"`
	SitePhones     []string          `json:"site_phones,omitempty"`
	ExtractionDate string            `json:"extraction_date"` // شمسی: 1405/08/18
	ExtractionDay  string            `json:"extraction_day"`  // روز هفته: دوشنبه
	ExtractionTime string            `json:"extraction_time"` // ساعت: 10:45:00
	ExtractedAt    string            `json:"extracted_at"`    // ISO 8601
}
