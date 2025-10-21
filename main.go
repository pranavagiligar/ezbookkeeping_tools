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
	"time"

	"github.com/go-gomail/gomail"
	"github.com/joho/godotenv"
)

// --- Global Configuration Variables ---
var (
	baseURL   string
	loginName string
	password  string
	debugMode bool
	printMode bool

	// Email Configuration
	emailRecipient string
	smtpHost       string
	smtpPort       int
	smtpUsername   string
	smtpPassword   string
	smtpSender     string

	// Config file
	configFile string
)

type AccountCategory int

const (
	Cash AccountCategory = iota + 1
	CheckingAccount
	CreditCard
	VirtualAccount
	DebtAccount
	Receivables
	InvestmentAccount
	SavingsAccount
	CertificateOfDeposit
)

// String returns the human-readable name for the AccountType.
func (a AccountCategory) String() string {
	switch a {
	case Cash:
		return "Cash"
	case CheckingAccount:
		return "Checking Account"
	case CreditCard:
		return "Credit Card"
	case VirtualAccount:
		return "Virtual Account"
	case DebtAccount:
		return "Debt Account"
	case Receivables:
		return "Receivables"
	case InvestmentAccount:
		return "Investment Account"
	case SavingsAccount:
		return "Savings Account"
	case CertificateOfDeposit:
		return "Certificate of Deposit"
	default:
		return "Unknown"
	}
}

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
	// API Flags
	flag.StringVar(&baseURL, "url", "", "The base URL of the API (e.g., https://domain_name)")
	flag.StringVar(&loginName, "user", "", "The login name for API authorization")
	flag.StringVar(&password, "pass", "", "The password for API authorization")
	flag.BoolVar(&debugMode, "debug", false, "Enable detailed HTTP request/response logging")
	flag.BoolVar(&printMode, "print", false, "Print CSV data to the console")

	// Email Flags
	flag.StringVar(&emailRecipient, "email-to", "", "Recipient email address for the report.")
	flag.StringVar(&smtpHost, "smtp-host", "", "SMTP server host.")
	flag.IntVar(&smtpPort, "smtp-port", 587, "SMTP server port (default 587).")
	flag.StringVar(&smtpUsername, "smtp-user", "", "SMTP username.")
	flag.StringVar(&smtpPassword, "smtp-pass", "", "SMTP password.")
	flag.StringVar(&smtpSender, "smtp-from", "", "Sender email address (must match SMTP user for some servers).")

	// Config file (optional)
	flag.StringVar(&configFile, "config", ".env", "Path to configuration file (default .env)")
}

func main() {
	flag.Parse()

	// Load from config file (.env) if command-line args are missing
	if baseURL == "" || loginName == "" || password == "" {
		if _, err := os.Stat(configFile); err == nil {
			fmt.Printf("📄 Loading configuration from %s\n", configFile)
			err := godotenv.Load(configFile)
			if err != nil {
				log.Fatalf("❌ Failed to load config file %s: %v", configFile, err)
			}

			// Load values from env if not set by flags
			if baseURL == "" {
				baseURL = os.Getenv("BASE_URL")
			}
			if loginName == "" {
				loginName = os.Getenv("LOGIN_NAME")
			}
			if password == "" {
				password = os.Getenv("PASSWORD")
			}
			if emailRecipient == "" {
				emailRecipient = os.Getenv("EMAIL_TO")
			}
			if smtpHost == "" {
				smtpHost = os.Getenv("SMTP_HOST")
			}
			if smtpPort == 0 {
				smtpPort = envToInt("SMTP_PORT", 587)
			}
			if smtpUsername == "" {
				smtpUsername = os.Getenv("SMTP_USER")
			}
			if smtpPassword == "" {
				smtpPassword = os.Getenv("SMTP_PASS")
			}
			if smtpSender == "" {
				smtpSender = os.Getenv("SMTP_FROM")
			}
		} else {
			log.Println("⚠️ No .env file found, using only command-line arguments")
		}
	}

	// Validate essential config
	if baseURL == "" || loginName == "" || password == "" {
		fmt.Println("Usage: go run main.go -url <base_url> -user <username> -pass <password> [email flags...]")
		flag.PrintDefaults()
		log.Fatal("🚨 Missing required API flags or .env values: -url, -user, -pass")
	}

	fmt.Printf("Attempting login to %s as user: %s\n", baseURL, loginName)

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

	// 3. Separate Accounts
	var assets []Account
	var liabilities []Account
	for _, account := range accounts {
		if account.IsAsset {
			assets = append(assets, account)
		} else if account.IsLiability {
			liabilities = append(liabilities, account)
		}
	}

	// 4. Generate Reports
	exportToCSV("assets.csv", assets)
	exportToCSV("liabilities.csv", liabilities)

	htmlContent := generateHTMLReport(assets, liabilities)

	// 5. Send Email (if required flags are present)
	if emailRecipient != "" && smtpHost != "" && smtpUsername != "" {
		err = sendReportEmail(htmlContent)
		if err != nil {
			log.Fatalf("🚨 Failed to send email: %v", err)
		}
		fmt.Printf("✅ Email report successfully sent to %s\n", emailRecipient)
	} else if emailRecipient != "" {
		log.Println("⚠️ Email flags missing. Not sending email. Use -smtp-host, -smtp-user, and -email-to.")
	}
}

