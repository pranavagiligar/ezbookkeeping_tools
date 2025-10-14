# Ezbookkeeping Account Summary Tool Usage 

This document describes how to build and run the Go script to retrieve account data from the API, separate it into assets and liabilities, format the balances using ISO 4217 standards, and export the results to CSV files.

# Prerequisites
- Go environment: Ensure Go (version 1.16 or higher) is installed on your system.

# Running the Script
You can run the script directly using go run or first build a standalone executable binary.
```
go run main.go
```

# Command-Line Flags
The script requires three mandatory flags for authentication and two optional flags for controlling output.

| Flag     | Type    | Description                                                                                                      | Required | Example                           |
|----------|---------|------------------------------------------------------------------------------------------------------------------|----------|-----------------------------------|
| -url     | string  | The base URL of the API (e.g., https://api.example.com).                                                             | Yes      | -url "https://api.example.com"  |
| -user    | string  | The login name used for API authorization.                                                                       | Yes      | -user "john.doe"                  |
| -pass    | string  | The password used for API authorization.                                                                         | Yes      | -pass "S3cr3tP@ssw0rd"            |
| -debug   | boolean | Optional. Enable detailed HTTP request and response logging for troubleshooting.                                 | No       | -debug                            |
| -print   | boolean | Optional. Print the resulting CSV data to the console (using tabs for alignment) in addition to exporting files. | No       | -print                            |

# Examples
1. Standard Run and Export
This command fetches the data, processes the accounts, and exports two files (assets.csv and liabilities.csv) to the current directory.

```
go run main.go \
    -url "https://api.example.com" \
    -user "myuser" \
    -pass "mypassword"
```

2. Export and Print to Console
Use the -print flag to simultaneously save the files and view the results in the terminal.

```
go run main.go \
    -url "https://api.example.com" \
    -user "myuser" \
    -pass "mypassword" \
    -print
```

3. Debugging Authentication or Data Retrieval Issues
Use the -debug flag to display the raw HTTP headers and request bodies, which helps in diagnosing connection or authorization errors.

```
go run main.go \
    -url "https://api.example.com" \
    -user "myuser" \
    -pass "mypassword" \
    -debug
```

# Output Files
Upon successful execution, two CSV files will be created in the directory where the script is run:
* assets.csv
* liabilities.csv

The Balance field in these files is correctly converted from the API's minor units (e.g., cents) to the major unit (e.g., dollars) based on the *ISO 4217* Currency Code.

**CSV Header:** ID, Name, Currency, Balance, Category, IsAsset, IsLiability, Comment
