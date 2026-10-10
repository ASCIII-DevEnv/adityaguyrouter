# Antigravity Gateway — Liquid Glass Design System Specification

## 1. Design Philosophy

Antigravity AI Gateway is an ultra-low memory AI gateway and multi-account failover manager. The visual interface is constructed around **Liquid Glass as its exclusive material system**:
- **Tactile Depth & Translucency:** Interfaces utilize multi-layered materials that allow the underlying environment to refract softly without obscuring critical observability metrics.
- **Precision & Clarity:** High-density data, logs, and token analytics remain readable through solid inner materials when necessary, while structural elements project an airy, luminous quality.
- **Human-Crafted Artistry:** Controlled light diffusion, hairline glass boundaries, and purposeful geometry eliminate generic AI slop.

---

## 2. Liquid Glass Material System

### A. Primary Liquid Glass (Floating Navigation & Header)
- **Background:** `rgba(255, 255, 255, 0.72)`
- **Blur:** `20px` (`backdrop-filter: blur(20px)`)
- **Edge Definition:** `1px solid rgba(15, 23, 42, 0.06)`
- **Inner Refraction Highlight:** `inset 0 1px 1px 0 rgba(255, 255, 255, 0.95), inset 0 -1px 1px 0 rgba(15, 23, 42, 0.03)`
- **Elevation Shadow:** `0 12px 32px -4px rgba(15, 23, 42, 0.06), 0 4px 12px -2px rgba(15, 23, 42, 0.03)`

### B. Secondary Liquid Glass (Section Panels, Stat Cards)
- **Background:** `rgba(255, 255, 255, 0.82)`
- **Blur:** `16px` (`backdrop-filter: blur(16px)`)
- **Edge Definition:** `1px solid rgba(15, 23, 42, 0.07)`
- **Inner Refraction Highlight:** `inset 0 1px 1px 0 rgba(255, 255, 255, 0.9)`
- **Elevation Shadow:** `0 8px 24px -3px rgba(15, 23, 42, 0.05), 0 2px 6px -1px rgba(15, 23, 42, 0.02)`

### C. Solid Functional Material (Tables, Dense Logs, Code Snippets)
- **Background:** `#ffffff` and `#0f172a` (code blocks)
- **Borders:** `rgba(226, 232, 240, 0.9)`
- Provides high contrast and 100% legibility for streaming inspection, cURL reproduction, and model quota tables.

---

## 3. Avatar & Media Discipline

- **Profile Pictures (`.item-avatar`):**
  - Rigid bounding box: `width: 44px; height: 44px; min-width: 44px; max-width: 44px;`
  - Border radius: `12px` squircle with hairline inset border (`border: 1.5px solid #bfdbfe;`)
  - Image behavior: `object-fit: cover; width: 100%; height: 100%; border-radius: inherit; display: block;`
  - Fallback mechanism: In the event of broken image links or missing pictures, gracefully falls back to the account's capitalized initial letter inside an indigo-sky badge without breaking layout geometry.

---

## 4. Lazy-Loaded Modular Architecture

- **Sub-100ms Initial Load:** The heavy Token Analytics module (`/public/partials/analytics.html`) containing Chart.js instances and historical logs is decoupled from the main DOM and loaded asynchronously only when requested.
- **Asynchronous Mount:** When switched to, `requestAnimationFrame` ensures all DOM elements and canvas contexts are verified before triggering Chart.js compilation and data binding.

---

## 5. Anti-Patterns (Strictly Prohibited)

1. Unconstrained image tags (`img` without aspect-ratio or rigid container boundaries).
2. Opaque flat gray boxes without refractive glass highlights.
3. Random floating blurred blobs without spatial purpose.
4. Heavy saturation gradients behind dense tabular data.
