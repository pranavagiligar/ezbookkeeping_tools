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
	"github.com/go-pdf/fpdf"
	"github.com/joho/godotenv"
)

// --- Global Configuration Variables ---
var (
	baseURL   string
	apiToken  string
	loginName string
	password  string
	debugMode bool
	printMode bool
	dryRun    bool

	// Email Configuration
	emailRecipient string
	emailMessage   string
	smtpHost       string
	smtpPort       int
	smtpUsername   string
	smtpPassword   string
	smtpSender     string

	// Config file
	configFile string

	// PDF Config
	pdfPassword string
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
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	ParentID                string    `json:"parentId"`
	Category                int       `json:"category"`
	Type                    int       `json:"type"`
	Icon                    string    `json:"icon"`
	Color                   string    `json:"color"`
	Currency                string    `json:"currency"`
	Balance                 float64   `json:"balance"` // This holds the balance in minor units (e.g., cents)
	Comment                 string    `json:"comment"`
	DisplayOrder            int       `json:"displayOrder"`
	IsAsset                 bool      `json:"isAsset"`
	Hidden                  bool      `json:"hidden"`
	CreditCardStatementDate int       `json:"creditCardStatementDate"`
	IsLiability             bool      `json:"isLiability"`
	SubAccounts             []Account `json:"subAccounts"`
}

type AccountListResponse struct {
	Result  []Account `json:"result"`
	Success bool      `json:"success"`
}

// --- Initialization and Main Logic ---
func init() {
	// API Flags
	flag.StringVar(&baseURL, "url", "", "The base URL of the API (e.g., https://domain_name)")
	flag.StringVar(&apiToken, "token", "", "The Bearer token for API authorization")
	flag.StringVar(&loginName, "user", "", "The login name for API authorization")
	flag.StringVar(&password, "pass", "", "The password for API authorization")
	flag.BoolVar(&debugMode, "debug", false, "Enable detailed HTTP request/response logging")
	flag.BoolVar(&printMode, "print", false, "Print CSV data to the console")
	flag.BoolVar(&dryRun, "dry-run", false, "Generate report but skip sending email")

	// Email Flags
	flag.StringVar(&emailRecipient, "email-to", "", "Recipient email address for the report.")
	flag.StringVar(&emailMessage, "email-message", "", "Custom HTML message for the email body.")
	flag.StringVar(&smtpHost, "smtp-host", "", "SMTP server host.")
	flag.IntVar(&smtpPort, "smtp-port", 587, "SMTP server port (default 587).")
	flag.StringVar(&smtpUsername, "smtp-user", "", "SMTP username.")
	flag.StringVar(&smtpPassword, "smtp-pass", "", "SMTP password.")
	flag.StringVar(&smtpSender, "smtp-from", "", "Sender email address (must match SMTP user for some servers).")

	// Config file (optional)
	flag.StringVar(&configFile, "config", ".env", "Path to configuration file (default .env)")

	// PDF Flags
	flag.StringVar(&pdfPassword, "pdf-password", "", "Password to encrypt the PDF report.")
}

