# Telegram Storage API

REST API yang menggunakan Telegram sebagai backend penyimpanan file. Dibangun dengan Go + MTProto (gotd/td) untuk mendukung upload file hingga 2 GB.

## Fitur

- Upload file ke Telegram channel via MTProto (hingga 2 GB)
- Stream file langsung ke browser tanpa download ke server
- Download file via API
- Autentikasi JWT
- Multi-user support
- Docker ready

## Prasyarat

1. **API ID & API Hash** dari [my.telegram.org](https://my.telegram.org)
2. **Telegram Channel** — buat channel privat, lalu dapatkan channel ID (bisa pakai [@userinfobot](https://t.me/userinfobot) atau forward pesan dari channel ke [@JsonDumpBot](https://t.me/JsonDumpBot))
3. **Docker & Docker Compose** (untuk deploy)
4. **Go 1.22+** (untuk development lokal)

## Setup

### 1. Clone & konfigurasi

```bash
cp .env.example .env
```

Edit `.env` dengan data kamu:

```env
SERVER_PORT=8080
JWT_SECRET=ganti-dengan-secret-random

DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=telegram_storage
DB_SSLMODE=disable

TELEGRAM_API_ID=12345678
TELEGRAM_API_HASH=abcdef1234567890abcdef1234567890
TELEGRAM_PHONE=+628xxxxxxxxxx
TELEGRAM_CHANNEL_ID=-100xxxxxxxxxx
```

### 2. Jalankan dengan Docker

```bash
docker compose up --build
```

### 3. Atau jalankan lokal

```bash
# Pastikan PostgreSQL sudah berjalan
go run ./cmd/server
```

Saat pertama kali dijalankan, kamu akan diminta memasukkan kode verifikasi Telegram di terminal.

## API Documentation

### Base URL

```
http://localhost:8080
```

### Health Check

```
GET /health
```

Response:
```json
{
  "status": "ok",
  "telegram_ready": true
}
```

---

### Auth

#### Register

```
POST /api/auth/register
Content-Type: application/json

{
  "username": "admin",
  "password": "secret123"
}
```

#### Login

```
POST /api/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "secret123"
}
```

Response:
```json
{
  "success": true,
  "data": {
    "token": "eyJhbGci...",
    "user": {
      "id": 1,
      "username": "admin",
      "created_at": "2024-01-01T00:00:00Z"
    }
  }
}
```

---

### Files (memerlukan header `Authorization: Bearer <token>`)

#### Upload File

```
POST /api/files/upload
Authorization: Bearer <token>
Content-Type: multipart/form-data

file: <binary>
```

Response:
```json
{
  "success": true,
  "message": "file uploaded",
  "data": {
    "file": {
      "id": 1,
      "file_name": "video.mp4",
      "file_size": 52428800,
      "mime_type": "video/mp4",
      "created_at": "2024-01-01T00:00:00Z"
    },
    "stream_url": "/api/files/1/stream",
    "download_url": "/api/files/1/download"
  }
}
```

#### Upload Video + Metadata Google Sheet

Mengunggah video ke Telegram sekaligus menulis baris `file_name | judul | deskripsi | hashtag` ke Google Sheet (baris dengan `file_name` sama diperbarui, jika belum ada ditambahkan). Membutuhkan `GOOGLE_SERVICE_ACCOUNT_FILE` dan `GOOGLE_SHEET_ID`; tanpa itu endpoint membalas `503`.

```
POST /api/files/upload-video
Authorization: Bearer <token>
Content-Type: multipart/form-data

file: <video .mp4/.mov/.m4v/.webm/.mkv>   (wajib)
judul: <maks 100 karakter, tanpa < >>     (wajib)
deskripsi: <teks>                          (opsional)
hashtag: <#a #b>                           (opsional)
```

Total `judul + deskripsi + hashtag` maksimal 2200 karakter (batas caption Instagram). Nama file yang sudah ada ditolak dengan `409`.

Response `201`:
```json
{
  "success": true,
  "message": "video uploaded and sheet updated",
  "data": {
    "file": { "id": 5, "file_name": "ep04.mp4", "mime_type": "video/mp4", "...": "..." },
    "stream_url": "/api/public/files/5/stream",
    "download_url": "/api/public/files/5/download",
    "judul": "Judul video",
    "deskripsi": "Deskripsi",
    "hashtag": "#dibaliksaklar",
    "sheet_synced": true
  }
}
```

Jika video tersimpan tetapi penulisan sheet gagal, respons tetap `201` dengan `sheet_synced: false` dan `sheet_error` — jangan ulangi request (video akan terunggah dua kali); tambahkan barisnya manual.

Contoh:
```bash
curl -X POST http://localhost:8080/api/files/upload-video \
  -H "Authorization: Bearer <token>" \
  -F "file=@ep04.mp4" -F "judul=Judul video" -F "deskripsi=Deskripsi" -F "hashtag=#dibaliksaklar"
```

#### List Files

```
GET /api/files?limit=20&offset=0
Authorization: Bearer <token>
```

#### Stream File (tampilkan di browser/embed)

```
GET /api/files/:id/stream
Authorization: Bearer <token>
```

Cocok untuk `<img src>`, `<video src>`, atau embed di web.

#### Download File

```
GET /api/files/:id/download
Authorization: Bearer <token>
```

Browser akan mengunduh file.

#### Delete File

```
DELETE /api/files/:id
Authorization: Bearer <token>
```

## Contoh Penggunaan di Frontend

```html
<!-- Menampilkan gambar -->
<img src="http://localhost:8080/api/files/1/stream"
     headers="Authorization: Bearer TOKEN" />

<!-- Lebih praktis: fetch dengan JS -->
<script>
async function loadImage(fileId) {
  const res = await fetch(`http://localhost:8080/api/files/${fileId}/stream`, {
    headers: { 'Authorization': 'Bearer YOUR_TOKEN' }
  });
  const blob = await res.blob();
  document.getElementById('img').src = URL.createObjectURL(blob);
}
</script>
```

## Arsitektur

```
Client  →  REST API (Gin)  →  PostgreSQL (metadata)
                            →  Telegram MTProto (file storage)
```

- File disimpan di Telegram channel sebagai document
- Metadata (nama, ukuran, mime type, message ID) disimpan di PostgreSQL
- Saat stream/download, file langsung di-proxy dari Telegram ke client
