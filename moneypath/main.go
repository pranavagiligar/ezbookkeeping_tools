package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/joho/godotenv"
	"github.com/pranavagiligar/ezbookkeeping_tools/moneypath/client"
	"github.com/pranavagiligar/ezbookkeeping_tools/moneypath/models"
	"github.com/pranavagiligar/ezbookkeeping_tools/moneypath/web"
)

var (
	port        int
	baseURL     string
	apiToken    string
	loginName   string
	password    string
	configFile  string
	defaultSecs int
	ezClient    *client.EzClient
	mbtilesDB   *sql.DB
	mbtilesPath string
)

func init() {
	flag.IntVar(&port, "port", 8081, "Port to run MoneyPath web visualizer on")
	flag.StringVar(&baseURL, "url", "", "ezBookkeeping instance base URL (e.g. https://domain.com)")
	flag.StringVar(&apiToken, "token", "", "ezBookkeeping API Bearer Token")
	flag.StringVar(&loginName, "user", "", "ezBookkeeping username for session auth")
	flag.StringVar(&password, "pass", "", "ezBookkeeping password for session auth")
	flag.StringVar(&configFile, "config", "", "Path to .env configuration file")
	flag.IntVar(&defaultSecs, "duration", 30, "Default fixed phase animation duration in seconds")
}

func loadEnvironment() {
	// Try loading .env in moneypath directory first, then root .env
	envPaths := []string{}
	if configFile != "" {
		envPaths = append(envPaths, configFile)
	}
	envPaths = append(envPaths,
		filepath.Join("moneypath", ".env"),
		".env",
		filepath.Join("..", ".env"),
	)

	loaded := false
	for _, p := range envPaths {
		if _, err := os.Stat(p); err == nil {
			if err := godotenv.Load(p); err == nil {
				fmt.Printf("📄 Loaded environment configuration from: %s\n", p)
				loaded = true
				break
			}
		}
	}
	if !loaded {
		fmt.Println("ℹ️  No .env file found. Running in standalone mode with flags or mock support.")
	}

	if baseURL == "" {
		baseURL = os.Getenv("BASE_URL")
	}
	if apiToken == "" {
		apiToken = os.Getenv("API_TOKEN")
		if apiToken == "" {
			apiToken = os.Getenv("TOKEN")
		}
		if apiToken == "" {
			apiToken = os.Getenv("EBKTOOL_TOKEN")
		}
	}
	if loginName == "" {
		loginName = os.Getenv("LOGIN_NAME")
	}
	if password == "" {
		password = os.Getenv("PASSWORD")
	}
	if pStr := os.Getenv("PORT"); pStr != "" && port == 8081 {
		if pVal, err := strconv.Atoi(pStr); err == nil {
			port = pVal
		}
	}
	// MBTiles path for offline tiles (optional)
	if mb := os.Getenv("MBTILES_PATH"); mb != "" {
		mbtilesPath = mb
	} else {
		// default to ./tiles.mbtiles if present
		if _, err := os.Stat("tiles.mbtiles"); err == nil {
			mbtilesPath = "tiles.mbtiles"
		}
	}
}

