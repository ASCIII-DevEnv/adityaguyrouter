# Integrasi Native Provider Antigravity di LiteLLM

Spesifikasi, pola arsitektur, dan mitigasi pitfall saat mengintegrasikan Google Antigravity (Google Cloud Code PA) secara native ke dalam LiteLLM Proxy dan SDK.

## 1. Arsitektur Komponen

- **Pendaftaran Provider (`litellm/types/utils.py`)**:
  Tambahkan `ANTIGRAVITY = "antigravity"` ke enum `LlmProviders`. Ini otomatis mendaftarkan prefix model `antigravity/<model>` ke `provider_list`.
- **Routing Eksekusi (`litellm/main.py`)**:
  Tambahkan fungsi dispatcher `_complete_antigravity(ctx: CompletionDispatchContext)` yang menangani branching `acompletion` (sync vs async), lalu hubungkan ke switch `custom_llm_provider == "antigravity"`.
- **Modul Handler (`litellm/llms/antigravity/`)**:
  - `authenticator.py`: Google OAuth 2.0 token manager, fallback reader, auto-refresh token via `https://oauth2.googleapis.com/token`.
  - `common_utils.py`: Endpoints, header fingerprint (`antigravity/ide/2.11.0 linux/x64`, `X-Client-Name: antigravity`), discovery `loadCodeAssist`.
  - `chat/transformation.py`: `AntigravityConfig(BaseConfig)` mengonversi format OpenAI (`messages`) ke format envelope Google Cloud Code PA (`contents`, `generationConfig`, `systemInstruction`).
  - `chat/streaming_iterator.py`: `AntigravityModelResponseIterator(BaseModelResponseIterator)` untuk parsing SSE chunk dan ekstraksi teks delta serta token usage.
  - `chat/handler.py`: Entry point pemanggilan `BaseLLMHTTPHandler.completion`.
  - `oauth_routes.py`: Endpoint OAuth browser (`/oauth/antigravity/login` dan `/oauth/antigravity/callback`).

## 2. Format Request & Endpoints

- **Endpoint URL**:
  - Non-streaming: `https://daily-cloudcode-pa.googleapis.com/v1internal:generateContent`
  - Streaming: `https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse`
- **Envelope Body JSON**:
  ```json
  {
    "project": "aicode-consumers",
    "model": "gemini-2.5-flash",
    "request": {
      "contents": [
        {
          "role": "user",
          "parts": [{"text": "Prompt text"}]
        }
      ],
      "systemInstruction": {
        "parts": [{"text": "System prompt"}]
      },
      "generationConfig": {
        "temperature": 0.2,
        "maxOutputTokens": 4096
      }
    }
  }
  ```

## 3. Web UI OAuth & Dashboard Integration

- **Browser Login Flow**:
  1. `GET /oauth/antigravity/login`: Menghasilkan URL Google OAuth dan mengarahkan browser (`307 Redirect`).
  2. `GET /oauth/antigravity/callback`: Menerima authorization code, menukar ke `access_token` dan `refresh_token`, mengambil email profil, dan menyimpannya ke `~/.config/litellm/antigravity/credentials.json`.
  3. Mengembalikan kartu HTML konfirmasi sukses dengan tombol kembali ke `/ui`.
- **Integrasi Banner Dashboard**:
  LiteLLM Dashboard menampilkan banner dari endpoint `/get/user_banner`. Pasang banner tautan OAuth via `PATCH /update/user_banner` dengan payload Markdown:
  `{"enabled": true, "message": "Google Antigravity: [Login Akun Google OAuth](/oauth/antigravity/login)", "severity": "info"}`.

## 4. Mitigasi Pitfall Kritis

1. **Pitfall `stream` di Request Body**:
   - *Masalah*: Google Cloud Code menolak request jika body berisi `"stream": true` (`Invalid JSON payload received. Unknown name "stream": Cannot find field`).
   - *Mitigasi*: Override `@property def supports_stream_param_in_request_body(self) -> bool: return False` pada `AntigravityConfig`. Streaming Google hanya menggunakan query parameter `?alt=sse` di URL, bukan atribut di body JSON.

2. **Pitfall Tipe Data Usage pada Streaming Chunk**:
   - *Masalah*: `streaming_handler.py` melakukan unpacking `**chunk["usage"]`. Jika `usage` dikembalikan sebagai objek Pydantic `Usage`, Python melempar `TypeError: litellm.types.utils.Usage() argument after ** must be a mapping, not Usage`.
   - *Mitigasi*: Di `AntigravityModelResponseIterator`, kembalikan `usage` sebagai `dict` (`{"prompt_tokens": int, "completion_tokens": int, "total_tokens": int}`).

3. **Pitfall Database Dependency pada LiteLLM Web UI (`/ui`)**:
   - *Masalah*: Saat login ke Dashboard Web UI LiteLLM, server memanggil `/key/info`. Tanpa koneksi database PostgreSQL, server melempar error `Authentication Error, Not connected to DB!`.
   - *Mitigasi*: Dashboard Web UI LiteLLM wajib didukung PostgreSQL (Prisma). Konfigurasikan `database_url: "postgresql://..."` di config YAML atau environment variable `DATABASE_URL`.

4. **Pitfall FastAPI Lifespan Recursion Limit**:
   - *Masalah*: LiteLLM memiliki lebih dari 50 subrouter bersarang. Jika server dijalankan tanpa `LITELLM_MASTER_KEY`, pengecekan keamanan melempar `UnsafeMasterKeyError` yang saat stack unwinding menembus batas rekursi Python default (`RecursionError: maximum recursion depth exceeded`).
   - *Mitigasi*: Selalu setel `general_settings.master_key` di file konfigurasi YAML atau `export LITELLM_MASTER_KEY=...`.

5. **Pitfall Model Prefix Strip**:
   - *Masalah*: LiteLLM merutekan berdasarkan nama model `antigravity/gemini-2.5-flash`. Jika string `antigravity/` diteruskan ke upstream Google, API mengembalikan 404/400 model not found.
   - *Mitigasi*: Bersihkan prefix di `transform_request` menggunakan `clean_model = model.replace("antigravity/", "")`.
