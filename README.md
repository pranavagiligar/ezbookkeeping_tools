# Ezbookkeeping Account Summary Tool Usage 

This document describes how to build and run the Go script to retrieve account data from the API, separate it into assets and liabilities, format the balances using ISO 4217 standards, export the results to CSV files, and optionally send these files via email.

# Prerequisites
- Go environment: Ensure Go (version 1.16 or higher) is installed on your system.
- SMTP Server Access: If using the email feature, you must have access to a valid SMTP server (e.g., Gmail, Outlook, dedicated server) and credentials.

# Running the Script
You can run the script directly using go run or first build a standalone executable binary.
```
go run main.go
```

# Command-Line Flags
The script requires three mandatory flags for authentication and two optional flags for controlling output.
| Flag   | Type    | Description                                                                                               | Required | Example                        |
|--------|---------|-----------------------------------------------------------------------------------------------------------|----------|--------------------------------|
| -url   | string  | The base URL of the API (e.g., https://api.example.com).                                                  | Yes      | -url "https://api.example.com" |
| -user  | string  | The login name used for API authorization.                                                                | Yes      | -user "john.doe"               |
| -pass  | string  | The password used for API authorization.                                                                  | Yes      | -pass "S3cr3tP@ssw0rd"         |
| -debug | boolean | Optional. Enable detailed HTTP request and response logging.                                              | No       | -debug                         |
| -print | boolean | Optional. Print the resulting CSV data to the console in addition to exporting files.                     | No       | -print                         |
| -smtp  | string  | Optional. The SMTP server hostname (e.g., https://www.google.com/url?sa=E&source=gmail&q=smtp.gmail.com). | No       | -smtp "smtp.office365.com"     |
| -port  | int     | Optional. The SMTP server port (default is 587 for TLS).                                                  | No       | -port 465                      |
| -to    | string  | Recipient email address(es) for the report. Use commas for multiple addresses.                            | Yes      | -to "report@corp.com"          |
| -from  | string  | Optional. The sender email address. If omitted, uses -user flag value.                                    | No       | -from "tool.user@example.com"  |

# Note on Email Requirements
If you provide the -smtp flag, the script will attempt to send the report. The user/pass credentials provided for the API (-user and -pass) are reused for SMTP authentication.

# Examples
1. Standard Run and Export (No Email)
This command fetches, processes, and exports two files (assets.csv and liabilities.csv) to the current directory. The -to flag is still mandatory but its value is ignored if no SMTP server is specified.

```
go run main.go \
    -url "https://api.example.com" \
    -user "myuser" \
    -pass "mypassword" \
    -to "placeholder@example.com"
```

2. Run, Export, and Email Report
This command fetches the data, saves the CSV files, and sends both files as attachments to report@corp.com using Gmail's SMTP server.

```
go run main.go \
    -url "https://api.example.com" \
    -user "myemail@gmail.com" \
    -pass "my_app_password" \
    -to "recipient@corp.com,boss@corp.com" \
    -smtp "smtp.gmail.com" \
    -port 587
```

3. Email from a Different Sender
This command uses sender@tool.com as the email sender, authenticating with myuser and mypassword.

```
go run main.go \
    -url "https://api.example.com" \
    -user "myuser" \
    -pass "mypassword" \
    -to "recipient@corp.com" \
    -smtp "smtp.mydomain.com" \
    -from "sender@tool.com"
```

# Output Files
Upon successful execution, two CSV files will be created in the directory where the script is run:
* assets.csv
* liabilities.csv

The Balance field in these files is correctly converted from the API's minor units (e.g., cents) to the major unit (e.g., dollars) based on the *ISO 4217* Currency Code.

**CSV Header:** ID, Name, Currency, Balance, Category, IsAsset, IsLiability, Comment
