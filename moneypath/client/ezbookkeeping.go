package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pranavagiligar/ezbookkeeping_tools/moneypath/models"
)

type EzClient struct {
	BaseURL    string
	APIToken   string
	LoginName  string
	Password   string
	HTTPClient *http.Client
}

func NewEzClient(baseURL, apiToken, loginName, password string) *EzClient {
	return &EzClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIToken:   apiToken,
		LoginName:  loginName,
		Password:   password,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// EnsureAuthToken verifies or logs in to retrieve an API token if needed
func (c *EzClient) EnsureAuthToken() (string, error) {
	if c.APIToken != "" {
		return c.APIToken, nil
	}

	if c.LoginName == "" || c.Password == "" {
		return "", fmt.Errorf("no API token or username/password configured")
	}

	// Try both /api/v1/authorize.json and /api/authorize.json
	authEndpoints := []string{
		fmt.Sprintf("%s/api/v1/authorize.json", c.BaseURL),
		fmt.Sprintf("%s/api/authorize.json", c.BaseURL),
	}

	loginData := map[string]string{
		"loginName": c.LoginName,
		"password":  c.Password,
	}
	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth payload: %w", err)
	}

	var lastErr error
	for _, authURL := range authEndpoints {
		req, err := http.NewRequest("POST", authURL, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			continue // try next endpoint
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("auth API returned status %d: %s", resp.StatusCode, string(body))
			continue
		}

		var authResp struct {
			Success      bool   `json:"success"`
			ErrorCode    int    `json:"errorCode"`
			ErrorMessage string `json:"errorMessage"`
			Result       struct {
				Token string `json:"token"`
			} `json:"result"`
		}

		if err := json.Unmarshal(body, &authResp); err != nil {
			lastErr = fmt.Errorf("failed to parse auth response: %w", err)
			continue
		}

		if !authResp.Success || authResp.Result.Token == "" {
			lastErr = fmt.Errorf("login failed: %s (code: %d)", authResp.ErrorMessage, authResp.ErrorCode)
			continue
		}

		c.APIToken = authResp.Result.Token
		return c.APIToken, nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("failed to authenticate with ezBookkeeping")
}

// FetchTransactions queries ezBookkeeping with pagination and extracts geotagged points
func (c *EzClient) FetchTransactions(params models.FilterParams) (*models.MoneyPathResponse, error) {
	token, err := c.EnsureAuthToken()
	if err != nil {
		return nil, fmt.Errorf("authentication error: %w", err)
	}

	// Apply server-side filtering when supported by the caller; keep the raw response
	// available for UI-level refinement (type toggles, tag filters, text search).
	if params.DescriptionContains != "" || len(params.Types) > 0 || len(params.CategoryNames) > 0 || len(params.TagNames) > 0 {
		// Request is satisfied by the helper below, but pagination remains unchanged.
	}

	var allItems []models.EzTransaction
	page := 1
	pageSize := 50
	maxPages := 100 // Fetch up to 5000 records

	// Determine client timezone offset in minutes (e.g. 330 for IST)
	_, offsetSec := time.Now().Zone()
	offsetMin := offsetSec / 60
	if offsetMin == 0 {
		offsetMin = 330 // Default to IST offset if local is UTC
	}

	// Try /api/v1/ first, then fallback to /api/
	apiBasePaths := []string{
		fmt.Sprintf("%s/api/v1/transactions/list.json", c.BaseURL),
		fmt.Sprintf("%s/api/transactions/list.json", c.BaseURL),
	}

	activeBasePath := apiBasePaths[0]

	for page <= maxPages {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("count", strconv.Itoa(pageSize))
		q.Set("with_pictures", "true")
		q.Set("trim_account", "false")
		q.Set("trim_category", "false")
		q.Set("trim_tag", "false")

		if params.MinTime > 0 {
			q.Set("min_time", strconv.FormatInt(params.MinTime, 10))
		}
		if params.MaxTime > 0 {
			maxTimeMs := params.MaxTime
			if maxTimeMs < 100000000000 { // Convert seconds to milliseconds for ezBookkeeping API
				maxTimeMs = maxTimeMs*1000 + 999
			}
			q.Set("max_time", strconv.FormatInt(maxTimeMs, 10))
		}
		if len(params.CategoryIDs) > 0 {
			q.Set("category_ids", strings.Join(params.CategoryIDs, ","))
		}
		if len(params.AccountIDs) > 0 {
			q.Set("account_ids", strings.Join(params.AccountIDs, ","))
		}

		apiPath := fmt.Sprintf("%s?%s", activeBasePath, q.Encode())
		req, err := http.NewRequest("GET", apiPath, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build transaction request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Timezone-Offset", strconv.Itoa(offsetMin))

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("transaction API request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		// If 404 on /api/v1, try fallback to /api/ on page 1
		if resp.StatusCode == http.StatusNotFound && page == 1 && activeBasePath == apiBasePaths[0] {
			activeBasePath = apiBasePaths[1]
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("transaction API returned status %d: %s", resp.StatusCode, string(body))
		}

		var pageResult struct {
			Success      bool   `json:"success"`
			ErrorCode    int    `json:"errorCode"`
			ErrorMessage string `json:"errorMessage"`
			Result       struct {
				Items      []models.EzTransaction `json:"items"`
				TotalCount int                    `json:"totalCount"`
				NextTime   int64                  `json:"nextTime"`
			} `json:"result"`
		}

		if err := json.Unmarshal(body, &pageResult); err != nil {
			return nil, fmt.Errorf("failed to parse transactions JSON: %w", err)
		}

		if !pageResult.Success {
			return nil, fmt.Errorf("ezBookkeeping error: %s (code %d)", pageResult.ErrorMessage, pageResult.ErrorCode)
		}

		items := pageResult.Result.Items
		if len(items) == 0 {
			break
		}

		allItems = append(allItems, items...)

		if len(items) < pageSize || (params.Limit > 0 && len(allItems) >= params.Limit) {
			break
		}

		// Items are in descending time order; if oldest item in batch is older than MinTime, stop pagination
		if params.MinTime > 0 && items[len(items)-1].Time < params.MinTime {
			break
		}

		page++
	}

	filteredItems := applyTransactionFilters(allItems, params)
	return ProcessTransactions(filteredItems, false, params.SwapCoordinates, params.MinTime, params.MaxTime), nil
}

// ProcessTransactions transforms raw ezBookkeeping transactions into sequential MoneyPathPoints
func applyTransactionFilters(items []models.EzTransaction, params models.FilterParams) []models.EzTransaction {
	if len(items) == 0 {
		return items
	}

	allowedTypes := map[int]bool{}
	for _, t := range params.Types {
		allowedTypes[t] = true
	}

	allowedCategories := map[string]bool{}
	for _, name := range params.CategoryNames {
		n := strings.TrimSpace(strings.ToLower(name))
		if n != "" {
			allowedCategories[n] = true
		}
	}

	allowedTags := map[string]bool{}
	for _, name := range params.TagNames {
		n := strings.TrimSpace(strings.ToLower(name))
		if n != "" {
			allowedTags[n] = true
		}
	}

	keyword := strings.TrimSpace(strings.ToLower(params.Keyword))
	if params.DescriptionContains != "" {
		keyword = strings.TrimSpace(strings.ToLower(params.DescriptionContains))
	}

	filtered := make([]models.EzTransaction, 0, len(items))
	for _, item := range items {
		if len(allowedTypes) > 0 && !allowedTypes[item.Type] {
			continue
		}
		if len(allowedCategories) > 0 {
			catName := ""
			if item.Category != nil {
				catName = strings.TrimSpace(strings.ToLower(item.Category.Name))
			}
			if !allowedCategories[catName] {
				continue
			}
		}
		if len(allowedTags) > 0 {
			matched := false
			for _, tag := range item.Tags {
				if allowedTags[strings.TrimSpace(strings.ToLower(tag.Name))] {
					matched = true
					break
				}
			}
			for _, tagID := range item.TagIDs {
				if allowedTags[strings.TrimSpace(strings.ToLower(tagID))] {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if keyword != "" {
			desc := strings.TrimSpace(strings.ToLower(item.Comment))
			if !strings.Contains(desc, keyword) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func ProcessTransactions(items []models.EzTransaction, isMock bool, swapCoordinates bool, minTime, maxTime int64) *models.MoneyPathResponse {
	// 1. Filter items that contain valid GeoLocation
	type validGeoItem struct {
		item models.EzTransaction
		lat  float64
		lon  float64
	}

	var geoList []validGeoItem

	for _, item := range items {
		if minTime > 0 && item.Time < minTime {
			continue
		}
		if maxTime > 0 && item.Time > maxTime {
			continue
		}

		if item.GeoLocation != nil {
			lat := item.GeoLocation.Latitude
			lon := item.GeoLocation.Longitude

			// If user requested swap, or if lat is outside [-90, 90] bounds (e.g. lon was put in lat)
			if swapCoordinates || math.Abs(lat) > 90.0 {
				lat, lon = lon, lat
			}

			if (lat != 0 || lon != 0) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 {
				geoList = append(geoList, validGeoItem{
					item: item,
					lat:  lat,
					lon:  lon,
				})
			}
		}
	}

	// 2. Sort chronologically by timestamp (oldest first for forward animation)
	sort.Slice(geoList, func(i, j int) bool {
		if geoList[i].item.Time == geoList[j].item.Time {
			return geoList[i].item.TimeSequenceID < geoList[j].item.TimeSequenceID
		}
		return geoList[i].item.Time < geoList[j].item.Time
	})

	points := make([]models.MoneyPathPoint, 0, len(geoList))
	catMap := make(map[string]bool)
	accMap := make(map[string]bool)

	var totalDistanceM float64
	var totalExpense float64
	var totalIncome float64
	var primaryCurrency string = "INR"
	var minTs, maxTs int64

	for i, entry := range geoList {
		item := entry.item
		lat := entry.lat
		lon := entry.lon

		var distM float64
		if i > 0 {
			prevLat := geoList[i-1].lat
			prevLon := geoList[i-1].lon
			distM = Haversine(prevLat, prevLon, lat, lon)
			totalDistanceM += distM
		}

		// Determine type name
		typeName := "Expense"
		switch item.Type {
		case models.TransactionTypeModifyBalance:
			typeName = "Balance Modify"
		case models.TransactionTypeIncome:
			typeName = "Income"
		case models.TransactionTypeExpense:
			typeName = "Expense"
		case models.TransactionTypeTransfer:
			typeName = "Transfer"
		}

		// Amount calculations:
		// Minor units: 100 paise = 1 INR (amount = rawAmount / 100.0)
		var currency string = "INR"
		if item.SourceAccount != nil && item.SourceAccount.Currency != "" {
			currency = item.SourceAccount.Currency
		} else if item.DestinationAccount != nil && item.DestinationAccount.Currency != "" {
			currency = item.DestinationAccount.Currency
		}
		primaryCurrency = currency

		var amount float64
		if item.Type == models.TransactionTypeIncome && item.DestinationAmount > 0 {
			amount = float64(item.DestinationAmount) / 100.0
			totalIncome += amount
		} else {
			amount = float64(item.SourceAmount) / 100.0
			if item.Type == models.TransactionTypeExpense {
				totalExpense += amount
			}
		}

		// Category & Account metadata
		categoryName := "Uncategorized"
		categoryColor := "#888888"
		categoryIcon := "receipt"
		if item.Category != nil {
			if item.Category.Name != "" {
				categoryName = item.Category.Name
			}
			if item.Category.Color != "" {
				if !strings.HasPrefix(item.Category.Color, "#") {
					categoryColor = "#" + item.Category.Color
				} else {
					categoryColor = item.Category.Color
				}
			}
			if item.Category.Icon != "" {
				categoryIcon = item.Category.Icon
			}
		}
		catMap[categoryName] = true

		accountName := "General"
		if item.SourceAccount != nil && item.SourceAccount.Name != "" {
			accountName = item.SourceAccount.Name
		} else if item.DestinationAccount != nil && item.DestinationAccount.Name != "" {
			accountName = item.DestinationAccount.Name
		}
		accMap[accountName] = true

		var tags []string
		for _, t := range item.Tags {
			tags = append(tags, t.Name)
		}

		var pics []string
		for _, p := range item.Pictures {
			if p.OriginalURL != "" {
				pics = append(pics, p.OriginalURL)
			}
		}

		if minTs == 0 || item.Time < minTs {
			minTs = item.Time
		}
		if item.Time > maxTs {
			maxTs = item.Time
		}

		sourceAccName := ""
		destAccName := ""
		if item.SourceAccount != nil && item.SourceAccount.Name != "" {
			sourceAccName = item.SourceAccount.Name
		}
		if item.DestinationAccount != nil && item.DestinationAccount.Name != "" {
			destAccName = item.DestinationAccount.Name
		}

		points = append(points, models.MoneyPathPoint{
			ID:                     item.ID,
			SequenceIndex:          i,
			Timestamp:              item.Time,
			FormattedTime:          models.FormatTimestamp(item.Time, item.UTCOffset),
			Latitude:               lat,
			Longitude:              lon,
			Type:                   item.Type,
			TypeName:               typeName,
			CategoryName:           categoryName,
			CategoryColor:          categoryColor,
			CategoryIcon:           categoryIcon,
			AccountName:            accountName,
			SourceAccountName:      sourceAccName,
			DestinationAccountName: destAccName,
			Currency:               currency,
			Amount:                 amount,
			RawAmount:              item.SourceAmount,
			Comment:                item.Comment,
			Tags:                   tags,
			Pictures:               pics,
			DistancePrevM:          math.Round(distM*100) / 100,
		})
	}

	var categories []string
	for c := range catMap {
		categories = append(categories, c)
	}
	sort.Strings(categories)

	var accounts []string
	for a := range accMap {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)

	return &models.MoneyPathResponse{
		Success:         true,
		TotalFetched:    len(items),
		TotalWithGeo:    len(points),
		TotalDistanceKm: math.Round((totalDistanceM/1000.0)*100) / 100,
		TotalExpense:    math.Round(totalExpense*100) / 100,
		TotalIncome:     math.Round(totalIncome*100) / 100,
		PrimaryCurrency: primaryCurrency,
		MinTimestamp:    minTs,
		MaxTimestamp:    maxTs,
		Points:          points,
		Categories:      categories,
		Accounts:        accounts,
		IsMock:          isMock,
	}
}

// GenerateMockData generates 3000 realistic sequential geotagged transactions centered in Bangalore
func GenerateMockData(count int) *models.MoneyPathResponse {
	if count <= 0 {
		count = 3000
	}

	r := rand.New(rand.NewSource(42)) // deterministic seed for reproducibility

	// Base coordinates in Bangalore (MG Road / Indiranagar / Koramangala / Jayanagar area)
	baseLat := 12.9716
	baseLon := 77.5946

	categories := []struct {
		Name  string
		Color string
		Icon  string
	}{
		{"Dining & Food", "#EF4444", "utensils"},
		{"Groceries", "#F59E0B", "shopping-cart"},
		{"Transportation", "#3B82F6", "car"},
		{"Coffee & Cafes", "#10B981", "coffee"},
		{"Entertainment", "#8B5CF6", "film"},
		{"Shopping", "#EC4899", "shopping-bag"},
		{"Fuel & Gas", "#6366F1", "gas-pump"},
		{"Rent & Home", "#14B8A6", "home"},
	}

	accounts := []string{"HDFC Bank", "SBI Savings", "Cash Wallet", "UPI Pay"}

	sampleComments := []string{
		"Breakfast: Idli Vada & Filter Coffee at CTR",
		"Namma Metro ride from MG Road to Indiranagar",
		"Supermarket grocery run at Nature's Basket",
		"Lunch with team at Koramangala bistro",
		"Auto rickshaw fare to Jayanagar 4th Block",
		"Fuel fill-up at HP Petrol Bunk",
		"Dinner: Butter Masala Dosa at Vidyarthi Bhavan",
		"Pharmacy essentials purchase",
		"Cinema tickets & snacks at PVR Forum Mall",
		"Silk saree boutique purchase",
		"Cab ride to Kempegowda International Airport",
		"Co-working desk pass at Indiranagar",
	}

	now := time.Now().Unix()
	startTime := now - int64(count*300) // approx 5 mins between points over span

	items := make([]models.EzTransaction, 0, count)

	currLat := baseLat
	currLon := baseLon
	heading := r.Float64() * 2 * math.Pi // initial direction

	for i := 0; i < count; i++ {
		// Realistic wandering random walk with directional persistence
		heading += (r.Float64() - 0.5) * 0.4 // gentle direction changes
		stepDistKm := 0.05 + r.Float64()*0.4 // 50m to 450m jumps

		// 1 degree lat is ~111km, 1 degree lon is ~111 * cos(lat) km
		dLat := (stepDistKm / 111.0) * math.Cos(heading)
		dLon := (stepDistKm / (111.0 * math.Cos(currLat*math.Pi/180.0))) * math.Sin(heading)

		currLat += dLat
		currLon += dLon

		// Keep in reasonable Bangalore bounding box
		if currLat < 12.75 || currLat > 13.20 {
			currLat = baseLat + (r.Float64()-0.5)*0.05
		}
		if currLon < 77.40 || currLon > 77.80 {
			currLon = baseLon + (r.Float64()-0.5)*0.05
		}

		cat := categories[r.Intn(len(categories))]
		acc := accounts[r.Intn(len(accounts))]
		comment := sampleComments[r.Intn(len(sampleComments))]
		amountPaise := int64(15000 + r.Intn(450000)) // ₹150 to ₹4,650

		ts := startTime + int64(i*300) + int64(r.Intn(120))

		items = append(items, models.EzTransaction{
			ID:             fmt.Sprintf("mock_tx_%05d", i+1),
			TimeSequenceID: fmt.Sprintf("%010d_%05d", ts, i),
			Type:           models.TransactionTypeExpense,
			Time:           ts,
			UTCOffset:      330,
			SourceAccount: &models.AccountInfo{
				Name:     acc,
				Currency: "INR",
			},
			Category: &models.TransactionCategoryInfo{
				Name:  cat.Name,
				Color: cat.Color,
				Icon:  cat.Icon,
			},
			SourceAmount: amountPaise,
			Comment:      fmt.Sprintf("#%d: %s", i+1, comment),
			GeoLocation: &models.GeoLocation{
				Latitude:  currLat,
				Longitude: currLon,
			},
			Tags: []models.TransactionTag{
				{Name: "Bangalore-Route"},
			},
		})
	}

	return ProcessTransactions(items, true, false, 0, 0)
}

// Haversine calculates great-circle distance between two coordinates in meters
func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0

	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1Rad)*math.Cos(lat2Rad)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}
