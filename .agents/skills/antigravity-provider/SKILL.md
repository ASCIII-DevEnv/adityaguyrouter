---
name: antigravity-provider
description: "Use when integrating Antigravity APIs, quotas, or LiteLLM."
version: 1.1.0
author: Hermes Agent
license: MIT
platforms: [linux]
metadata:
  hermes:
    tags: [antigravity, google-oauth, quota-tracking, gemini-code-assist, ai-provider, litellm, a2a]
    related_skills: [local-ai-gateway, hermes-agent]
---

# Antigravity Provider & Quota Tracking

Prosedur dan spesifikasi integrasi upstream provider Antigravity (Google Cloud Code / Gemini Code Assist).

Lihat [references/litellm-provider-integration.md](references/litellm-provider-integration.md) untuk detail arsitektur integrasi native provider Antigravity di LiteLLM Proxy / SDK dan mitigasi pitfall-nya.

## 7. Mitigasi Latensi & Waiting on Provider di Gateway Multi-Akun

Ketika menggunakan custom gateway (`antigravity-gateway` / `antigravity-quota`):
- **Akar Gejala "Waiting on Provider"**:
  - *Session Affinity Imbalance*: Sesi panjang mengunci request ke 1 akun (`pool.go`). Akun pertama kebanjiran ratusan request sementara akun lain idle, memicu upstream rate-limit atau `context canceled`.
  - *Traffic Pacer Queueing*: Modul leaky-bucket (`pacer.go`) menahan request ketika burst habis (rate 0.5 req/s, min physical gap 150ms), serta artificial delay seperti `applyHumanJitter` (150-350ms).
- **Prosedur Penanganan & Optimasi Performa**:
  - **Pacer & Jitter Tuning**: Untuk agent interaktif multi-step tool call, perlonggar leaky bucket menyerupai CLI resmi (`agy`): rate `5.0` req/s, burst `15.0`, min gap `30ms`, dan batasi jitter maksimal `0-25ms` agar tidak ada sleep delay buatan yang menyebabkan timeout atau waiting status.
  - **Streaming WriteTimeout di Go**: Setel `WriteTimeout: 0` (unlimited) pada `http.Server` gateway. Nilai timeout statis (misal 120s) memutus koneksi streaming SSE di detik ke-120 saat model melakukan reasoning atau mengeluarkan respons panjang.
  - **Forwarding Usage Metadata pada Stream SSE**: Petakan `usageMetadata` dari Google CloudCode ke field `usage` pada chunk final OpenAI (`prompt_tokens`, `completion_tokens`, `total_tokens`, `cached_tokens`) sebelum `[DONE]`. Tanpa ini, client/agent (seperti Hermes `/status`) akan mencatat konsumsi token sebagai 0.
  - **Proteksi Avatar Hotlinking Google**: Gambar avatar akun Google (`lh3.googleusercontent.com`) diblokir HTTP 403 Forbidden jika browser mengirim header `Referer`. Pasang `<meta name="referrer" content="no-referrer">`, setel atribut `referrerpolicy="no-referrer"` pada tag `<img>`, dan sertakan HTTP response header `Referrer-Policy: no-referrer` dari router gateway.
  - **Routing Fill First & Fallback Model**:
    - **Routing & Fallback**:
      - `fill_first`: Pakai 1 akun tuntas hingga 0% atau error HTTP 429 sebelum beralih ke akun berikutnya guna mempertahankan prompt cache. Bypass ambang pre-emptive parking 15% pada mode ini.
      - `smart_quota`: Gunakan ambang batas aman 15% agar akun istirahat dan jendela 5 jam sempat pulih.
      - Router wajib menguji seluruh akun pada model utama terlebih dahulu sebelum mengeksekusi fallback model yang lebih mahal (misal Claude/GPT).
    - **Streaming Timeout (`WriteTimeout`)**: Pada Go `http.Server` yang melayani SSE streaming LLM, set `WriteTimeout: 0` (unlimited) agar streaming reasoning/konteks besar tidak diputus paksa di tengah jalan dengan error `context canceled`.
    - **Google User Avatar CDN Hotlinking (HTTP 403)**: Avatar Google (`lh3.googleusercontent.com`) memblokir permintaan jika membawa header `Referer` origin lokal. Wajib pasang `<meta name="referrer" content="no-referrer">`, tag `img` dengan `referrerpolicy="no-referrer"`, dan header respons HTTP `Referrer-Policy: no-referrer`.
  - **Urutan Build Container**: Lakukan seluruh modifikasi kode dan aset web sebelum memicu build Docker, verifikasi diff, baru jalankan build & restart container sekali jalan.
  - Ubah routing strategy ke `least_used` atau `round_robin` jika ingin beban request dibagi merata:
    ```bash
    curl -s -X POST http://localhost:3456/api/provider/strategy -H "Content-Type: application/json" -d '{"strategy":"least_used"}'
    ```
  - Cek distribusi beban akun secara berkala via `GET /api/accounts`.
  - Bersihkan sesi chat agent (`/new` atau `/compact`) untuk mereset session affinity lock.


- **Client ID Publik (IDE/CLI)**:
  - Client ID: Tersedia di `internal/config/config.go`
  - Client Secret: Tersedia di `internal/config/config.go`
  - Scopes:
    - `https://www.googleapis.com/auth/cloud-platform`
    - `https://www.googleapis.com/auth/userinfo.email`
    - `https://www.googleapis.com/auth/userinfo.profile`
    - `https://www.googleapis.com/auth/cclog`
    - `https://www.googleapis.com/auth/experimentsandconfigs`
- **Base URLs**:
  - `https://daily-cloudcode-pa.googleapis.com` (primer)
  - `https://cloudcode-pa.googleapis.com` (fallback)
- **Fingerprint Headers**:
  - `User-Agent: antigravity/ide/2.11.0 darwin/arm64`
  - `X-Client-Name: antigravity`
  - `X-Client-Version: 2.11.0`

## 2. Project ID Discovery (`loadCodeAssist`)

Sebelum request quota atau chat, ambil project ID aktif via:
- **Endpoint**: `POST https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist`
- **Headers**: `Authorization: Bearer <access_token>`, `User-Agent`, `x-request-source: local`
- **Body**: `{"metadata": {"ideType": 9, "platform": 3, "pluginType": 2}}` (3 = Linux x64, 2 = macOS arm64)

## 3. Integrasi LiteLLM & Protokol A2A

LiteLLM (`/home/lptik/Projects/litellm`) mendukung beberapa pola integrasi dengan Antigravity:

