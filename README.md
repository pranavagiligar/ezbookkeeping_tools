# Ezbookkeeping Account Summary Tool Usage

This tool fetches account data from the API, separates it into **assets** and **liabilities**, formats balances using **ISO 4217** standards, exports the results to CSV files, and optionally sends these files via email.

The tool can read configuration from **command-line flags** or from a **.env file** (default `.env` in the working directory).

---

## Prerequisites

* **Go environment**: Ensure Go (version 1.16 or higher) is installed.
* **SMTP Server Access**: If using email, you must have access to a valid SMTP server (Gmail, Outlook, or a dedicated server) and credentials.
* **Optional .env file**: Stores API credentials, SMTP details

Example `.env` file:

```bash
# API credentials
BASE_URL=https://api.example.com
LOGIN_NAME=admin
PASSWORD="My#SecretPassword"

# Email configuration
EMAIL_TO="recipient@example.com"
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=myemail@gmail.com
SMTP_PASS="my#app_password"
SMTP_FROM=myemail@gmail.com
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

## Command-Line Flags

| Flag       | Type    | Description                                                                         | Required | Example                                                      |
| ---------- | ------- | ----------------------------------------------------------------------------------- | -------- | ------------------------------------------------------------ |
| -url       | string  | Base URL of the API (e.g., [https://api.example.com](https://api.example.com)).     | Yes*     | -url "[https://api.example.com](https://api.example.com)"    |
| -user      | string  | Login name for API authorization.                                                   | Yes*     | -user "john.doe"                                             |
| -pass      | string  | Password for API authorization.                                                     | Yes*     | -pass "S3cr3tP@ssw0rd"                                       |
| -debug     | boolean | Optional. Enable detailed HTTP request/response logging.                            | No       | -debug                                                       |
| -print     | boolean | Optional. Print CSV data to console in addition to exporting files.                 | No       | -print                                                       |
| -smtp-host | string  | Optional. SMTP server hostname.                                                     | No       | -smtp-host "smtp.gmail.com"                                  |
| -smtp-port | int     | Optional. SMTP server port (default 587 for TLS).                                   | No       | -smtp-port 587                                               |
| -smtp-user | string  | Optional. SMTP username.                                                            | No       | -smtp-user "[myemail@gmail.com](mailto:myemail@gmail.com)"   |
| -smtp-pass | string  | Optional. SMTP password.                                                            | No       | -smtp-pass "my#app_password"                                 |
| -email-to  | string  | Recipient email address(es) for the report. Comma-separated for multiple addresses. | No       | -email-to "[report@corp.com](mailto:report@corp.com)"        |
| -smtp-from | string  | Optional. Email sender address. Defaults to SMTP username if omitted.               | No       | -smtp-from "[sender@example.com](mailto:sender@example.com)" |
| -config    | string  | Optional. Path to a configuration file (default `.env`).                            | No       | -config "./myconfig.env"                                     |

> *Required if not provided via `.env`.

---

## Notes on Email

* If `-smtp-host` is provided, the tool attempts to send the report via email.

---

## Examples

### 1. Standard Run and Export (No Email)

Exports CSV files only:

```bash
go run main.go \
    -url "https://api.example.com" \
    -user "myuser" \
    -pass "mypassword"
```

> The tool will also attempt to load missing values from `.env`.

### 2. Run, Export, and Email

Fetches the data, saves CSVs, and sends the report via SMTP:

```bash
go run main.go \
    -url "https://api.example.com" \
    -user "myemail@gmail.com" \
    -pass "my_app_password" \
    -email-to "recipient@corp.com,boss@corp.com" \
    -smtp-host "smtp.gmail.com" \
    -smtp-port 587
```

### 3. Email from a Different Sender

```bash
go run main.go \
    -url "https://api.example.com" \
    -user "myuser" \
    -pass "mypassword" \
    -email-to "recipient@corp.com" \
    -smtp-host "smtp.mydomain.com" \
    -smtp-from "sender@tool.com"
```

### 4. With .env located on same as binary or main.go


```bash
go run main.go
```

---

## Output Files

Upon successful execution, two CSV files are created in the current directory:

* `assets.csv`
* `liabilities.csv`

The **Balance** field is converted from API minor units to major units based on *ISO 4217* currency codes.

**CSV Header:** `ID, Name, Currency, Balance, Category, IsAsset, IsLiability, Comment`