func main() {
	flag.Parse()
	loadEnvironment()

	ezClient = client.NewEzClient(baseURL, apiToken, loginName, password)

	// Open MBTiles DB if configured
	if mbtilesPath != "" {
		if _, err := os.Stat(mbtilesPath); err == nil {
			db, err := sql.Open("sqlite", mbtilesPath)
			if err != nil {
				log.Printf("Failed to open MBTiles file %s: %v", mbtilesPath, err)
			} else {
				mbtilesDB = db
				log.Printf("Serving tiles from MBTiles: %s", mbtilesPath)
				defer mbtilesDB.Close()
			}
		}
	}

	mux := http.NewServeMux()

	// API Endpoints
	mux.HandleFunc("/api/transactions", handleTransactions)
	mux.HandleFunc("/api/mock", handleMock)
	mux.HandleFunc("/export/transaction/csv", handleExportTransactionCSV)
	mux.HandleFunc("/export/transaction/geojson", handleExportTransactionGeoJSON)
	mux.HandleFunc("/api/health", handleHealth)

	// Expose lightweight config endpoint for frontend (e.g. CARTO API key)
	mux.HandleFunc("/config", handleConfig)

	// Embedded Static Assets
	fileSystem, err := web.GetFileSystem()
	if err != nil {
		log.Fatalf("Failed to initialize embedded web filesystem: %v", err)
	}
	mux.Handle("/", http.FileServer(fileSystem))

	// Local tile server (serve files from ./tiles/{z}/{x}/{y}.png)
	mux.HandleFunc("/tiles/", handleLocalTiles)

	addr := fmt.Sprintf(":%d", port)
	banner := `
  __  __                         _____      _   _     
 |  \/  |                       |  __ \    | | | |    
 | \  / | ___  _ __   ___ _   _ | |__) |_ _| |_| |__  
 | |\/| |/ _ \| '_ \ / _ \ | | ||  ___/ _' | __| '_ \ 
 | |  | | (_) | | | |  __/ |_| || |  | (_| | |_| | | |
 |_|  |_|\___/|_| |_|\___|\__, ||_|   \__,_|\__|_| |_|
                           __/ |                      
                          |___/                       
  🗺️  ezBookkeeping Geotag Route Animator
`
	fmt.Print(banner)
	fmt.Printf("🚀 MoneyPath server is running at: http://localhost:%d\n", port)
	if baseURL != "" {
		fmt.Printf("🔗 Connected to ezBookkeeping instance: %s\n", baseURL)
	} else {
		fmt.Printf("⚡ ezBookkeeping API not configured. Please configure EZBOOKKEEPING_BASE_URL and EZBOOKKEEPING_TOKEN in .env\n")
	}
	fmt.Printf("⏱️  Default Animation Phase: %d seconds\n\n", defaultSecs)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleTransactions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	q := r.URL.Query()
	var minTime, maxTime int64
	if minStr := q.Get("min_time"); minStr != "" {
		minTime, _ = strconv.ParseInt(minStr, 10, 64)
	}
	if maxStr := q.Get("max_time"); maxStr != "" {
		maxTime, _ = strconv.ParseInt(maxStr, 10, 64)
	}

	var categoryIDs []string
	if catStr := q.Get("category_ids"); catStr != "" {
		categoryIDs = strings.Split(catStr, ",")
	}
	var accountIDs []string
	if accStr := q.Get("account_ids"); accStr != "" {
		accountIDs = strings.Split(accStr, ",")
	}

	var limit int
	if limStr := q.Get("limit"); limStr != "" {
		limit, _ = strconv.Atoi(limStr)
	}

	swapCoords := q.Get("swap_coords") == "true" || q.Get("swap_coords") == "1"

	params := models.FilterParams{
		MinTime:         minTime,
		MaxTime:         maxTime,
		CategoryIDs:     categoryIDs,
		AccountIDs:      accountIDs,
		Limit:           limit,
		SwapCoordinates: swapCoords,
	}

	// If ezBookkeeping credentials are not provided, return mock data with notice
	if ezClient.BaseURL == "" || (ezClient.APIToken == "" && (ezClient.LoginName == "" || ezClient.Password == "")) {
		mockResp := client.GenerateMockData(3000)
		mockResp.ErrorMessage = "ezBookkeeping credentials not set in .env. Displaying 3,000 synthetic GPS points."
		_ = json.NewEncoder(w).Encode(mockResp)
		return
	}

	resp, err := ezClient.FetchTransactions(params)
	if err != nil {
		w.WriteHeader(http.StatusOK) // Return JSON error for frontend to handle gracefully
		_ = json.NewEncoder(w).Encode(models.MoneyPathResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func handleMock(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	count := 3000
	if cStr := r.URL.Query().Get("count"); cStr != "" {
		if cVal, err := strconv.Atoi(cStr); err == nil && cVal > 0 {
			count = cVal
		}
	}

	resp := client.GenerateMockData(count)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"version":     "1.0.0",
		"has_api_url": ezClient.BaseURL != "",
	})
}

// handleConfig returns a small JSON payload with public configuration values
// consumed by the MoneyPath frontend (e.g. optional CARTO API key).
func handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg := map[string]interface{}{
		"carto_api_key":              os.Getenv("CARTO_API_KEY"),
		"default_animation_duration": defaultSecs,
	}
	_ = json.NewEncoder(w).Encode(cfg)
}

// export helpers
func writeCSVDownload(w http.ResponseWriter, fileName, csv string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(csv))
}

func writeGeoJSONDownload(w http.ResponseWriter, fileName, geo string) {
	w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(geo))
}

// handleExportTransactionCSV serves a single transaction as CSV
func handleExportTransactionCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	// Reuse the FetchTransactions path to obtain points (apply optional filters)
	params := models.FilterParams{}
	if minStr := q.Get("min_time"); minStr != "" {
		if v, err := strconv.ParseInt(minStr, 10, 64); err == nil {
			params.MinTime = v
		}
	}
	if maxStr := q.Get("max_time"); maxStr != "" {
		if v, err := strconv.ParseInt(maxStr, 10, 64); err == nil {
			params.MaxTime = v
		}
	}
	if q.Get("swap_coords") == "true" {
		params.SwapCoordinates = true
	}

	var resp *models.MoneyPathResponse
	if ezClient.BaseURL == "" {
		resp = client.GenerateMockData(3000)
	} else {
		rresp, err := ezClient.FetchTransactions(params)
		if err != nil {
			http.Error(w, "failed to fetch transactions", http.StatusInternalServerError)
			return
		}
		resp = rresp
	}

	// find point by id
	var found *models.MoneyPathPoint
	for i := range resp.Points {
		if resp.Points[i].ID == id {
			found = &resp.Points[i]
			break
		}
	}
	if found == nil {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}

	// Build CSV
	csv := fmt.Sprintf("ID,Timestamp,FormattedTime,Latitude,Longitude,Category,Account,SourceAccount,DestinationAccount,Amount,Currency,Comment\n%v,%v,\"%v\",%v,%v,\"%v\",\"%v\",\"%v\",\"%v\",%v,%v,\"%v\"",
		found.ID, found.Timestamp, found.FormattedTime, found.Latitude, found.Longitude,
		found.CategoryName, found.AccountName, found.SourceAccountName, found.DestinationAccountName, found.Amount, found.Currency, strings.ReplaceAll(found.Comment, "\"", "\"\""))

	writeCSVDownload(w, fmt.Sprintf("transaction_%s.csv", id), csv)
}