1. **Jalur OpenAI-Compatible (Zero-Code / Rekomendasi)**:
   - Manfaatkan gateway lokal (OmniRoute di `http://127.0.0.1:20130/v1`) yang sudah menangani Google OAuth, refresh token, dan fingerprinting.
   - Konfigurasi LiteLLM Proxy (`config.yaml`):
     ```yaml
     model_list:
       - model_name: antigravity-gemini
         litellm_params:
           model: openai/antigravity/gemini-2.5-pro
           api_base: http://127.0.0.1:20130/v1
           api_key: dummy
     ```

2. **Jalur Native Provider (`litellm/llms/antigravity/`)**:
   - Memerlukan implementasi Google OAuth token manager (auto-refresh access token via Google OAuth endpoint).
   - Inisialisasi discovery via `POST /v1internal:loadCodeAssist` untuk memperoleh project ID aktif.
   - Translasi payload OpenAI format ke `streamGenerateCode` / `generateCode` Cloud Code upstream dengan fingerprint headers.
   - Daftarkan provider ke registry di `litellm/types/router.py` dan `litellm/main.py`.

3. **Jalur A2A (Agent-to-Agent Protocol)**:
   - LiteLLM telah mengimplementasikan protokol A2A di `litellm/llms/a2a/` berbasis JSON-RPC 2.0 (`message/send` dan `message/stream`).
   - Endpoint menerima request JSON-RPC 2.0 pada base URL dengan parameter pesan `{kind: "message", role: "user", parts: [...]}`.
   - Antigravity dapat berperan sebagai agent worker backend jika dibungkus service JSON-RPC 2.0 A2A dan dipanggil via `custom_llm_provider: a2a`.

## 5. Gateway UI & Replay Inspector Best Practices
- **Penegakan Utilitas Tailwind CSS v4 & Zero Inline CSS**:
  - Dilarang keras menggunakan CSS inline statis (`style="..."`) atau CSS native monolitik berlebihan pada markup HTML statis maupun komponen yang di-render dinamis via JavaScript (tabel, kartu, badge, modal). Seluruh styling wajib menggunakan utility class murni **Tailwind CSS v4** (`flex`, `grid`, `gap-*`, `rounded-*`, `border-*`, `bg-*`, `text-*`, `p-*`, dsb).
  - **Penetapan Tipe Stylesheet Tailwind v4 Browser**: Saat menggunakan runtime browser Tailwind (`@tailwindcss/browser@4`), blok `<style>` kustom yang mendefinisikan `@theme` dan `@utility` WAJIB menggunakan atribut `type="text/tailwindcss"`. Penggunaan atribut non-standar seperti `<style type="text/theme">` atau `<style>` biasa menyebabkan parser Tailwind mengabaikan seluruh stylesheet kustom, melumpuhkan utilitas tata letak, dan memicu *catastrophic unstyled layout failure* (tampilan teks mentah tanpa gaya CSS).
  - Dilarang memanipulasi styling dinamis via properti inline JS `element.style.color = '...'` atau atribut `style="..."`; gunakan manipulasi class utilitas Tailwind (`classList.add(...)`, `classList.remove(...)`, atau class dinamis utilitas Tailwind).
- **Arsitektur Aset Sprite SVG Terpusat (`public/assets/icons.svg`)**:
  - Jangan menanam ratusan baris definisi simbol SVG langsung di dalam file HTML utama (`index.html`). Simpan seluruh definisi simbol SVG pada berkas sprite terpusat di `public/assets/icons.svg`.
  - Muat sprite secara asinkron di awal dokumen ke container tersembunyi `<div id="svgSpriteContainer" class="hidden"></div>` via `fetch('./assets/icons.svg')` agar seluruh pemanggilan `<use href="#icon-...">` di partials dan template JS berfungsi secara native tanpa kendala CORS atau fragment identifier browser.
- **Normalisasi Selektor CSS & Global Window Binding pada Refaktorisasi Modular**:
  - Di dalam `@utility` Tailwind CSS v4, aturan nesting dengan pseudo/state modifier seperti `&.active` tidak terkompilasi menjadi selektor majemuk standar (`.tab-content.active`). Gunakan selektor CSS standar langsung (`.tab-content { display: none; } .tab-content.active { display: block; }`) pada berkas `assets/css/style.css` agar status visibilitas tab dan modal aktif berfungsi sempurna.
  - Saat mengekstrak logika monolitik ke berkas eksternal (`assets/js/app.js`), ikat secara eksplisit fungsi event handler interaktif ke scope global `window` (`window.switchTab = switchTab;`, `window.ensureTabLoaded = ensureTabLoaded;`, dsb.) agar atribut inline `onclick` pada markup HTML tetap dapat memanggil fungsinya.
  - **Larangan Nesting Ilegal Tag Gambar & SVG**: Dilarang membungkus `<img>` di dalam tag `<svg class="icon">` (seperti `<svg class="icon"><img src="./assets/logo.png"/></svg>`); render gambar logo harus berada di dalam tag kontainer blok/div biasa (misal `<div class="brand-icon"><img .../></div>`).
  - **Integritas ID Simbol SVG**: Pastikan penamaan icon pada elemen UI parsial (seperti tombol refresh atau reset) konsisten dengan ID yang dideklarasikan pada sprite `icons.svg` (misal gunakan `#icon-refresh-cw`, bukan simbol yatim piatu tak terdefinisi `#icon-refresh` atau `#icon-history`).
  - Pengecualian atribut `style` hanya diperbolehkan untuk nilai dinamis murni berbasis runtime yang tidak dapat diprediksi secara statis (seperti persentase kalkulasi progres bar: `style="width: ${percent}%;"`).
  - Pastikan seluruh interpolasi string dinamis atau pesan error selalu disanitasi menggunakan `escapeHtml()` sebelum diinjeksi ke template string DOM guna mencegah kerentanan XSS.
- **Filter Multi-Dimensi Analitik Token & Eliminasi Kanvas Yatim Piatu**:
  - Pada halaman analitik token (`/api/analytics/tokens`), sediakan filter dropdown model (`#analyticsModelSelect`) di samping kontrol rentang hari (`#analyticsDaysSelect`). Teruskan parameter query `&model=${encodeURIComponent(selectedModel)}` ke backend agar seluruh kartu KPI, grafik efisiensi token saver, diagram tren harian, dan tabel riwayat transaksi tersaring secara deterministik.
  - Isi opsi model dropdown secara dinamis dari array `data.byModel` dengan fallback model default gateway.
  - Hapus elemen kanvas yatim piatu (seperti `#modelTokenDistributionChart` yang tidak pernah di-render script Chart.js) guna mengeliminasi dead DOM footprint.
