package models

import "time"

// TransactionType constants from ezBookkeeping
const (
	TransactionTypeModifyBalance = 1
	TransactionTypeIncome        = 2
	TransactionTypeExpense       = 3
	TransactionTypeTransfer      = 4
)

// GeoLocation represents latitude and longitude
type GeoLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// TransactionPicture represents an attached image
type TransactionPicture struct {
	PictureID   string `json:"pictureId"`
	OriginalURL string `json:"originalUrl"`
}

// TransactionCategoryInfo represents category metadata
type TransactionCategoryInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  int    `json:"type"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

// AccountInfo represents account metadata
type AccountInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category int    `json:"category"`
	Type     int    `json:"type"`
	Currency string `json:"currency"`
	Color    string `json:"color"`
	Icon     string `json:"icon"`
}

// TransactionTag represents a tag
type TransactionTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EzTransaction is the raw transaction object returned by ezBookkeeping API
type EzTransaction struct {
	ID                   string                   `json:"id"`
	TimeSequenceID       string                   `json:"timeSequenceId"`
	Type                 int                      `json:"type"`
	CategoryID           string                   `json:"categoryId"`
	Category             *TransactionCategoryInfo `json:"category,omitempty"`
	Time                 int64                    `json:"time"`      // Unix timestamp in seconds
	UTCOffset            int                      `json:"utcOffset"` // UTC offset in minutes
	SourceAccountID      string                   `json:"sourceAccountId"`
	SourceAccount        *AccountInfo             `json:"sourceAccount,omitempty"`
	DestinationAccountID string                   `json:"destinationAccountId,omitempty"`
	DestinationAccount   *AccountInfo             `json:"destinationAccount,omitempty"`
	SourceAmount         int64                    `json:"sourceAmount"`      // In minor currency units (cents)
	DestinationAmount    int64                    `json:"destinationAmount"` // In minor currency units (cents)
	HideAmount           bool                     `json:"hideAmount"`
	TagIDs               []string                 `json:"tagIds"`
	Tags                 []TransactionTag         `json:"tags,omitempty"`
	Pictures             []TransactionPicture     `json:"pictures,omitempty"`
	Comment              string                   `json:"comment"`
	GeoLocation          *GeoLocation             `json:"geoLocation,omitempty"`
	Editable             bool                     `json:"editable"`
}

// EzPageWrapper is the standard paginated response wrapper from ezBookkeeping
type EzPageWrapper struct {
	Items      []EzTransaction `json:"items"`
	TotalCount int             `json:"totalCount"`
	NextTime   int64           `json:"nextTime"`
}

// EzAPIResponse is standard response container
type EzAPIResponse struct {
	Success      bool            `json:"success"`
	Result       jsonRawResponse `json:"result"`
	ErrorCode    int             `json:"errorCode,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
}

type jsonRawResponse struct {
	Items      []EzTransaction `json:"items"`
	TotalCount int             `json:"totalCount"`
	Token      string          `json:"token"`
}

// MoneyPathPoint is the standardized, enriched point sent to the frontend visualizer
type MoneyPathPoint struct {
	ID                     string   `json:"id"`
	SequenceIndex          int      `json:"sequenceIndex"`
	Timestamp              int64    `json:"timestamp"`
	FormattedTime          string   `json:"formattedTime"`
	Latitude               float64  `json:"latitude"`
	Longitude              float64  `json:"longitude"`
	Type                   int      `json:"type"`
	TypeName               string   `json:"typeName"`
	CategoryName           string   `json:"categoryName"`
	CategoryColor          string   `json:"categoryColor"`
	CategoryIcon           string   `json:"categoryIcon"`
	AccountName            string   `json:"accountName"`
	SourceAccountName      string   `json:"sourceAccountName,omitempty"`
	DestinationAccountName string   `json:"destinationAccountName,omitempty"`
	Currency               string   `json:"currency"`
	Amount                 float64  `json:"amount"`    // Major unit (e.g. $12.34)
	RawAmount              int64    `json:"rawAmount"` // Minor unit
	Comment                string   `json:"comment"`
	Tags                   []string `json:"tags"`
	Pictures               []string `json:"pictures"`
	DistancePrevM          float64  `json:"distancePrevM"` // Distance from previous point in meters
}

// MoneyPathResponse is the response payload sent to the Web UI
type MoneyPathResponse struct {
	Success         bool             `json:"success"`
	TotalFetched    int              `json:"totalFetched"`
	TotalWithGeo    int              `json:"totalWithGeo"`
	TotalDistanceKm float64          `json:"totalDistanceKm"`
	TotalExpense    float64          `json:"totalExpense"`
	TotalIncome     float64          `json:"totalIncome"`
	PrimaryCurrency string           `json:"primaryCurrency"`
	MinTimestamp    int64            `json:"minTimestamp"`
	MaxTimestamp    int64            `json:"maxTimestamp"`
	Points          []MoneyPathPoint `json:"points"`
	Categories      []string         `json:"categories"`
	Accounts        []string         `json:"accounts"`
	IsMock          bool             `json:"isMock,omitempty"`
	ErrorMessage    string           `json:"errorMessage,omitempty"`
}

// FilterParams holds the query parameters for filtering transactions
type FilterParams struct {
	MinTime             int64    `json:"minTime"`
	MaxTime             int64    `json:"maxTime"`
	Types               []int    `json:"types"`
	CategoryIDs         []string `json:"categoryIds"`
	CategoryNames       []string `json:"categoryNames"`
	AccountIDs          []string `json:"accountIds"`
	Keyword             string   `json:"keyword"`
	DescriptionContains string   `json:"descriptionContains"`
	TagNames            []string `json:"tagNames"`
	Limit               int      `json:"limit"`
	UseMock             bool     `json:"useMock"`
	MockCount           int      `json:"mockCount"`
	SwapCoordinates     bool     `json:"swapCoordinates"`
}

// Config represents runtime configuration loaded from .env or flags
type Config struct {
	BaseURL                  string
	APIToken                 string
	LoginName                string
	Password                 string
	Port                     int
	DefaultAnimationDuration int // in seconds
	DefaultMapStyle          string
	DefaultCurrency          string
	DefaultCurrencyExponent  int
}

// Helper time formatter
func FormatTimestamp(sec int64, offsetMin int) string {
	t := time.Unix(sec, 0).UTC().Add(time.Duration(offsetMin) * time.Minute)
	return t.Format("2006-01-02 15:04:05")
}
