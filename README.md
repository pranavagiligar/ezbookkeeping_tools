# Ezbookkeeping Account Summary Tool Usage

This tool fetches account data from the API, separates it into **assets** and **liabilities**, formats balances using **ISO 4217** standards, exports the results to CSV files, and optionally sends these files via email.

The tool can read configuration from **command-line flags** or from a **.env file** (default `.env` in the working directory).

---

## Prerequisites

* **Go environment**: Ensure Go (version 1.16 or higher) is installed.
* **SMTP Server Access**: If using email, you must have access to a valid SMTP server (Gmail, Outlook, or a dedicated server) and credentials.
* **Optional .env file**: Stores API credentials, SMTP details.

Example `.env` file:

```bash
# API credentials (either API_TOKEN or LOGIN_NAME/PASSWORD)
BASE_URL=https://api.example.com
API_TOKEN="your_bearer_token_here"
# Alternatively:
# LOGIN_NAME=admin
# PASSWORD="My#SecretPassword"

# Email configuration
EMAIL_TO="recipient@example.com,spouse@example.com"
EMAIL_MESSAGE="<p>Here is your <b>monthly</b> report.</p>"
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=myemail@gmail.com
SMTP_PASS="my#app_password"
SMTP_FROM=myemail@gmail.com

# PDF configuration
PDF_PASSWORD="SecretFilePassword"
```

> ⚠️ Note: If a value contains `#`, spaces, or `$`, **wrap it in quotes** (single `'` or double `"`).

---

## Running the Script

You can either run it directly using `go run` or build a standalone binary:

```bash
go run main.go
```

or

```bash
go build -o ezbookkeeping_tools main.go
./ezbookkeeping_tools
```

By default, the tool attempts to load `.env` if command-line flags are missing. You can specify a custom config file:

```bash
./ezbookkeeping_tools -config /path/to/custom.env
```

---

## Features

### 1. Account Processing
-   **Structure**: Recursively discovers subaccounts and displays them as `Subaccount (Parent)`.
-   **Filtering**: Excludes parent containers and zero-balance accounts.

### 2. PDF Reporting
-   **Format**: Generates a password-protected PDF attachment.
-   **Styling**: Modern design with colored headers, zebra-striped rows, and color-coded balances (Green/Red).
-   **Content**: Full details including Name, Currency, Balance, Category, and wrapping Comments.

### 3. Email Delivery
-   **Multiple Recipients**: Supports comma-separated email addresses.
-   **Custom Message**: Allows a custom HTML body message via flag/env.

### 4. Dry Run Mode
-   **Safe Testing**: Use `--dry-run` to generate the PDF and verify logic without sending any emails.

---

## Command-Line Flags

| Flag            | Type    | Description                                                                         | Required | Example                                                      |
| --------------- | ------- | ----------------------------------------------------------------------------------- | -------- | ------------------------------------------------------------ |
| -url            | string  | Base URL of the API.                                                                | Yes*     | -url "https://api.example.com"                               |
| -token          | string  | Bearer token for API authorization (takes precedence over user/pass).               | Yes*     | -token "eyJhbGciOiJIUzI1NiIsIn..."                           |
| -user           | string  | Login name for API authorization (if token not provided).                           | Yes*     | -user "john.doe"                                             |
| -pass           | string  | Password for API authorization (if token not provided).                             | Yes*     | -pass "S3cr3tP@ssw0rd"                                       |
| -debug          | boolean | Enable detailed HTTP request/response logging.                                      | No       | -debug                                                       |
| -print          | boolean | Print CSV data to console.                                                          | No       | -print                                                       |
| -dry-run        | boolean | Generate PDF but skip sending email.                                                | No       | -dry-run                                                     |
| -email-to       | string  | Comma-separated recipient email addresses.                                          | No       | -email-to "me@corp.com,boss@corp.com"                        |
| -email-message  | string  | Custom HTML message for the email body.                                             | No       | -email-message "<b>Here is the report</b>"                   |
| -smtp-host      | string  | SMTP server hostname.                                                               | No       | -smtp-host "smtp.gmail.com"                                  |
| -smtp-port      | int     | SMTP server port (default 587).                                                     | No       | -smtp-port 587                                               |
| -smtp-user      | string  | SMTP username.                                                                      | No       | -smtp-user "myemail@gmail.com"                               |
| -smtp-pass      | string  | SMTP password.                                                                      | No       | -smtp-pass "app_password"                                    |
| -smtp-from      | string  | Email sender address. Defaults to SMTP username.                                    | No       | -smtp-from "sender@example.com"                              |
| -pdf-password   | string  | Password to encrypt the PDF report.                                                 | No       | -pdf-password "FileSafe123"                                  |
| -config         | string  | Path to a configuration file (default `.env`).                                      | No       | -config "./myconfig.env"                                     |

> *Either `-token` (or `API_TOKEN`/`TOKEN` in `.env`) OR `-user` and `-pass` (or `LOGIN_NAME` and `PASSWORD` in `.env`) is required along with `-url`.

---

---

## Examples

### 1. Running with Bearer Token (Safe Dry Run)

```bash
go run main.go \
    -url "https://api.example.com" \
    -token "eyJhbGciOiJIUzI1NiIsIn..." \
    --dry-run
```

### 2. Standard Run with Username and Password with Email

Fetches data, generates PDF, and sends it to multiple recipients.

```bash
go run main.go \
    -url "https://api.example.com" \
    -user "admin" \
    -pass "secret" \
    -email-to "me@example.com,spouse@example.com" \
    -smtp-host "smtp.gmail.com" \
    -smtp-user "me@gmail.com" \
    -smtp-pass "app_password"
```

### 3. Custom Email Message

Sends a report with a custom HTML body.

```bash
go run main.go \
    --email-to "me@example.com" \
    --email-message "Here is your <b>monthly</b> financial summary." \
    [other flags...]
```

### 4. Encrypted PDF Report

Protects the generated PDF with a password.

```bash
go run main.go \
    --pdf-password "MySecretFiles" \
    [other flags...]
```

---

## Output Files

Upon successful execution, the following files are created:
1.  `Financial_Report_YYYY-MM-DD.pdf` (The main report)
2.  `assets.csv`
3.  `liabilities.csv`