- **Defensive Model & API Response Accessor (Anti-Runtime Breakage)**:
  - Pada API response `/api/accounts`, objek model di bawah `families[family].models` memiliki properti `id`, `displayName`, `remainingPercent`, `resetAt` (tidak menjamin key `name`). Memanggil `m.name.replace(...)` secara langsung memicu fatal runtime `TypeError: Cannot read properties of undefined (reading 'replace')` yang menggagalkan seluruh render kartu kuota usage (`renderUsage`). Selalu gunakan defensive fallback: `(m.displayName || m.id || m.name || '').replace('models/', '')` dan `Math.round(m.remainingPercent ?? 0)`.
  - Pada endpoint `/api/analytics/tokens`, data transaksi terakhir disajikan backend Go dalam properti `data.recentLogs` (bukan `recentTransactions`). Selalu terapkan fallback ganda: `data.recentLogs || data.recentTransactions || []` agar tabel audit transaksi tidak kosong.
- **Canvas Chart & Asynchronous Safety**:
  - Sinkronkan ID elemen canvas HTML (`tokenSaverComparisonChart`, `modelTokenDistributionChart`, `dailyTokenTrendChart`) dengan pemanggilan JavaScript agar tidak terjadi silent no-op.
  - Selalu periksa ketersediaan library Chart (`typeof Chart !== 'undefined'`) sebelum menginisialisasi `new Chart(...)` guna menghindari unhandled `ReferenceError` jika script CDN dimuat secara asinkron atau dalam runtime testing.
- **SPA Tab Navigation, Hash Routing & Tailwind v4 Display Pitfalls**:
  - Hindari nesting `&.active` di dalam `@utility tab-content` pada Tailwind CSS v4 CDN runtime; sintaks tersebut sering tidak terkompilasi menjadi selector spesifisitas majemuk. Gunakan CSS class selector murni dengan proteksi spesifisitas tegas:
    ```css
    .tab-content { display: none !important; }
    .tab-content.active { display: block !important; animation: fadeIn 0.25s cubic-bezier(0.16, 1, 0.3, 1); }
    ```
  - Selalu kaitkan perubahan tab (`switchTab`) ke URL Hash (`window.location.hash` / `history.pushState`) dan pasang listener `window.addEventListener('hashchange', ...)` agar URL deep link (seperti `/#analytics`, `/#usage`) dapat di-refresh atau di-bookmark tanpa mengembalikan pengguna ke halaman kosong.
  - Bungkus inisialisasi awal navigasi tab dan routing hash dalam pengecekan kesiapan DOM (`document.readyState === 'loading' ? document.addEventListener('DOMContentLoaded', ...) : ...`) untuk menjamin semua kontainer modul/partial sudah terpasang sebelum render data dijalankan.
  - Setiap pemisahan skrip modular dari HTML monolitik ke berkas eksternal (misal `app.js`), bind semua fungsi callback handler yang dipanggil via atribut inline HTML (`onclick="switchTab(...)"`, `onclick="copyApiKey(...)"`) secara eksplisit ke objek global `window` (`window.switchTab = switchTab;`).
  - Pastikan favicon dideklarasikan secara eksplisit di tag `<head>` (`<link rel="icon" type="image/png" href="./assets/logo.png">`) agar browser tidak menghasilkan status blank/error 404 untuk favicon default.
- **Deterministic cURL Generation**:
  - Hindari menanam payload JSON berukuran besar langsung ke dalam inline attribute tag HTML (`onclick="..."`). Petik ganda/tunggal dan karakter unescaped merusak parser DOM browser dan menyebabkan tombol copy tidak merespons tanpa error yang jelas di konsol.
  - Pisahkan data record ke in-memory array/store dan panggil fungsi via indeks unik (`copyInspectorCurl(idx)`).
  - Format cURL output wajib manusiawi (*human-readable*): terapkan `JSON.stringify(parsed, null, 2)` pada body request dan escape single quotes bash (`'\''`) sebelum disalin ke clipboard atau textarea fallback.
- **Hygiene Badge & Sinkronisasi Model**:
  - Jangan gunakan badge statis redundan ("AI Gateway Active", "Official Provider", "Ring Buffer 60", "Aktif") atau merender badge soup panjang untuk daftar default skills pada kartu API key.
  - Saat merotasi atau memperbarui model (misal menghentikan model legacy dan menaikkan versi Claude Sonnet/Opus ke versi terbaru), sinkronkan secara serentak di registry `/v1/models`, kamus fallback rules, playground selector, dan analitik dashboard (lihat [references/dashboard-ui-guidelines.md](references/dashboard-ui-guidelines.md)).

- **Pewarnaan Kolom Data Tabel & Konflik CSS Global**:
  - Hati-hati terhadap aturan CSS elemen global seperti `td { color: var(--text-main); }` pada stylesheet utama. Aturan tersebut memiliki spesifisitas yang menimpa kelas warna utilitas teks Tailwind CSS (seperti `text-purple-600`, `text-blue-600`, `text-emerald-600`). Hapus penimpaan properti `color` pada tag `td` agar styling warna utilitas teks dapat diterapkan secara langsung pada baris data.
- **Transparansi Brand Icon & Favicon**:
  - Hindari membungkus logo PNG dengan gradien background kontras atau efek bayangan tebal di header jika logo sudah memiliki aksen bentuk tersendiri. Gunakan wrapper transparan (`background: transparent; box-shadow: none;`) dan class `object-contain`.
  - Selalu sertakan `<link rel="icon" type="image/png" href="./assets/logo.png">` di `<head>` agar browser menampilkan favicon resmi secara konsisten.
- **Tampilan Waktu Reset Kuota Multi-Akun**:
  - Setiap progress bar kuota (5 jam sesi dan mingguan) wajib dilengkapi indikator waktu reset lokal (`resetAt`) serta durasi relatif (misal: `Reset: 16:09 (2j 45m lagi)` atau `Reset: 20:29 (segera)`) menggunakan utilitas `formatResetTime()` agar pengguna mengetahui secara pasti sisa jendela rolling kuota.
