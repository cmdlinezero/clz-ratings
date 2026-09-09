# CERTIN API

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.23%2B-blue.svg)](https://golang.org/)
[![Docker](https://img.shields.io/badge/Docker-Distroless-informational.svg)](Dockerfile)

A lightweight Go API built for handling rating functionality on static sites (such as Hugo), featuring Google Cloud Storage (GCS) and local file persistence across sessions.

---

## ✨ Features

- **Zero-Database Overhead:** Uses file-based storage or Google Cloud Storage objects (`.json`) per content ID.
- **Concurrent-Safe GCS Voting:** Implements generation-match preconditions and retry loops to safely handle race conditions during concurrent votes.
- **Dynamic SVG Badges:** Generates clean, responsive inline star-rating SVGs (`.svg`) for dynamic embedding.
- **Flexible Storage Backends:** Easily switch between local file storage (`file://`) for development and GCS (`gs://`) for production.
- **Production Ready:** Ships with a multi-stage Docker build utilizing Distroless containers for minimal footprint and maximum security.

---

## 🛠️ Configuration & Storage

The application is configured using environment variables:

| Variable | Default | Description |
| :--- | :--- | :--- |
| `RATINGS_STORAGE_URI` | `gs://clz-certin/ratings` | Storage backend URI (`gs://bucket/prefix` or `file://directory`). |
| `IMAGE_BASE_URL` | *unset* | Optional public base URL for hosted rating SVG images. |
| `PORT` | `8080` | Port for the HTTP server to listen on. |


## Persistence

The default persistence endpoint is:

```text
gs://clz-certin/ratings
```

Each content item is stored as one object:

```text
gs://clz-certin/ratings/{id}.json
```

For example, `my-post` is stored at:

```text
gs://clz-certin/ratings/my-post.json
```

The endpoint is configurable with `RATINGS_STORAGE_URI`:

```bash
RATINGS_STORAGE_URI=gs://another-bucket/some-prefix go run .
```

For local development you can retain file persistence:

```bash
RATINGS_STORAGE_URI=file://./data go run .
```

### GCS authentication

The GCS store uses Google Application Default Credentials (ADC). On a developer machine, authenticate with the Google Cloud CLI or set `GOOGLE_APPLICATION_CREDENTIALS`. On Cloud Run, attach a service account with access to the bucket.

The service account needs object read/create/update permissions for the configured bucket/prefix. A simple bucket-level role for an MVP is `roles/storage.objectUser`.

Votes use GCS generation-match preconditions. The service reads the current object generation and only replaces that exact generation. If another request wins the race, the vote is reread and retried rather than overwriting the concurrent vote.

## Dependencies

Fetch modules with:

```bash
go mod tidy
```

Then:

```bash
go test ./...
go run .
```

## API

### Get a rating

```bash
curl http://localhost:8080/v1/ratings/my-post
```

New IDs return:

```json
{"id":"my-post","rating":null,"count":0,"max":5}
```

### Submit a vote

```bash
curl -X POST http://localhost:8080/v1/ratings/my-post/vote \
  -H 'content-type: application/json' \
  -d '{"rating":4}'
```

The first vote creates the object automatically.

### SVG

```bash
curl http://localhost:8080/v1/ratings/my-post.svg
```

Hugo example:

```html
<img src="https://ratings.example.com/v1/ratings/{{ .Params.rating_id }}.svg"
     alt="Article rating"
     loading="lazy">
```

### Image URL

```bash
curl http://localhost:8080/v1/ratings/bash/image
```

Set an external public SVG base URL with:

```bash
IMAGE_BASE_URL=https://storage.googleapis.com/clz-certin/images go run .
```

The response is:

```json
{"id":"bash","image":"https://storage.googleapis.com/clz-certin/images/bash.svg"}
```

If `IMAGE_BASE_URL` is unset, the endpoint returns the API's generated SVG URL.

## Cloud Run

The application defaults to GCS, so a typical Cloud Run deployment only needs the storage URI if you want to override it:

```bash
gcloud run deploy clz-ratings \
  --source . \
  --region europe-west1 \
  --set-env-vars RATINGS_STORAGE_URI=gs://clz-certin/ratings
  --service-account SERVICE_ACCOUNT_EMAIL
```

Make sure the Cloud Run service account has permission to read and write objects in `gs://clz-certin`.

## Docker

```bash
docker build -t rating-api .
docker run --rm -p 8080:8080 \
  -e RATINGS_STORAGE_URI=file://./data \
  -v "$PWD/data:/app/data" \
  rating-api
```
