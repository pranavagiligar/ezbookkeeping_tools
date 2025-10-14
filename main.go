package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
)

// --- Global Configuration Variables ---
var (
	baseURL   string
	loginName string
	password  string
	debugMode bool
	printMode bool
)

// --- ISO 4217 Currency Exponent Mapping ---
// Most currencies use an exponent of 2 (e.g., 100 units = 1 major unit).
// This map stores the exponent to use (e.g., USD: 2 means balance / 10^2).
// Reference: https://en.wikipedia.org/wiki/ISO_4217
var currencyExponents = map[string]int{
	"USD": 2, "EUR": 2, "GBP": 2, "JPY": 0, "CNY": 2, "INR": 2, "CAD": 2, "AUD": 2,
	"HUF": 2, "JOD": 3, "KWD": 3, "OMR": 3, // Examples of 0, 3-exponent currencies
}

type AuthResponse struct {
	Result struct {
		Token string `json:"token"`
	} `json:"result"`
}

type Account struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	ParentID                string  `json:"parentId"`
	Category                int     `json:"category"`
	Type                    int     `json:"type"`
	Icon                    string  `json:"icon"`
	Color                   string  `json:"color"`
	Currency                string  `json:"currency"`
	Balance                 float64 `json:"balance"` // This holds the balance in minor units (e.g., cents)
	Comment                 string  `json:"comment"`
	DisplayOrder            int     `json:"displayOrder"`
	IsAsset                 bool    `json:"isAsset"`
	Hidden                  bool    `json:"hidden"`
	CreditCardStatementDate int     `json:"creditCardStatementDate"`
	IsLiability             bool    `json:"isLiability"`
}

type AccountListResponse struct {
	Result  []Account `json:"result"`
	Success bool      `json:"success"`
}

// --- Initialization and Main Logic ---
func init() {
	flag.StringVar(&baseURL, "url", "", "The base URL of the API (e.g., https://domain_name)")
	flag.StringVar(&loginName, "user", "", "The login name for API authorization")
	flag.StringVar(&password, "pass", "", "The password for API authorization")
	flag.BoolVar(&debugMode, "debug", false, "Enable detailed HTTP request/response logging")
	flag.BoolVar(&printMode, "print", false, "NEW: Print the CSV data to the console")
}

func main() {
	flag.Parse()

	if baseURL == "" || loginName == "" || password == "" {
		fmt.Println("Usage: go run main.go -url <base_url> -user <username> -pass <password> [-debug] [-print]")
		flag.PrintDefaults()
		log.Fatal("🚨 Missing required flags. Please provide -url, -user, and -pass.")
	}

	fmt.Printf("Attempting login to %s as user: %s\n", baseURL, loginName)
	if debugMode {
		fmt.Println("🚀 Debug mode is ENABLED.")
	}

	// 1. Get the Bearer Token
	authToken, err := getAuthToken()
	if err != nil {
		log.Fatalf("🚨 Failed to get authentication token: %v", err)
	}
	fmt.Printf("✅ Successfully retrieved token.\n")

	// 2. Fetch the Account List
	accounts, err := fetchAccountList(authToken)
	if err != nil {
		log.Fatalf("🚨 Failed to fetch account list: %v", err)
	}

	// 3. Separate and Process Accounts
	var assets []Account
	var liabilities []Account
	for _, account := range accounts {
		if account.IsAsset {
			assets = append(assets, account)
		} else if account.IsLiability {
			liabilities = append(liabilities, account)
		}
	}

	// 4. Export to CSV (and print if -print is enabled)
	exportToCSV("assets.csv", assets)
	exportToCSV("liabilities.csv", liabilities)

	fmt.Printf("✅ Export complete. Created %s and %s\n", "assets.csv", "liabilities.csv")
}

// --- Utility Functions ---

// convertBalance adjusts the balance from minor units (e.g., cents) to major units (e.g., dollars).
func convertBalance(balance float64, currency string) string {
	// Look up the exponent for the currency, default to 2 (common for USD, EUR)
	exp, ok := currencyExponents[strings.ToUpper(currency)]
	if !ok {
		// Use 2 as a safe default for unknown currencies
		exp = 2
	}

	// Apply the exponent: balance / 10^exponent
	divisor := math.Pow(10, float64(exp))
	majorUnitBalance := balance / divisor

	// Format the float into a string with the correct precision based on the exponent
	return fmt.Sprintf("%.*f", exp, majorUnitBalance)
}