- **Sinkronisasi Otomatis Model Fallback dari `/v1/models` & Auto-Save**:
  - Batasi model pada rantai fallback hanya pada model-model upstream yang benar-benar aktif di registry gateway (`gemini-3.8-flash-tiered`, `claude-opus-4-6-thinking`, `claude-sonnet-4-6`, `gpt-oss-120b-medium`), hindari mencemari pilihan autocomplete pengguna dengan varian sub-tier internal atau model yang belum tersedia di upstream.
  - Selalu gabungkan daftar model dari `GET /v1/models` dan `GET /api/fallbacks` ke dalam `<datalist id="fallbackModelOptions">` agar seluruh model yang didukung sistem (termasuk model yang baru ditambahkan) otomatis tersedia sebagai pilihan autocomplete pada rantai fallback.
  - Setiap mutasi rantai fallback (tambah rantai, tambah step model, hapus step) harus memicu auto-save ke backend (`POST /api/fallbacks`) secara instan dan sediakan tombol penyimpanan eksplisit "Simpan Semua Pengaturan" pada header.
- **Skill Lifecycle, Modal Dialog & Strict Two-Step Deletion**:
  - **Peniadaan Dialog Prompt Browser**: Dilarang menggunakan dialog `prompt()` bawaan browser untuk pembuatan skill prompt atau API keys; sediakan form modal terdedikasi (`.modal-backdrop` + `.modal-dialog`) dengan input nama, deskripsi, textarea system prompt monospace, serta penanganan submit/error langsung.
  - **Strict Entity Name Verification Guard**: Pada aksi penghapusan permanen skill atau entity penting, modal konfirmasi wajib mewajibkan pengguna mengetik nama entitas secara persis (`verifyText: name`), bukan teks generik `HAPUS`, guna mencegah kesalahan klik destruktif.
  - **Pruning Otomatis Playground Combo Skills**: State pill combo skills di Playground wajib memvalidasi ketersediaan dan status aktif skill (`sk.enabled`). Jika suatu skill dinonaktifkan atau dihapus oleh pengguna, pangkas ID skill tersebut dari `selectedComboSkills` secara deterministik agar tidak terinjeksi ke prompt Playground.
  - **Eliminasi Kanvas Yatim Piatu & Ikon Ghost**: Periksa dan bersihkan elemen DOM helper (seperti datalist yang tidak dipakai atau elemen ikon SVG yang ID simbolnya tidak ada di `icons.svg`) agar DOM tree tetap bersih dan bebas dead-code footprint.

## 6. Context Caching & Secret Redaction
- **Prompt Cache Alignment**:
  - Google Gemini / Cloud Code context caching bergantung pada kesamaan byte prefix awal secara identik.
  - Urutkan deklarasi tools (`FunctionDeclarations`) secara alfabetis deterministik sebelum dikirim ke Google untuk memaksimalkan rasio cache hit (>80%).
- **Outbound Secret Shield**:
  - Selalu jalankan pemindaian regex pada prompt keluar (API Key, JWT, DB connection strings, token).
  - Redact ke placeholder acak `[REDACTED_SECRET_<HASH>]` sebelum meluncur ke upstream dan pulihkan kembali (*unredact*) saat streaming/response diterima agen lokal.

## 7. Model ID vs Display Label Mapping (Pitfall Upstream Discovery)
- **Upstream Model IDs Tetap Konservatif**:
  - Google Cloud Code upstream (`cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels`) mempertahankan ID internal stabil (misalnya `claude-sonnet-4-6`, `claude-opus-4-6-thinking`).
  - Jangan mengirimkan alias versi publik/eksternal yang belum terdaftar di upstream (seperti `claude-sonnet-5-5` langsung ke Google), karena API upstream akan menolak dengan HTTP 404 / 400.
  - Sediakan layer alias deterministik pada gateway jika ingin mendukung request client berlabel baru (`claude-sonnet-5-5` -> `claude-sonnet-4-6`), namun pastikan transparansi kepada pengguna bahwa inferensi upstream tetap dieksekusi oleh model ID Google yang aktif.