// handleExportTransactionGeoJSON serves a single transaction as GeoJSON
func handleExportTransactionGeoJSON(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	params := models.FilterParams{}
	if minStr := q.Get("min_time"); minStr != "" {
		if v, err := strconv.ParseInt(minStr, 10, 64); err == nil {
			params.MinTime = v
		}
	}
	if maxStr := q.Get("max_time"); maxStr != "" {
		if v, err := strconv.ParseInt(maxStr, 10, 64); err == nil {
			params.MaxTime = v
		}
	}
	if q.Get("swap_coords") == "true" {
		params.SwapCoordinates = true
	}

	var resp *models.MoneyPathResponse
	if ezClient.BaseURL == "" {
		resp = client.GenerateMockData(3000)
	} else {
		rresp, err := ezClient.FetchTransactions(params)
		if err != nil {
			http.Error(w, "failed to fetch transactions", http.StatusInternalServerError)
			return
		}
		resp = rresp
	}

	var found *models.MoneyPathPoint
	for i := range resp.Points {
		if resp.Points[i].ID == id {
			found = &resp.Points[i]
			break
		}
	}
	if found == nil {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}

	feature := map[string]interface{}{
		"type": "Feature",
		"properties": map[string]interface{}{
			"id":                 found.ID,
			"timestamp":          found.Timestamp,
			"formattedTime":      found.FormattedTime,
			"category":           found.CategoryName,
			"account":            found.AccountName,
			"sourceAccount":      found.SourceAccountName,
			"destinationAccount": found.DestinationAccountName,
			"amount":             found.Amount,
			"comment":            found.Comment,
		},
		"geometry": map[string]interface{}{
			"type":        "Point",
			"coordinates": []float64{found.Longitude, found.Latitude},
		},
	}
	fc := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": []interface{}{feature},
	}
	geoBytes, _ := json.MarshalIndent(fc, "", "  ")
	writeGeoJSONDownload(w, fmt.Sprintf("transaction_%s.geojson", id), string(geoBytes))
}

// handleLocalTiles serves tiles from a local directory structure (tiles/{z}/{x}/{y}.png)
func handleLocalTiles(w http.ResponseWriter, r *http.Request) {
	// Expected path: /tiles/{z}/{x}/{y}.png
	// Trim the prefix
	p := strings.TrimPrefix(r.URL.Path, "/tiles/")
	// Prevent path traversal
	if strings.Contains(p, "..") {
		http.Error(w, "invalid tile path", http.StatusBadRequest)
		return
	}

	// If MBTiles DB is available, try to serve from it first
	if mbtilesDB != nil {
		parts := strings.Split(p, "/")
		if len(parts) >= 3 {
			z, errZ := strconv.Atoi(parts[0])
			x, errX := strconv.Atoi(parts[1])
			// strip extension from y (e.g., y.png)
			yPart := parts[2]
			yStr := strings.TrimSuffix(yPart, filepath.Ext(yPart))
			y, errY := strconv.Atoi(yStr)
			if errZ == nil && errX == nil && errY == nil {
				// convert to TMS row if necessary (MBTiles uses TMS)
				tmsY := (1 << uint(z)) - 1 - y
				var tileData []byte
				query := "SELECT tile_data FROM tiles WHERE zoom_level=? AND tile_column=? AND tile_row=? LIMIT 1"
				row := mbtilesDB.QueryRow(query, z, x, tmsY)
				if err := row.Scan(&tileData); err == nil {
					w.Header().Set("Content-Type", "image/png")
					w.Header().Set("Cache-Control", "public, max-age=86400")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(tileData)
					return
				}
			}
		}
	}

	// Map to local file under ./tiles/ as fallback
	localPath := filepath.Join("tiles", filepath.FromSlash(p))
	if _, err := os.Stat(localPath); err != nil {
		http.NotFound(w, r)
		return
	}

	// Serve file with proper content type
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, localPath)
}
