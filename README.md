# NHA HTTPS reference server (limited compatibility testing)

**Purpose:** Verify NHA's *custom* HTTPS upload/download protocol with independently hosted infrastructure. **Not a speed benchmark.** This server does not belong to the NHA desktop app and makes no changes to NHA's code or defaults.

## Endpoints

- `GET /health`: health check.
- `GET /__down?bytes=N`: HTTP 200, exactly N pseudorandom binary bytes, 1..8,388,608. Cache-disabled.
- `POST /__up`: HTTP 200 and `X-NHA-Bytes-Received: N` ONLY after exactly N request-body bytes have been read. Known content length required, 1..8,388,608.
- All other URLs: HTTP 404.

In Render, TLS is terminated by the platform's own HTTPS endpoint, so the Go server listens on HTTP internally at `$PORT` on `0.0.0.0`. It does not persist uploaded data or measurement logs; the platform may maintain service/access logs independently.

## Create a public GitHub repository

1. Create a new **Public** GitHub repository, e.g. `nha-https-reference-server`. **Do not upload your NHA Windows project** or any credentials.
2. Extract this ZIP and upload `main.go`, `main_test.go`, `go.mod`, and `README.md` into the *root* of the GitHub repository, using **Add file > Upload files** and **Commit changes**. GitHub does not unzip an uploaded archive for you.
3. Copy the repository URL `https://github.com/YOUR-USER/nha-https-reference-server`.

## Configure Render > New Web Service

- Source: **Public Git Repository**; paste your GitHub repository URL and choose **Connect**.
- Name: `nha-https-reference-server` (adjust if unavailable).
- Language: `Go`.
- Build command: `go build -o server .`
- Start command: `./server`
- Health check path: `/health` (if available).
- Instance type: **Free** (verify before deploying).
- No paid add-ons, no database, no persistent disk, no credentials required.

After it reports Live, use the assigned `https://...onrender.com` URL. Render's auto-assigned URL is unpredictable until the service is created.

## Verify safely from PowerShell (8 KiB each direction)

Change `$base` to your Render service's exact HTTPS URL:

```powershell
$base = 'https://YOUR-SERVICE.onrender.com'
$dl = Invoke-WebRequest -Uri "$base/__down?bytes=8192" -UseBasicParsing
$dl.RawContentLength
$headers = @{ 'Content-Type' = 'application/octet-stream' }
$ul = Invoke-WebRequest -Uri "$base/__up" -Method Post -Body ([byte[]]::new(8192)) -Headers $headers -UseBasicParsing
$ul.StatusCode
$ul.Headers['X-NHA-Bytes-Received']
```

Expected download bytes: `8192`, upload status `200`, upload receipt `8192`. **If testing via Windows PowerShell 5.1, binary-content length reporting through Invoke-WebRequest can be unreliable; use the earlier HttpClient byte-array probe for definitive length checks.**

You may also supply this URL as both download and upload base URLs in NHA, but **a single controlled Custom Diagnostics download-only run may be attempted**, but the 64 MiB-per-direction runtime budget can end the load early, making speed/loaded-latency comparisons INCONCLUSIVE. Do not use this server to rate your ISP or perform repeated full Diagnostics. It resets if the process restarts, so it is not an absolute monthly usage cap.

## Compatibility correction (v2)

NHA 3.11.2.5.1 initially makes three 256 KiB download requests, then uses 8 MiB requests during its 15-second sustained phase. The original reference server rejected every request over 2 MiB with HTTP 400. Version 2 accepts up to 8 MiB per request while retaining the 64 MiB cumulative budget per direction. A subsequent request may return HTTP 429 when that budget is exhausted. This is an expected **server limitation**, not a measurement of ISP speed.

Deploy by replacing the four files in the existing public GitHub repository and committing the changes; Render auto-deploys on commit if that option remains enabled. The base URL and NHA endpoint fields are unchanged.

## Safety / limits

- 8 MiB per individual transfer, 64 MiB per direction per process, 24 concurrent requests maximum.
- The service is **public and unauthenticated** because NHA custom-server requests have no authorization-token setting. Anyone knowing the URL could use it, cause the small budget to be consumed, or trigger repeated instance restarts. It is **not production safe**.
- On the Render Free tier, the app may sleep between tests, and Render may still apply bandwidth/instance limits. Check the current service/billing dashboard before deploying.
- **Delete the Render service immediately after verification** to avoid ongoing public exposure. Do not treat this server as a speed test or as evidence that your ISP is faster or slower than Cloudflare.

## Local tests

```sh
go test ./...
go test -race ./...
go vet ./...
go build -o server .
```

Go 1.23 or later, standard library only. No external packages.