## 8. Gemini Tier Resolution (Complexity Classifier & Effort)
- Penentuan tingkat model Gemini 3.8 (`low`, `medium`, `high`) berjalan di dua layer:
  1. **Hermes/OmniRoute Client Layer**: Parameter `effort` (low/medium/high) diteruskan via request completions. Flag `/reasoning high` memicu pengiriman `effort=high` dengan penalaran lebih mendalam.
  2. **Gateway Complexity Classifier**: Menganalisis isi prompt secara otomatis:
     - `high`: Panjang > 3.000 karakter, memuat >= 3 blok kode (` ``` `), keyword arsitektur (*concurrency, deadlock, security audit, microservice*), atau percakapan >= 8 turn.
     - `medium`: Panjang 600–3.000 karakter, memuat 1 blok kode, atau keyword umum (*debug, fix bug, database, query, api*).
     - `low`: Pesan singkat / sapaan < 600 karakter.

## 10. Optimasi Konsumsi Token & Kuota Mingguan Upstream (Gemini Code Assist Mechanics)
- **Token Penalaran Tersembunyi (Hidden Thinking Tokens)**:
  - Upstream Antigravity (Google Cloud Code / Gemini Code Assist) menagih internal thinking/reasoning tokens ke batas kuota mingguan akun.
  - Setelan `reasoning_effort: high` menghasilkan 4.000–16.000 token internal thinking per request. Gaya keluaran ringkas (seperti Caveman) hanya memangkas token output yang tampak di layar, bukan token thinking internal di hulu.
  - Untuk tugas koding rutin harian, gunakan `reasoning_effort: medium` atau `low` (`/reasoning medium` di client) dan cadangkan `high` hanya untuk perancangan arsitektur kritis atau audit keamanan mendalam.
- **Efek Penggandaan Subagent (`delegate_task`)**:
  - Hindari delegasi subagent untuk tugas mekanikal sederhana. Setiap subagent menginisialisasi sesi baru lengkap dengan system prompt, skema tools aktif, dan reasoning effort mandiri.
  - Pola N subagent dengan M iterasi mengalikan pemanggilan API (N x M), menghabiskan kuota multi-akun secara cepat.
  - Gunakan `execute_code` (eksekusi Python lokal) untuk inspeksi multi-file dan kompresi data: berjalan di mesin lokal (0 API token) dan hanya mengembalikan hasil akhir yang ringkas ke konteks model.
- **Akumulasi Konteks Sesi**:
  - Bersihkan sesi secara teratur dengan `/new` setelah menyelesaikan topik kerja agar prompt tidak terus-menerus mengirim ulang riwayat puluhan ribu input token dari percakapan lama.

## 9. Grill-Me Skill, Zero-Typing Interactive UI & Autonomous Master Prompt Architecture
- **Pola Multi-Stage Interrogation & Autonomous Master Prompt**:
  1. **Fase Wawancara Terstruktur (Deep Multi-Round)**:
     - Jangan langsung meng-generate kode atau master prompt prematur pada ide awal pengguna.
     - Ajukan minimal 5 hingga 6 pertanyaan mendalam per ronde: Identitas/Nama aplikasi, Tech Stack (Zero-Asset vs Libraries), Data Model & Storage, Visual/Tema & Web Audio, Formula/Kalkulasi inti, dan Edge Cases/Testing.
     - Setiap pertanyaan memuat opsi pilihan `[A]`, `[B]`, `[C]`, dan WAJIB menyediakan opsi **Write-in / Kustom** di akhir agar pengguna dapat memasukkan spesifikasi custom tanpa batas.
     - **Penjaminan Rekomendasi & Format Bersih**: Wajib menyertakan minimal 1 rekomendasi konkret untuk setiap pertanyaan. Format pertanyaan dilarang menggunakan markdown asterisks ganda (`**`) di judul agar parser tidak menyisakan simbol mentah.
     - **Tampilan Rekomendasi Badge-Only**: Indikator rekomendasi cukup ditampilkan sebagai badge ringkas pada opsi bersangkutan (`Rekomendasi`), tanpa menduplikasi kotak teks penjelasan rekomendasi di bagian bawah kartu opsi.
     - **Dukungan Rekomendasi Kombinasi (Multi-Select)**: Jika rekomendasi memuat lebih dari satu opsi (misal `[A] + [D]`) atau judul memuat multi-pilihan, render form wajib menggunakan checkbox (`input[type="checkbox"]`), bukan radio button tunggal.
     - **Deduplikasi Opsi Write-in**: Jika pertanyaan model sudah menyediakan opsi write-in bawaan, sematkan input teks langsung di dalam opsi tersebut dan jangan menambahkan opsi write-in duplikat di bawah.
     - Jika konteks formula, workflow, atau spesifikasi masih belum tuntas, jalankan **Ronde 2** sebelum merumuskan Master Prompt akhir.
  2. **Zero-Typing Interactive Selection Card Pattern (Antigravity Style)**:
     - Parser frontend mendeteksi token interogasi (`❓ Q1`, `[A]`, `[B]`, `➡️ Rekomendasi`).
     - Alih-alih merender teks mentah yang memaksa user mengetik jawaban, render menjadi kartu interaktif dengan radio buttons per opsi.
     - Rekomendasi model otomatis tercentang (*pre-selected*), dilengkapi tombol cepat "Pilih Semua Rekomendasi" (1-klik) dan tombol "Kirim Pilihan & Generate Master Prompt".
     - Bila opsi Write-in dipilih, tampilkan secara dinamis input teks inline khusus untuk menampung spesifikasi pengguna dan satukan ke dalam payload saat submit.
     - Submit form otomatis menginjeksi sintesis pilihan ke input chat dan menembakkannya ke API tanpa user perlu mengetik manual.
  3. **Fase Formulasi Master Prompt**: Setelah jawaban terkumpul, hasilkan prompt otonom terstruktur:
     - Header: `Build "<PROJECT_NAME>": ... Work fully autonomously from start to finish.`
     - `AUTONOMY RULES (critical)`: Mandiri tanpa interupsi, larangan bertanya balik, cari workaround sendiri.
     - `TECH & ASSET RULES / HARD RULES`: Larangan aset download, procedural generation, canvas shaders, Web Audio API synthesis.
     - `THE SYSTEM / THE DOMAIN`: Logika inti bisnis, formula, model entitas, state.
     - `INTERACTIVITY & CONTROLS / WHAT I CAN DO`: Kontrol kamera, mode inspeksi, grafik dan metrik real-time.
     - `PRESENTATION & POLISH`: Estetika visual, tema, audio ambient, performa 60 FPS.
     - `QUALITY BAR & SELF-TESTING`: Pengujian otomatis headless browser (Playwright), 2 siklus perbaikan, file README.
- **UI Sizing, Streaming Control & Action Bar**:
  - Elemen chat input pada antarmuka Playground wajib menggunakan styling full-width (`width: 100%`, `flex: 1`) agar nyaman untuk prompt bertaraf enterprise/panjang.
  - Saat respons AI sedang di-stream (`stream: true`), tombol kirim wajib berubah menjadi tombol Stop (`icon-square`, warna merah) dan memicu `.abort()` pada `AbortController` saat diklik untuk mencegah pengiriman request ganda. Input chat dinonaktifkan (`disabled = true`) selama streaming berlangsung.
  - Setiap respons AI di Playground wajib dilengkapi tombol aksi cepat: **Salin** (clipboard API) dan **Export .md** (Blob download berkas markdown) pada header gelembung chat.
  - **SVG Rendering Pitfall**: Jangan menanamkan ekspresi template string JS seperti `${svgIcon('play', ...)}` ke dalam markup HTML statis murni karena browser tidak mengevaluasinya dan akan merender teks mentah. Gunakan tag SVG native: `<svg class="icon icon-sm"><use href="#icon-play"></use></svg>`.
- **Frontend & Builtin Engine Sync**:
  - Daftarkan ID skill di `GetBuiltinSkills()` pada backend Go (`internal/skills/engine.go`) dan sinkronkan dengan pill selector Playground di frontend (`#pgSkillsPills`).
- **Antigravity Interactive Question Cards & Custom Write-in**:
  - Saat skill interogasi (seperti `grill-me`) aktif, format respons interogasi dengan parser DOM frontend yang mengubah bullet text pertanyaan (`❓ Q1`, `[A]`, `[B]`, `[C]`, `➡️ Rekomendasi`) menjadi kartu interaktif.
  - **Sanitasi Total Karakter Markdown**: Bersihkan seluruh variasi asterisk bold/italic (`**...**`, `*...*`, backtick) serta sisa asterisks ganda/tunggal tak berpasangan dari judul pertanyaan dan opsi. Jangan biarkan simbol markdown mentah merusak tampilan UI.
  - **Pencegahan False-Positive Awalan Opsi**: Gunakan regex ketat yang memeriksa bracket eksplisit `^[-*•\d.]*\s*\[([A-Z])\]\s*[:-]?\s*(.*)` agar kata bahasa alami di awal baris (seperti "Berapa", "Bagaimana") tidak keliru dideteksi sebagai opsi `[B] erapa`.
  - **Kontrol Input Dinamis (Radio vs Multi-Select Checkbox)**: Jika rekomendasi memuat lebih dari 1 opsi (misal: `[A] + [D]`) atau judul pertanyaan mengandung kata kunci pilihan ganda (*multi-select* / kombinasi), kontrol input WAJIB dinamis beralih ke tipe `checkbox` (bukan dipaksa `radio` button) dengan penanda badge `Multi-Select`.
  - **Deduplikasi Opsi Write-In**: Jika model sudah menyediakan opsi write-in (misal `[D] Write-in`), sistem tidak boleh menambahkan opsi write-in redundan kedua di bagian bawah. Sisipkan input teks kustom langsung ke dalam kartu opsi tersebut.
  - **Penjaminan Minimal 1 Rekomendasi & Badge-Only UI**: Setiap pertanyaan wajib memiliki minimal 1 rekomendasi yang ditandai secara visual HANYA melalui badge `Rekomendasi` pada opsi terkait (tanpa kartu teks penjelasan di bawah yang memakan ruang), dengan fallback otomatis ke opsi `[A]` jika model tidak menyediakan rekomendasi.
  - **Streaming Preview vs Deferred Compilation**: Saat teks AI sedang di-stream (`isStreaming = true`), parser kartu interaktif TIDAK BOLEH langsung mengompilasi markup form di setiap chunk potongan teks yang belum lengkap. Tampilkan preview teks mentah/bersih secara streaming terlebih dahulu dengan indikator kursor berdenyut, dan hanya kompilasi form kartu interaktif secara deterministik saat streaming selesai (`[DONE]`).
  - **Penegakan Rekomendasi Tunggal/Multi & Tipe Input Dinamis**:
    - AI wajib menyertakan minimal 1 rekomendasi pada setiap pertanyaan. Terapkan fallback otomatis ke opsi pertama `[A]` jika rekomendasi absen.
    - Rekomendasi hanya ditampilkan sebagai badge ringkas `Rekomendasi` pada opsi yang bersangkutan; hindari menampilkan blok teks rekomendasi terpisah di bawah opsi untuk menjaga UI tetap bersih.
    - Jika rekomendasi memuat lebih dari 1 opsi (misal `[A] + [D]`) atau judul mengindikasikan pilihan ganda, alihkan tipe input otomatis dari radio button (`input[type="radio"]`) menjadi checkbox (`input[type="checkbox"]`) dan sertakan badge `Multi-Select`.
  - **Eliminasi Opsi Write-In Ganda**: Periksa apakah opsi bawaan sudah memuat opsi bertipe write-in (`/write[- ]?in|tentukan sendiri|isi sendiri/i`). Jika sudah ada, sematkan input teks kustom langsung ke dalam kartu opsi tersebut dan jangan buat opsi write-in tambahan di bagian bawah.
  - Sediakan tombol auto-select rekomendasi (`Pilih Semua Rekomendasi`) dan tombol eksekusi 1-klik untuk merangkum seluruh pilihan pengguna ke model tanpa mewajibkan mengetik manual.
- **Streaming State & UI Controls**:
  - Saat request Playground dalam status streaming (`isStreaming`), ubah tombol Kirim secara deterministik menjadi tombol Stop (`AbortController.abort()`) dengan styling bahaya (`#ef4444`) dan nonaktifkan textarea/input untuk mencegah double submission.
  - Sediakan tombol aksi *Salin ke Clipboard* dan *Export to Markdown (.md)* pada setiap chat bubble respons asisten.
- **Mobile Floating Navigation & Dropdown Stacking**:
  - Dropdown navigasi mobile wajib menggunakan `position: absolute; top: calc(100% + 0.5rem); left: 0; right: 0; z-index: 1000;` dengan background solid pekat (`#ffffff`, hindari transparansi tembus pandang) agar tidak mendorong flow header ke bawah dan tidak membayangi elemen di baliknya.
  - Tambahkan penutup otomatis saat pengguna mengklik area luar header (*click-outside detection*).
- **Interactive Question Form (Grill-Me Pattern) & Robust Parsing**:
  - **Pembersihan Simbol Markdown & Eliminasi Redundansi**: Sanitasi teks pertanyaan, opsi, dan penjelasan rekomendasi dari seluruh artefak markdown (`**bold**`, `*italic*`, backtick, trailing asterisks) sebelum injeksi ke kartu DOM agar tampilan bersih tanpa sisa tanda bintang. Jangan tampilkan kotak teks penjelasan rekomendasi terpisah di bawah kartu; tampilkan rekomendasi secara elegan HANYA berupa badge `Rekomendasi` langsung pada opsi yang bersangkutan.
  - **Penjaminan Minimal 1 Rekomendasi**: Wajibkan minimal 1 opsi rekomendasi per pertanyaan di system prompt engine, serta pasang fallback frontend ke opsi pertama `[A]` jika AI tidak menandai rekomendasi, agar tombol auto-select selalu memiliki target valid.
  - **Regex Deteksi Judul & Strict Bracket Matching**: Deteksi judul pertanyaan menggunakan regex fleksibel `/(?:❓|\?|\bQ\d+[:\s\-]|\*\*Q\d+)/i` dan pastikan baris bukan opsi (`!/^[-*•\d.]*\s*\[[A-Z]\]/i`) guna menangani variasi unicode emoji `❓`. Untuk opsi pilihan, wajibkan format kurung siku ketat `/^[-*•\d.]*\s*\[([A-Z])\]\s*[:-]?\s*(.*)/i` agar kata biasa di awal baris (seperti "Berapa") tidak salah ditangkap sebagai opsi `[B] erapa`.
  - **Dukungan Rekomendasi Multi-Select (Checkbox vs Radio)**: Jika rekomendasi memuat lebih dari 1 opsi (misal `[A] + [D]`) atau judul memuat indikator multi-pilihan, alihkan input kontrol dari radio button ke checkbox (`input[type="checkbox"]`) dan tandai badge `Multi-Select`. Tombol auto-select rekomendasi wajib mencentang seluruh kombinasi opsi yang disarankan.
  - **Deduplikasi Opsi Write-In**: Periksa apakah opsi dari model sudah memuat write-in (`/write[- ]?in|tentukan sendiri|isi sendiri/i`). Jika sudah ada, sematkan input teks kustom langsung pada kartu opsi tersebut dan jangan menambahkan opsi write-in duplikat di bagian bawah.
  - **Isolasi Streaming Lifecycle vs Render Form**: Jangan jalankan parser form kartu interaktif saat streaming masih berlangsung (chunk parsial belum lengkap) karena form akan rusak/kosong dan chat tampak tidak membalas. Tampilkan preview teks bersih real-time dengan animasi kursor selama streaming, dan hanya kompilasi form kartu interaktif setelah stream selesai (`[DONE]`). Selalu picu final flush render DOM setelah stream selesai dibaca.
  - **Cache-Busting Statis Web Gateway**: Sertakan query string versi (misal `app.js?v=0.1.1` dan `style.css?v=0.1.1`) pada `index.html` agar pembaruan parser frontend tidak terhambat oleh disk cache browser.
  - **Dukungan Rekomendasi Multi-Select (Checkbox vs Radio)**: Jika rekomendasi memuat lebih dari 1 opsi (misal `[A] + [D]`) atau judul memuat kata multi/ganda, render kontrol input sebagai checkbox (`type="checkbox"`), bukan radio button, agar pengguna dapat memilih kombinasi opsi secara bersamaan.
  - **Deduplikasi Opsi Write-In**: Periksa apakah opsi bawaan sudah memiliki pilihan write-in (`/write[- ]?in|tentukan sendiri|isi sendiri/i`). Jika sudah ada, sematkan input teks langsung di dalam opsi tersebut dan jangan menambahkan opsi kustom kedua di bawahnya.
  - **Preservasi Intensi Scroll Pengguna saat Streaming**: Pada chat streaming, deteksi jika pengguna sedang men-scroll ke atas (`userScrolledUp`). Jalankan `scrollTop = scrollHeight` hanya jika pengguna berada di batas bawah scroll, mencegah pengguna ditarik paksa ke bawah saat membaca jawaban sebelumnya.
- **Telegram Bot Webhook Integration & Chat ID**:
  - Webhook Telegram (`https://api.telegram.org/bot<TOKEN>/sendMessage?chat_id=<CHAT_ID>`) membutuhkan parsing URL query parameter `chat_id` di backend dan pengiriman payload JSON `{ "chat_id": ..., "text": ..., "parse_mode": "Markdown" }`. Sediakan panduan lookup `chat_id` (via `@userinfobot`) dan pastikan pengguna telah mengirim `/start` ke bot sebelum menguji notifikasi.
  - Simpan destination URL webhook otomatis ke `localStorage` pada event `oninput` dan muat ulang saat inisialisasi agar input pengguna tidak hilang saat browser di-refresh.
  - Hindari penggunaan sintaks template literal JS (`${svgIcon(...)}`) di dalam markup HTML statis; gunakan tag SVG asli `<svg class="icon"><use href="#icon-name"></use></svg>` untuk mencegah teks mentah muncul di browser.
- **Tab Lifecycle, Dynamic Partials & URL Hash Routing Pitfalls**:
  - Saat memecah modul SPA menjadi AJAX partials (`/partials/<tab>.html`), hindari menjalankan handler render tab (`renderUsage()`, `renderProvider()`) sebelum dataset global (`fetchAccounts()`) selesai diunduh. Bungkus inisialisasi awal ke dalam fungsi boot asinkron yang meng-`await fetchAccounts()` sebelum memproses `handleHashChange()` dari URL hash.
  - Tambahkan fallback penjaga di dalam `switchTab(tabId)`: jika dataset akun masih kosong saat pengguna langsung membuka tab bertarget akun (`tab-usage`, `tab-provider`), jalankan `await fetchAccounts()` terlebih dahulu sebelum memanggil render agar antarmuka tidak blank.
  - Pastikan pemuatan partial dinamis (seperti `loadAnalyticsPartial()`) di-`await` secara penuh di dalam router sebelum fungsi komputasi metrik atau inisialisasi canvas Chart dijalankan, guna menjamin elemen container target (`#analyticsMount`) telah terpasang di DOM.
  - Hindari nesting pseudo-class CSS (`&.active`) di dalam directive `@utility` Tailwind v4 karena browser parser gagal mengekspansinya. Definisikan selektor status aktif secara eksplisit dengan CSS standar (`.tab-content { display: none !important; } .tab-content.active { display: block !important; }`).
  - **Penghindaran Spesifisitas Monolitik pada Elemen Tabel**: Hindari penegakan warna teks global seperti `td { color: var(--text-main); }` pada berkas CSS kustom, karena spesifisitas tag HTML tersebut akan menimpa utility class warna teks Tailwind v4 per kolom (`text-purple-600`, `text-amber-500`, `text-blue-600`, `text-emerald-600`).
- **Quota Reset Countdown & Fallback Model Synchronization**:
  - Tampilkan waktu reset lokal (`resetAt`) dan kalkulasi sisa durasi relatif di bawah progress bar kuota 5-jam dan mingguan pada masing-masing akun provider.
  - Pada halaman Fallback Chain, gabungkan daftar autocomplete model secara dinamis dari endpoint `GET /v1/models` dan `GET /api/fallbacks`, serta sediakan aksi simpan otomatis (`POST /api/fallbacks`) pada setiap mutasi rantai model.
- **Brand Logo & Clean Transparent Header**:
  - Kontainer brand logo pada navbar/header wajib menggunakan background transparan (`background: transparent !important; box-shadow: none !important; border: none !important;`) dengan `object-contain` pada elemen logo murni, menghindari penumpukan background gradien atau box shadow yang merusak estetika clean minimalism.
- **Multi-Tab Lifecycle & Dynamic Partials Async Boot**:
  - Pada arsitektur SPA yang memuat partial HTML secara dinamis (`/partials/*.html`), pastikan data global latar belakang (`fetchAccounts()`) selesai di-`await` sebelum memproses route hash URL aktif (`handleHashChange()`).
  - Lakukan pengecekan defensif di dalam fungsi `switchTab()`: jika state array data akun masih kosong saat pengguna langsung mengakses tab dependen (`usage`, `provider`), panggil `await fetchAccounts()` terlebih dahulu sebelum memicu render DOM agar halaman tidak kosong (blank).
- **Reverse Proxy HTTPS & OAuth Callback in Hosting**:
  - Saat meng-host aplikasi gateway di balik reverse proxy (Traefik / Nginx), jangan menggunakan `http://%s/callback` secara statis di backend. Gunakan pengecekan header `X-Forwarded-Proto` agar redirect URI Google OAuth tetap valid menggunakan skema HTTPS.
- **Tailwind CSS Specificity & Global Tag Overrides**:
  - Hindari deklarasi global CSS pada elemen tabel seperti `td { color: var(--text-main); }` di berkas CSS kustom karena akan menimpa utilitas pewarnaan teks spesifik Tailwind (`text-purple-600`, `text-emerald-600`, dll.).
  - Untuk visibilitas tab dinamis, gunakan selektor tegas standar `.tab-content { display: none !important; }` dan `.tab-content.active { display: block !important; }` guna mencegah tab kosong (blank) akibat benturan spesifisitas engine CSS browser.
- **Container Registry CI/CD & Versioned Distribution**:
  - Untuk distribusi gateway siap pakai tanpa clone repo, sediakan workflow GitHub Actions yang mengompilasi multi-platform (`linux/amd64`, `linux/arm64`) dan menerbitkan image ke GHCR (`ghcr.io/<owner>/<repo>:<tag>`).
  - Dukung penomoran tag kustom (misal `agyrouter0.1`) di metadata action agar pengguna dapat langsung menjalankan kontainer via `docker run -d -p 3456:3456 -v $(pwd)/data:/app/data ghcr.io/<owner>/<repo>:<tag>`.
- **Google OAuth Popup Flow & Multi-Channel Cross-Window Messaging**:
  - Saat login Google OAuth di dashboard SPA, buka alur otentikasi di jendela popup (`window.open('/api/auth/google', 'googleLoginPopup', 'width=550,height=680...')`).
  - Halaman landing callback OAuth di backend wajib merespons dengan HTML self-closing yang memancarkan event sukses ke parent window melalui tiga lapis komunikasi konkuren (`BroadcastChannel('ag_auth_channel')`, `window.opener.postMessage(...)`, dan `localStorage` event fallback), lalu auto-close via `window.close()`.
  - Pada listener SPA (`app.js`), dengarkan `BroadcastChannel` dan `window.addEventListener('message', ...)` untuk memicu re-fetch dataset akun dan re-render kartu kuota seketika tanpa me-refresh tab utama.
  - **Presisi Persentase Kuota Non-Integer**: Hindari pembulatan integer kasar (`Math.round`) yang menghilangkan desimal mikro; tampilkan nilai bulat jika integer (`100%`) dan pertahankan hingga 2 digit desimal jika pecahan (`99.85%`) untuk merefleksikan akurasi upstream Antigravity.
  - **Auto-Polling Pending Quota**: Pasang timer polling otomatis 2 detik jika terdeteksi akun aktif baru yang snapshot kuotanya masih kosong/pending dari Google API.
- **Skills Lifecycle Management, Built-in Soft-Deletion & Playground Sync**:
  - **Built-in Skill Soft-Deletion & Toggle**: Built-in skill tidak boleh dihapus secara permanen dari basis data; gunakan penanda `Deleted bool` (soft-delete) dan `Enabled = false` saat dihapus agar dapat dipulihkan (*restored*) ke preset bawaan kapan saja via aksi `reset_builtins`. Hanya skill custom yang dihapus permanen.
  - **Preservasi State Across Reboots**: Saat inisialisasi penyimpanan (boot), jangan menimpa status `Enabled` dan `Deleted` built-in skill yang sudah dimodifikasi pengguna dengan nilai default bawaan kode.
  - **Two-Step Destructive Confirmation Guard**: Setiap aksi penghapusan skill (baik built-in maupun custom) wajib melalui dialog konfirmasi 2 langkah (`showDestructiveConfirm`) dengan penjelasan konsekuensi yang gamblang.
  - **Playground Auto-Pruning & Reactive Badges**: Sinkronkan daftar combo skills di Playground secara reakt- **Manajemen Siklus Hidup Skill Prompt & Modal Guard**:
  - Hindari penggunaan prompt dialog bawaan browser (`window.prompt()`) untuk pembuatan entitas sistem/skill; sediakan form modal terstruktur (`#createSkillModal`) dengan validasi field wajib (nama, deskripsi, textarea monospace untuk directive prompt) agar tidak menghasilkan entitas terpotong atau rusak.
  - Untuk aksi destruktif penghapusan permanen skill, terapkan *strict entity-name verification guard*: pengguna wajib mengetikkan nama entitas secara persis (`verifyText: name`, bukan sekadar teks statis `'HAPUS'`) pada dialog konfirmasi 2 langkah (`#confirmModal`) sebelum tombol hapus diaktifkan.
  - Sinkronisasi state seleksi Playground reaktif: skill yang dinonaktifkan atau dihapus wajib otomatis tereliminasi dari state seleksi aktif (`selectedComboSkills`) dan tidak diinjeksi ke prompt inferensi. Tampilkan badge status visual (`Aktif` vs `Nonaktif`), tombol toggle instan, dan aksi *Pulihkan Builtin* untuk mereset seluruh preset default bila sewaktu-waktu dibutuhkan.
  - Preservasi persistensi konfigurasi: status `enabled` dan penghapusan skill pada boot storage backend wajib menghormati konfigurasi pengguna di disk (`skills.json`) agar server restart tidak menimpa status skill nonaktif kembali ke kondisi aktif secara sepihak.
- **Strict Name-Match Deletion Guard & Dedicated Creation Modal**:
  - Hindari penggunaan prompt dialog bawaan browser (`window.prompt()`) untuk form konfigurasi atau penambahan data (seperti pembuatan skill atau preset); gunakan modal kustom terstruktur Tailwind v4 dengan styling monospace pada textarea directive agar input nyaman dan terformat.
  - Untuk aksi penghapusan permanen entitas konfigurasi (seperti skill), tegakkan two-step modal guard yang mewajibkan input teks nama entitas secara persis (`verifyText: entity.name`) sebelum tombol submit berbahaya diaktifkan guna mencegah bencana salah klik.
  - Batasi daftar default fallback model chain pada endpoint discovery (`GET /api/fallbacks`, `GET /v1/models`) hanya pada model resmi yang benar-benar aktif di gateway (misal: `gemini-3.8-flash-tiered`, `claude-opus-4-6-thinking`, `claude-sonnet-4-6`, `gpt-oss-120b-medium`), hindari mencemari pilihan autocomplete pengguna dengan varian sub-tier internal atau model yang belum tersedia di upstream.
- **Streaming Auto-Scroll Locking Pitfall pada Chat & Playground**:
  - Jangan memanggil `container.scrollTop = container.scrollHeight` tanpa syarat pada setiap chunk kata yang di-stream (`reader.read()`). Hal ini mengunci scrollbar ke bawah dan secara agresif menarik layar pengguna saat mencoba membaca pesan sebelumnya ke atas.
  - Pasang passive scroll listener pada container pesan untuk mendeteksi intensi scroll pengguna (`isNearBottom = (scrollHeight - scrollTop - clientHeight) <= threshold`). Hanya jalankan auto-scroll ke dasar jika pengguna memang berada di area bawah (`isNearBottom && !userScrolledUp`), dan paksa scroll ke dasar hanya saat pengguna mengirim pesan baru.