func main() {
	flag.Parse()

	// Load from config file (.env) if command-line args are missing
	if baseURL == "" || (apiToken == "" && (loginName == "" || password == "")) {
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
			if emailRecipient == "" {
				emailRecipient = os.Getenv("EMAIL_TO")
			}
			if emailMessage == "" {
				emailMessage = os.Getenv("EMAIL_MESSAGE")
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
			if pdfPassword == "" {
				pdfPassword = os.Getenv("PDF_PASSWORD")
			}
		} else {
			log.Println("⚠️ No .env file found, using only command-line arguments")
		}
	}

	baseURL = strings.TrimRight(baseURL, "/")

	// Validate essential config
	if baseURL == "" || (apiToken == "" && (loginName == "" || password == "")) {
		fmt.Println("Usage: go run main.go -url <base_url> [-token <bearer_token> | -user <username> -pass <password>] [email flags...]")
		flag.PrintDefaults()
		log.Fatal("🚨 Missing required API flags or .env values: -url and (-token OR -user/-pass)")
	}

	var authToken string
	if apiToken != "" {
		fmt.Printf("🔑 Using Bearer token for API authorization with %s\n", baseURL)
		authToken = apiToken
	} else {
		fmt.Printf("Attempting login to %s as user: %s\n", baseURL, loginName)

		// 1. Get the Bearer Token
		token, err := getAuthToken()
		if err != nil {
			log.Fatalf("🚨 Failed to get authentication token: %v", err)
		}
		authToken = token
		fmt.Printf("✅ Successfully retrieved token.\n")
	}

	// 2. Fetch the Account List
	accounts, err := fetchAccountList(authToken)
	if err != nil {
		log.Fatalf("🚨 Failed to fetch account list: %v", err)
	}

	// Flatten the accounts list to include subaccounts, hiding parents
	accounts = flattenAndFormatAccounts(accounts, "")

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

	// 4. Generate Reports (CSV)
	exportToCSV("assets.csv", assets)
	exportToCSV("liabilities.csv", liabilities)

	// 5. Generate PDF Report
	pdfFilename, err := generatePDFReport(assets, liabilities)
	if err != nil {
		log.Fatalf("🚨 Failed to generate PDF report: %v", err)
	}
	fmt.Printf("📄 Successfully generated PDF report: %s\n", pdfFilename)

	// 6. Send Email (if required flags are present)
	if emailRecipient != "" && smtpHost != "" && smtpUsername != "" {
		if dryRun {
			fmt.Println("🏃 Dry Run: Email sending skipped.")
		} else {
			err = sendReportEmail(pdfFilename)
			if err != nil {
				log.Fatalf("🚨 Failed to send email: %v", err)
			}
			fmt.Printf("✅ Email report successfully sent to %s\n", emailRecipient)
		}
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
// sendReportEmail configures and sends the email using gomail with the PDF attachment.
func sendReportEmail(attachmentPath string) error {
	sender := smtpSender
	if sender == "" {
		sender = smtpUsername // Default to using username as sender if not specified
	}

	m := gomail.NewMessage()
	m.SetHeader("From", sender)

	// Support multiple recipients split by comma
	recipients := strings.Split(emailRecipient, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}
	m.SetHeader("To", recipients...)

	m.SetHeader("Subject", "Financial Account Balance Report (PDF)")

	body := emailMessage
	if body == "" {
		body = "Please find the attached Financial Account Balance Report."
	}
	m.SetBody("text/html", body)
	m.Attach(attachmentPath)

	d := gomail.NewDialer(smtpHost, smtpPort, smtpUsername, smtpPassword)

	return d.DialAndSend(m)
}

// generatePDFReport creates a password-protected PDF report.
func generatePDFReport(assets, liabilities []Account) (string, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Arial", "", 12)

	// Enable protection if password is provided
	if pdfPassword != "" {
		pdf.SetProtection(fpdf.CnProtectPrint, pdfPassword, "")
	}

	pdf.AddPage()

	// Title
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, "Financial Account Summary")
	pdf.Ln(12)

	// Report Date
	pdf.SetFont("Arial", "", 10)
	pdf.Cell(0, 10, fmt.Sprintf("Report generated on: %s", time.Now().Format("2006-01-02 15:04:05 MST")))
	pdf.Ln(12)

	// Summary Section
	// We combine assets and liabilities to calculate total net worth per currency
	allAccounts := append(append([]Account{}, assets...), liabilities...)
	totals := calculateTotalBalances(allAccounts)

	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(0, 10, "  Summary", "0", 1, "L", true, 0, "")
	pdf.Ln(5)

	pdf.SetFont("Arial", "", 12)
	for currency, summary := range totals {
		// Asset Line
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(40, 8, "Total Assets:")
		pdf.SetFont("Arial", "", 12)
		pdf.SetTextColor(39, 174, 96) // Green
		pdf.Cell(0, 8, fmt.Sprintf("%s %.2f", currency, summary.TotalAsset))
		pdf.Ln(6)

		// Liability Line
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(40, 8, "Total Liabilities:")
		pdf.SetFont("Arial", "", 12)
		pdf.SetTextColor(192, 57, 43) // Red
		pdf.Cell(0, 8, fmt.Sprintf("%s %.2f", currency, summary.TotalLiability))
		pdf.Ln(6)

		// Net Worth Line
		net := summary.TotalAsset + summary.TotalLiability
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(40, 8, "Net Worth:")
		pdf.SetFont("Arial", "B", 12)
		if net >= 0 {
			pdf.SetTextColor(39, 174, 96) // Green
		} else {
			pdf.SetTextColor(192, 57, 43) // Red
		}
		pdf.Cell(0, 8, fmt.Sprintf("%s %.2f", currency, net))
		pdf.Ln(12)
	}

	pdf.SetTextColor(0, 0, 0) // Reset text color

	// Assets Table
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(0, 10, "Assets")
	pdf.Ln(10)
	drawPDFTable(pdf, assets)
	pdf.Ln(10)

	// Liabilities Table
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(0, 10, "Liabilities")
	pdf.Ln(10)
	drawPDFTable(pdf, liabilities)

	// Save to file
	filename := fmt.Sprintf("Financial_Report_%s.pdf", time.Now().Format("2006-01-02"))
	err := pdf.OutputFileAndClose(filename)
	return filename, err
}

func drawPDFTable(pdf *fpdf.Fpdf, accounts []Account) {
	// Header
	pdf.SetFont("Arial", "B", 10)
	pdf.SetFillColor(41, 128, 185)  // Blue Header
	pdf.SetTextColor(255, 255, 255) // White Text

	// Widths: Name(60), Curr(20), Bal(30), Cat(35), Com(45) = 190
	pdf.CellFormat(60, 10, "Name", "1", 0, "", true, 0, "")
	pdf.CellFormat(20, 10, "Cur", "1", 0, "", true, 0, "")
	pdf.CellFormat(30, 10, "Balance", "1", 0, "R", true, 0, "")
	pdf.CellFormat(35, 10, "Category", "1", 0, "", true, 0, "")
	pdf.CellFormat(45, 10, "Comment", "1", 0, "", true, 0, "")
	pdf.Ln(-1)

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(0, 0, 0) // Reset text color

	for i, acc := range accounts {
		// Zebra Striping
		if i%2 == 0 {
			pdf.SetFillColor(245, 245, 245) // Very Light Gray
		} else {
			pdf.SetFillColor(255, 255, 255) // White
		}

		// Calculate row height based on wrapping text
		lineHt := 6.0

		nameLines := pdf.SplitLines([]byte(acc.Name), 60)
		commentLines := pdf.SplitLines([]byte(acc.Comment), 45)

		nLines := len(nameLines)
		if len(commentLines) > nLines {
			nLines = len(commentLines)
		}
		if nLines < 1 {
			nLines = 1
		}

		rowHeight := float64(nLines) * lineHt

		// Page break check
		_, pageHeight := pdf.GetPageSize()
		_, _, _, botMargin := pdf.GetMargins()
		if pdf.GetY()+rowHeight > pageHeight-botMargin {
			pdf.AddPage()
			// Re-print header
			pdf.SetFont("Arial", "B", 10)
			pdf.SetFillColor(41, 128, 185)  // Blue Header
			pdf.SetTextColor(255, 255, 255) // White Text

			pdf.CellFormat(60, 10, "Name", "1", 0, "", true, 0, "")
			pdf.CellFormat(20, 10, "Cur", "1", 0, "", true, 0, "")
			pdf.CellFormat(30, 10, "Balance", "1", 0, "R", true, 0, "")
			pdf.CellFormat(35, 10, "Category", "1", 0, "", true, 0, "")
			pdf.CellFormat(45, 10, "Comment", "1", 0, "", true, 0, "")
			pdf.Ln(-1)

			pdf.SetFont("Arial", "", 10)
			pdf.SetTextColor(0, 0, 0)

			// Reset fill for the new page row
			if i%2 == 0 {
				pdf.SetFillColor(245, 245, 245)
			} else {
				pdf.SetFillColor(255, 255, 255)
			}
		}

		x := pdf.GetX()
		y := pdf.GetY()

		// Determine Fill
		fill := true // Always fill to show zebra striping

		// Name (MultiCell)
		pdf.SetXY(x, y)
		pdf.MultiCell(60, lineHt, acc.Name, "1", "L", fill)

		// Currency
		pdf.SetXY(x+60, y)
		pdf.CellFormat(20, rowHeight, acc.Currency, "1", 0, "C", fill, 0, "")

		// Balance (with color logic)
		formattedBalance := convertBalance(acc.Balance, acc.Currency)

		if acc.Balance >= 0 {
			pdf.SetTextColor(39, 174, 96) // Green
		} else {
			pdf.SetTextColor(192, 57, 43) // Red
		}

		pdf.SetXY(x+80, y)
		pdf.CellFormat(30, rowHeight, formattedBalance, "1", 0, "R", fill, 0, "")
		pdf.SetTextColor(0, 0, 0) // Reset to black

		// Category
		category := AccountCategory(acc.Category).String()
		pdf.SetXY(x+110, y)
		pdf.CellFormat(35, rowHeight, category, "1", 0, "L", fill, 0, "")

		// Comment (MultiCell)
		pdf.SetXY(x+145, y)
		pdf.MultiCell(45, lineHt, acc.Comment, "1", "L", fill)

		// Move to next row position
		pdf.SetXY(x, y+rowHeight)
	}
}

// TotalSummary holds the total balance for assets and liabilities separately.
type TotalSummary struct {
	TotalAsset     float64
	TotalLiability float64
}

// calculateTotalBalances sums the balances of accounts, grouped by currency.
func calculateTotalBalances(accounts []Account) map[string]TotalSummary {
	totals := make(map[string]TotalSummary)
	for _, acc := range accounts {
		exp, ok := currencyExponents[strings.ToUpper(acc.Currency)]
		if !ok {
			exp = 2
		}
		divisor := math.Pow(10, float64(exp))
		majorUnitBalance := acc.Balance / divisor

		s := totals[acc.Currency]
		// In ezBookkeeping:
		// Asset accounts usually have +ve balance.
		// Liability accounts usually have -ve balance (credit card debt etc).
		// We want to track them by their absolute magnitude logic or just sum natural values?
		// The previous logic for net worth was: net = totalAsset + totalLiability.
		// If Liability is -ve number, this is correct (Asset - Debt).
		// Let's just sum them naturally into the struct to be safe.
		if acc.IsAsset {
			s.TotalAsset += majorUnitBalance
		} else if acc.IsLiability {
			s.TotalLiability += majorUnitBalance
		} else {
			// Fallback if neither flag is strictly set (though they should be partitioned)
			if majorUnitBalance >= 0 {
				s.TotalAsset += majorUnitBalance
			} else {
				s.TotalLiability += majorUnitBalance
			}
		}
		totals[acc.Currency] = s
	}
	return totals
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

// flattenAndFormatAccounts takes a list of accounts and returns a flat list of leaf accounts.
// Parent accounts are excluded. Leaf accounts are renamed to "Name (ParentName)".
// Accounts with zero balance are excluded.
func flattenAndFormatAccounts(accounts []Account, parentName string) []Account {
	var flat []Account
	for _, acc := range accounts {
		if len(acc.SubAccounts) > 0 {
			// It is a parent account. Do not add it to the list.
			// Recursively process its children, passing this account's name as the parent.
			flat = append(flat, flattenAndFormatAccounts(acc.SubAccounts, acc.Name)...)
		} else {
			// It is a leaf account (no subaccounts).
			// Skip if balance is zero
			if acc.Balance == 0 {
				continue
			}

			if parentName != "" {
				acc.Name = fmt.Sprintf("%s (%s)", acc.Name, parentName)
			}
			flat = append(flat, acc)
		}
	}
	return flat
}