// --- Utility Functions ---

func envToInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	var num int
	_, err := fmt.Sscanf(val, "%d", &num)
	if err != nil {
		return defaultVal
	}
	return num
}

// --- Reporting and Email Functions ---
// sendReportEmail configures and sends the email using gomail.
func sendReportEmail(htmlBody string) error {
	sender := smtpSender
	if sender == "" {
		sender = smtpUsername // Default to using username as sender if not specified
	}

	m := gomail.NewMessage()
	m.SetHeader("From", sender)
	m.SetHeader("To", emailRecipient)
	m.SetHeader("Subject", "Financial Account Balance Report")
	m.SetBody("text/html", htmlBody)

	d := gomail.NewDialer(smtpHost, smtpPort, smtpUsername, smtpPassword)

	return d.DialAndSend(m)
}

// generateHTMLReport creates a single HTML page with two tables.
func generateHTMLReport(assets, liabilities []Account) string {
	reportTime := time.Now().Format("2006-01-02 15:04:05 MST") // Get current time

	assetTotals := calculateTotalBalances(assets)
	liabilityTotals := calculateTotalBalances(liabilities)
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("<p>Report generated on: <strong>%s</strong></p>", reportTime)) // Add report time
	summary.WriteString("<h2>Financial Summary</h2>")
	for currency, total := range assetTotals {
		liabilityTotal := liabilityTotals[currency]
		// liabilityTotal are negative. So negate it
		totalAsset := total - liabilityTotal
		netAsset := totalAsset + liabilityTotal
		summary.WriteString(fmt.Sprintf("<p><strong>Total Assets (%s):</strong> <span class=\"positive\">%.2f</span></p>", currency, totalAsset))
		summary.WriteString(fmt.Sprintf("<p><strong>Total Liabilities (%s):</strong> <span class=\"negative\">%.2f</span></p>", currency, liabilityTotal))
		summary.WriteString(fmt.Sprintf("<p><strong>Net Assets (%s):</strong> <span class=\"%s\">%.2f</span></p>", currency, getBalanceClass(netAsset), netAsset))
	}
	htmlTemplate := `
			<!DOCTYPE html>
			<html>
			<head>
			<style>
			body { font-family: Arial, sans-serif; }
			table { width: 80%%; border-collapse: collapse; margin-bottom: 20px; }
			th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
			th { background-color: #f2f2f2; }
			.positive { color: green; font-weight: bold; }
			.negative { color: red; font-weight: bold; }
			</style>
			</head>
			<body>
			<h1>Financial Account Summary</h1>
			<p>This report contains a summary of your Assets and Liabilities.</p>
			%s
			<h2>Assets</h2>
			%s

			<h2>Liabilities</h2>
			%s

			</body>
			</html>
			`
	assetTable := generateHTMLTable(assets)
	liabilityTable := generateHTMLTable(liabilities)

	return fmt.Sprintf(htmlTemplate, summary.String(), assetTable, liabilityTable)
}

// calculateTotalBalances sums the balances of accounts, grouped by currency, and returns them in major units.
func calculateTotalBalances(accounts []Account) map[string]float64 {
	totals := make(map[string]float64)
	for _, acc := range accounts {
		exp, ok := currencyExponents[strings.ToUpper(acc.Currency)]
		if !ok {
			exp = 2 // Default to 2 if currency exponent is unknown
		}
		divisor := math.Pow(10, float64(exp))
		majorUnitBalance := acc.Balance / divisor
		totals[acc.Currency] += majorUnitBalance
	}
	return totals
}

func getBalanceClass(balance float64) string {
	if balance >= 0 {
		return "positive"
	}
	return "negative"
}

// generateHTMLTable is a helper function to create the HTML table structure.
func generateHTMLTable(accounts []Account) string {
	if len(accounts) == 0 {
		return "<p>No accounts found in this category.</p>"
	}

	var table strings.Builder
	table.WriteString("<table><thead><tr><th>Name</th><th>Currency</th><th>Balance</th><th>Category</th><th>Comment</th></tr></thead><tbody>")

	for _, acc := range accounts {
		formattedBalance := convertBalance(acc.Balance, acc.Currency)
		balanceClass := "positive"
		if acc.IsLiability {
			balanceClass = "negative"
		}

		table.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td class=\"%s\">%s</td><td>%s</td><td>%s</td></tr>",
			acc.Name,
			acc.Currency,
			balanceClass,
			formattedBalance,
			AccountCategory(acc.Category).String(),
			acc.Comment,
		))
	}

	table.WriteString("</tbody></table>")
	return table.String()
}

// convertBalance adjusts the balance from minor units (e.g., cents) to major units (e.g., dollars).
func convertBalance(balance float64, currency string) string {
	exp, ok := currencyExponents[strings.ToUpper(currency)]
	if !ok {
		exp = 2
	}
	divisor := math.Pow(10, float64(exp))
	majorUnitBalance := balance / divisor
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
			AccountCategory(acc.Category).String(),
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