// exportToCSV generates and saves the CSV file, and optionally prints to console
func exportToCSV(filename string, accounts []Account) {
	// Prepare the CSV content in memory first
	var csvData [][]string

	// Define CSV header
	header := []string{"ID", "Name", "Currency", "Balance", "Category", "IsAsset", "IsLiability", "Comment"}
	csvData = append(csvData, header)

	// Prepare data rows
	for _, acc := range accounts {
		// IMPORTANT: Convert the balance here
		formattedBalance := convertBalance(acc.Balance, acc.Currency)

		row := []string{
			acc.ID,
			acc.Name,
			acc.Currency,
			formattedBalance,
			fmt.Sprintf("%d", acc.Category),
			fmt.Sprintf("%t", acc.IsAsset),
			fmt.Sprintf("%t", acc.IsLiability),
			acc.Comment,
		}
		csvData = append(csvData, row)
	}

	// 1. Write to File
	file, err := os.Create(filename)
	if err != nil {
		log.Printf("❌ Could not create file %s: %v", filename, err)
		return
	}
	writer := csv.NewWriter(file)

	if err := writer.WriteAll(csvData); err != nil {
		log.Printf("❌ Error writing data to %s: %v", filename, err)
	}
	writer.Flush()
	file.Close()
	fmt.Printf("📝 Successfully wrote %d records to %s\n", len(accounts), filename)

	// 2. Write to Console (if printMode is enabled)
	if printMode {
		fmt.Printf("\n--- Console Output: %s ---\n", strings.ToUpper(strings.TrimSuffix(filename, ".csv")))

		// Use a CSV writer tied to the console for alignment
		consoleWriter := csv.NewWriter(os.Stdout)
		consoleWriter.Comma = '\t' // Use tab for better console alignment

		if err := consoleWriter.WriteAll(csvData); err != nil {
			log.Printf("❌ Error printing to console: %v", err)
		}
		consoleWriter.Flush()
		fmt.Println("----------------------------------------------------------------")
	}
}

// --- HTTP Request Functions (Minimized for brevity, logic remains the same) ---

func getAuthToken() (string, error) {
	authData := map[string]string{
		"loginName": loginName,
		"password":  password,
	}
	jsonData, _ := json.Marshal(authData)
	authURL := baseURL + "/api/authorize.json"
	req, err := http.NewRequest("POST", authURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if debugMode {
		dumpRequest(req, "Auth Request")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error executing auth request: %w", err)
	}
	defer resp.Body.Close()
	if debugMode {
		dumpResponseHeaders(resp, "Auth Response")
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("authorization failed with status code: %d, response body: %s", resp.StatusCode, string(bodyBytes))
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", fmt.Errorf("error decoding auth response: %w", err)
	}

	return authResp.Result.Token, nil
}

func fetchAccountList(token string) ([]Account, error) {
	listURL := baseURL + "/api/v1/accounts/list.json?visible_only=false"
	req, err := http.NewRequest("GET", listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating list request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	if debugMode {
		dumpRequest(req, "List Request")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing list request: %w", err)
	}
	defer resp.Body.Close()

	if debugMode {
		dumpResponseHeaders(resp, "List Response")
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("account list retrieval failed with status code: %d, response body: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading list response body: %w", err)
	}

	var listResp AccountListResponse
	if err := json.Unmarshal(bodyBytes, &listResp); err != nil {
		return nil, fmt.Errorf("error decoding account list response: %w", err)
	}

	if !listResp.Success {
		return nil, fmt.Errorf("account list API returned success: false")
	}

	return listResp.Result, nil
}

// Helper functions for debugging (dumpRequest, dumpResponseHeaders) remain the same
func dumpRequest(req *http.Request, title string) {
	dump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		log.Printf("Error dumping %s: %v", title, err)
		return
	}
	fmt.Printf("\n--- DEBUG: %s Details ---\n%s\n--- END %s ---\n", title, dump, title)
}

func dumpResponseHeaders(resp *http.Response, title string) {
	fmt.Printf("\n--- DEBUG: %s Headers ---\n", title)
	fmt.Printf("Status: %s\n", resp.Status)
	for key, values := range resp.Header {
		fmt.Printf("%s: %s\n", key, values)
	}
	fmt.Printf("--- END %s Headers ---\n", title)
}
