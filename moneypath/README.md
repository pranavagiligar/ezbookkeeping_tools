# 🗺️ MoneyPath: ezBookkeeping Geotag Route Animator

**MoneyPath** is a high-performance visualizer and route animator designed for **ezBookkeeping** transaction geotags. It queries transaction history, connects sequential coordinates into smooth chronological trajectories, and animates travel trails across 3,000+ GPS points using fixed-phase or real-time simulation.

---

## ✨ Features

- **Sequential 3,000+ Coordinate Line Drawing**: Canvas-accelerated polyline rendering at 60 FPS.
- **Fixed-Time Phase Animation**: Choose total animation duration (e.g. 10s, 30s, 60s, 120s) or real-time proportional playback.
- **Dynamic Traveler Marker & Auto-Cam**: Pulsing locator follows active coordinate with smooth camera panning.
- **Live Cumulative Spend HUD**: Real-time ticker showing spending increase as the route progresses geographically.
- **Date Range Filters**: Presets for *Last 7 Days, Last 30 Days, This Month, This Year, All-Time, or Custom Start/End dates*.
- **Interactive Scrubber & Speed Controls**: Scrub forwards/backwards, step point-by-point, or adjust speed ($0.5\times$ to $25\times$).
- **Multiple Map Styles**: Carto Dark Matter, OpenStreetMap, Voyager Light, and Satellite Imagery.
- **Standalone Export**:
  - Export to a single self-contained `.html` map file for offline presentation or sharing.
  - Export to standard GIS `.geojson`.
  - Export to `.csv`.
- **Zero-Config 3,000 Point Synthetic Demo Generator**: Instant stress-testing and demonstration even without an active ezBookkeeping server.

---

## 🚀 Quick Start

### 1. Configuration (Optional)
Copy `.env.example` to `moneypath/.env` (or use root `.env`):

```bash
cp moneypath/.env.example moneypath/.env
```

Edit `moneypath/.env` with your ezBookkeeping instance details:
```env
BASE_URL=https://your-ezbookkeeping-instance.com
API_TOKEN=your_token_here
PORT=8081
```

### 2. Run the Application

```bash
go run ./moneypath
```

Or pass flags directly:
```bash
go run ./moneypath -url "https://my-ezbookkeeping.com" -token "YOUR_TOKEN" -port 8081
```

Open your browser at:
👉 **[http://localhost:8081](http://localhost:8081)**

## 📦 Offline Tiles (MBTiles)

MoneyPath can serve offline tiles in two ways:

- MBTiles file: set `MBTILES_PATH` in your `moneypath/.env` to point to a `.mbtiles` file (e.g. `tiles.mbtiles`). The server will read tiles from the MBTiles file and expose them at `/tiles/{z}/{x}/{y}.png`.
- Directory tiles: place PNG tiles in `moneypath/tiles/{z}/{x}/{y}.png`. The server will serve these files directly at `/tiles/{z}/{x}/{y}.png`.

Choose `Local Tiles` from the map style selector in the UI to use either source. MBTiles is preferred for compact distribution and ease of transfer.

---

## ⌨️ Controls & Shortcuts

| Action | Shortcut / Control |
|---|---|
| **Play / Pause** | `Spacebar` or Play button |
| **Step Backward / Forward** | `Left Arrow` / `Right Arrow` |
| **Scrub Timeline** | Drag Timeline slider at bottom |
| **Toggle Camera Follow** | Click `Follow Cam` button |
| **Demo 3,000 Points** | Click `Demo 3,000 GPS` in top bar |
| **Filter by Date** | Click Date button in top bar |
| **Export Data** | Click `Export` in top bar |

---

## 🛠 Developer Notes

Quick commands for development and debugging:

- Install dependencies and tidy modules:
  ```bash
  go mod tidy
  ```
- Run the server (from the `moneypath` folder):
  ```bash
  cd moneypath
  go run main.go
  ```
- Run with a different port:
  ```bash
  go run main.go -port=8082
  # or set env var
  PORT=8082 go run main.go
  ```

Troubleshooting tips:

- If you see "bind: address already in use", identify the process using the port:
  ```bash
  ss -ltnp | grep ':8081' || lsof -i :8081
  kill <PID>
  ```
- If tiles are not loading, check these sources (in order):
  1. MBTiles file: set `MBTILES_PATH` in `.env` or place `tiles.mbtiles` in `moneypath/`.
  2. Directory tiles: ensure `moneypath/tiles/{z}/{x}/{y}.png` exists and is readable.

Testing & static checks:

```bash
go vet ./...
gofmt -w .
``` 

Packaging notes:

- To distribute offline tiles, prefer an `.mbtiles` file and set `MBTILES_PATH` in `.env`.
- The UI can export single-transaction CSV and GeoJSON via `/export/transaction/*` endpoints.

Contact / Handoff

If you're handing this project off, share the following with the new maintainer:

- The `moneypath/.env` (or root `.env`) with `BASE_URL` and `API_TOKEN` if connecting to ezBookkeeping.
- Any MBTiles file (optional) under `moneypath/tiles.mbtiles`.
- The current plan is stored under `/memories/repo/moneypath_plan.json` in this workspace for task history.

## 📁 Architecture

- `moneypath/main.go`: Server entrypoint, flag parsing, `.env` loader, and HTTP handlers.
- `moneypath/client/ezbookkeeping.go`: ezBookkeeping API client, pagination, distance calculation (Haversine), and synthetic 3,000-point generator.
- `moneypath/models/models.go`: Data structures for transactions, geo-coordinates, and JSON payloads.
- `moneypath/web/`: Embedded web UI (HTML5, modern CSS glassmorphism, Leaflet.js Canvas engine, animation controller).
