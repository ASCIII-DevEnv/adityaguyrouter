# Panduan Redesign UI Dashboard Gateway (Anti-AI Slop & Clean Glassmorphism)

Panduan teknis dan standar desain untuk antarmuka web gateway monolitik (`public/index.html` yang disajikan Go/Node.js).

## 1. Menghilangkan Klise AI-Slop (Anti-Slop Principles)
- **Eliminasi Ornamen Berlebihan**:
  - Hapus ambient floating orbs melayang dengan animasi `pulse`/`translate` yang tidak membawa informasi fungsional.
  - Hapus teks gradien warna-warni yang mencolok pada heading utama dan angka statistik.
  - Hapus drop shadow ungu/pink berlebihan dan border neon yang melelahkan mata.
- **Tipografi Tajam & Hirarki Jelas**:
  - Gunakan font sans-serif modern yang terbaca bersih (Geist, Inter, atau Plus Jakarta Sans) dipadukan dengan JetBrains Mono untuk metrik angka, token, dan latency.
  - Hindari font ukuran raksasa tanpa proporsi fungsional.

## 2. Glassmorphism Presisi (Light Mode Architecture)
- **Fondasi Warna Light Theme**:
  - Background kanvas: Gradien ultra-subtil dari Slate sangat terang ke Sky halus (`#f8fafc` ke `#f0f7ff`).
  - Panel & Card Kaca: `rgba(255, 255, 255, 0.75)` dengan filter `backdrop-blur-md` dan border tipis `rgba(226, 232, 240, 0.8)`.
  - Shadow Bertingkat Lembut: Gunakan natural layered shadows (`0 1px 3px rgba(0,0,0,0.03), 0 4px 12px rgba(0,0,0,0.02)`), bukan fuzzy heavy colored shadows.

## 3. Komponen Inti & Navigasi
- **Header & Ringkasan Metrik**:
  - Tampilkan logo bersih dan status koneksi live.
  - Letakkan metrik ringkas: Total Request, Token Terhemat (Token Saver), Latency Rata-rata, Akun Aktif.
- **Tab Navigasi Fungsional**:
  - Overview / Usage Pool (Kapasitas per akun Google & kuota model).
  - Model Fallback Matrix (Chain urutan fallback per model).
  - API Keys & Token Management.
  - Skills Injection Manager (Prompt persona).
  - Live Request Inspector (Streaming SSE & replay cURL).
  - Analytics & Cost Efficiency.
  - Interactive Playground (Testbed prompt & persona).

## 4. Pelajaran Penting & Pitfalls Teruji
- **Pewarnaan Metrik Token Terhemat**:
  - Gunakan palet warna yang konsisten dan kontras:
    - Tanpa Saver: `text-slate-400`
    - Dihemat: `text-emerald-600`
- **Pembersihan Konflik Selektor Tabel**:
  - Hindari penulisan aturan CSS global `td { color: var(--text-main); }` pada stylesheet utama karena secara otomatis menimpa styling kelas utilitas Tailwind pada teks sel tabel. Biarkan warna mewarisi dari kelas container atau selnya sendiri.
- **Tampilan Waktu Reset Kuota**:
  - Sertakan waktu reset jam lokal dan durasi relatif (misal: `Reset: 16:09 (2j 45m lagi)`) di bawah setiap progress bar kuota 5-jam dan mingguan.
- **Playground Streaming Final Flush & Auto-Scroll Safety**:
  - Wajib memanggil render final (`renderPlaygroundMessages()`) tepat setelah stream pembacaan SSE selesai (`[DONE]` / `reader.read() done`). Mengabaikan render akhir dapat menyebabkan teks respons tersimpan di memori tetapi gelembung chat tampak kosong/tidak membalas.
  - Saat streaming aktif (`isStreaming = true`), tampilkan preview teks bersih secara real-time dan tunda kompilasi form kartu interaktif kompleks (`grill-me`) hingga respons lengkap selesai diterima untuk mencegah parser merender form kosong prematur.
- **Grill-Me Form Parsing & Markdown Separation Pitfalls**:
  - Abaikan garis pemisah horizontal Markdown (`---`, `***`, `___`) di parser interogasi agar garis tersebut tidak memutus parser opsi atau mengabaikan nomor pertanyaan berikutnya.
  - Jangan mengasumsikan nomor pertanyaan selalu di awal baris dengan awalan emoji tertentu; gunakan pengujian fleksibel regex `/(?:❓|\?|\bQ\d+[:\s\-]|\*\*Q\d+)/i` dan pastikan tidak mencocokkan opsi pilihan `!/^[-*•\d.]*\s*\[[A-Z]\]/i`.
  - Bersihkan simbol asterisks ganda/tunggal Markdown (`**`, `*`) dari judul dan label opsi agar UI kartu bersih tanpa karakter format mentah.
  - Sediakan fallback minimal 1 rekomendasi badge default (misal opsi `[A]`) jika model lupa menandai rekomendasi secara eksplisit.
- **Cache-Busting pada Assets & Partials AJAX**:
  - Sertakan parameter versi (`?v=...`) tidak hanya pada tag `<script>` dan `<link>` di `index.html`, tetapi juga pada pemanggilan AJAX partial (`fetch('/partials/' + tabId + '.html?v=...')`) agar browser tidak menyajikan partial basi dari disk cache setelah perbaikan kode.
- **Proteksi Hotlinking Gambar Pihak Ketiga (Avatar Google / lh3.googleusercontent.com)**:
  - Google usercontent memblokir request gambar jika membawa header `Referer` lokal (HTTP 403 Forbidden).
  - Tambahkan `<meta name="referrer" content="no-referrer">` di `<head>` dokumen HTML atau pasang atribut `referrerpolicy="no-referrer"` langsung pada tag `<img src="...">` avatar agar gambar profil Google selalu tampil tanpa terpicu fallback `onerror`.
