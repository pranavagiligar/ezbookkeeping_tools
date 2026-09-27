Developer Quickstart

This file contains quick development commands, checks, and tips for working on MoneyPath.

Run locally

```bash
cd moneypath
go mod tidy
go run main.go
```

Run on alternate port

```bash
go run main.go -port=8082
# or
PORT=8082 go run main.go
```

Common checks

```bash
# format
gofmt -w .
# static checks
go vet ./...
# fetch modules
go mod download
```

Debugging bind errors

```bash
# find process listening on port 8081
ss -ltnp | grep ':8081'
# or
lsof -i :8081
# then kill the PID
kill <PID>
```

Tiles

- MBTiles: set `MBTILES_PATH` in `moneypath/.env` or put `tiles.mbtiles` in the `moneypath/` folder.
- Directory tiles: place files under `moneypath/tiles/{z}/{x}/{y}.png`.

Logs

- The server prints a small banner and useful diagnostics on startup.

Contributing

- Create a branch for feature work, keep changes small and focused, and open a PR with a short description of the change.
