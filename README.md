# Aditya GuY Router (Antigravity AI Gateway & Quota Manager)

AI Gateway lokal, proxy OpenAI/Anthropic kompatibel, dan dashboard manajemen multi-akun Google Cloud Code (Antigravity / Gemini Code Assist) dengan fitur **Token Saver Cerdas**, **Rantai Fallback Otomatis (Anti-429)**, dan **Pelacak Kuota Real-time**.

---

## Fitur Utama

- **OpenAI & Anthropic Compatible API**:
  - `POST /v1/chat/completions` (OpenAI format, streaming & function calling)
  - `POST /v1/messages` (Anthropic format, tool use)
  - `GET /v1/models` (Daftar model aktif)
- **Multi-Akun Google Antigravity & Rotasi Cerdas**:
  - Dukungan multi-akun dengan rotasi kuota tertinggi (*Smart Quota*), prioritas urutan akun (*Fill First*), atau failover akun saat mendekati limit.
  - Impor akun otomatis dari data OmniRoute lokal jika tersedia.
- **Pelacak Kuota Akurat (5-Jam & Mingguan)**:
  - Pemisahan kuota sesi 5-jam (rolling) dan batas mingguan untuk Gemini dan Claude.
  - Tampilan jam dan sisa durasi reset kuota secara otomatis.
- **Token Saver & Analytics**:
  - Pemangkasan prompt repetitif dengan persona cerdas (RTK, Caveman, Ponytail, Context Mode).
  - Grafik visual penghematan token riil vs estimasi mentah dan audit transaksi per-model.
- **Model Fallback Chain (Anti-429)**:
  - Pengalihan otomatis request ke model cadangan bila model upstream mengembalikan error kuota (429 / Quota Exceeded).
  - Konfigurasi rantai model tersimpan permanen dan tersinkronisasi langsung dengan `/v1/models`.
- **Manajemen API Key & Personas**:
  - Kunci akses aman untuk IDE (VS Code, Cursor, Windsurf) dan AI Agent CLI (Hermes, Claude Code, OpenCode).
- **Arsitektur Ramping (Ultra-Lightweight)**:
  - Backend murni Go tanpa framework besar, memory footprint < 20MB.
  - Frontend murni HTML, Tailwind CSS v4, dan JavaScript vanilla tanpa build step rumit.

---

## Persyaratan Sistem

- **Pilihan 1 (Rekomendasi)**: Docker & Docker Compose
- **Pilihan 2 (Manual)**: Go 1.22+ (jika menjalankan langsung dari source code)

---

## Panduan Instalasi & Menjalankan

### Cara 1: Langsung Jalankan via Docker (Instan / Zero-Setup)

Pengguna tidak perlu clone repository atau compile source code. Cukup jalankan image resmi dari GitHub Container Registry (GHCR):

```bash
# Menggunakan versi spesifik (misal: v0.1.1 atau agyrouter0.1.1)
docker run -d \
  --name antigravity-gateway \
  -p 3456:3456 \
  -v $(pwd)/data:/app/data \
  --restart unless-stopped \
  ghcr.io/asciii-devenv/adityaguyrouter:v0.1.1

# Atau selalu gunakan versi stabil terbaru (latest)
docker run -d \
  --name antigravity-gateway \
  -p 3456:3456 \
  -v $(pwd)/data:/app/data \
  --restart unless-stopped \
  ghcr.io/asciii-devenv/adityaguyrouter:latest
```

Buka di browser:
👉 **http://localhost:3456**

---

### Cara 2: Menggunakan Docker Compose (Build Lokal)

1. **Clone Repository**:
   ```bash
   git clone https://github.com/ASCIII-DevEnv/adityaguyrouter.git
   cd adityaguyrouter
   ```

2. **Jalankan Layanan**:
   ```bash
   docker compose up -d --build
   ```

3. **Buka Dashboard**:
   Akses di browser:
   👉 **http://localhost:3456**

---

### Cara 3: Menjalankan Langsung dengan Go (Tanpa Docker)

1. **Clone Repository**:
   ```bash
   git clone https://github.com/ASCIII-DevEnv/adityaguyrouter.git
   cd adityaguyrouter
   ```

2. **Jalankan Aplikasi**:
   ```bash
   go run .
   ```
   *(Atau compile terlebih dahulu: `go build -o antigravity-gateway . && ./antigravity-gateway`)*

3. **Buka Dashboard**:
   Akses di browser:
   👉 **http://localhost:3456**

---

## Panduan Penggunaan

### 1. Menghubungkan Akun Google
1. Buka tab **Provider** di dashboard.
2. Klik tombol **"Login Akun Google"** untuk mengautentikasi via Google OAuth.
3. Setelah login berhasil, akun Anda akan terdaftar dan kuota langsung dipantau secara otomatis.

### 2. Membuat API Key untuk Aplikasi / IDE
1. Buka tab **Home** lalu menuju bagian **Manajemen API Keys**.
2. Klik **"Buat API Key"**, beri nama (misal: `Cursor IDE` atau `Hermes Agent`), dan simpan API key yang digenerate (format: `ag_live_...`).

### 3. Menggunakan API Gateway di Alat AI Anda

Gunakan base URL dan API Key pada konfigurasi client Anda:

#### Contoh Konfigurasi OpenAI Client (Python / Node.js):
- **Base URL**: `http://localhost:3456/v1`
- **API Key**: `ag_live_xxx` (atau key yang Anda buat)
- **Model**: `gemini-3.8-flash-tiered`, `claude-sonnet-4-6`, `claude-opus-4-6-thinking`, atau `gpt-oss-120b-medium`

#### Contoh cURL:
```bash
curl http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ag_live_xxx" \
  -d '{
    "model": "gemini-3.8-flash-tiered",
    "messages": [{"role": "user", "content": "Halo! Siapa namamu?"}]
  }'
```

---

## Variabel Lingkungan (Environment Variables)

Aplikasi dapat dikonfigurasi menggunakan variabel lingkungan opsional:

| Variabel | Default | Keterangan |
| :--- | :--- | :--- |
| `PORT` | `3456` | Port HTTP gateway dan dashboard web |
| `DATA_DIR` | `./data` | Direktori penyimpanan berkas JSON (akun, key, skills, fallbacks) |

---

## Struktur Direktori

```text
├── internal/          # Logika backend Go (server, router, provider, storage, skills)
├── public/            # Frontend web UI
│   ├── assets/        # CSS, JS modular, icons.svg, dan logo.png
│   └── partials/      # Komponen HTML dinamis (analytics, usage, provider, dll.)
├── data/              # Database file lokal JSON (diabaikan oleh git)
├── Dockerfile         # Multi-stage Docker build ultra-ramping
├── docker-compose.yml # Konfigurasi container service
└── README.md          # Dokumentasi proyek
```

---

## Keamanan Data Pribadi

- Berkas token OAuth, credential, dan API key disimpan secara lokal di folder `data/` dan telah dimasukkan ke dalam `.gitignore`.
- Tidak ada data akun atau token yang dikirim ke server pihak ketiga manapun selain langsung ke endpoint resmi Google Cloud Code.

---

## Lisensi

[MIT License](LICENSE)
