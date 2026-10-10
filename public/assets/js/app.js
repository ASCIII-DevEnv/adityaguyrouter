let currentTab = 'dashboard';
    let accountsData = [];
    let keysData = [];
    let skillsData = [];
    let playgroundHistory = [];
    let isProviderCollapsed = false;
    let currentInspectorRecords = [];
    let isAnalyticsLoaded = false;

    function svgIcon(name, extraClass = '') {
      return `<svg class="icon ${extraClass}"><use href="#icon-${name}"></use></svg>`;
    }

    async function loadAnalyticsPartial() {
      const mount = document.getElementById('analyticsMount');
      if (isAnalyticsLoaded) {
        await fetchTokenAnalytics();
        return;
      }
      try {
        const res = await fetch('/partials/analytics.html');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const html = await res.text();
        if (mount) {
          mount.innerHTML = html;
          isAnalyticsLoaded = true;
          // Short frame delay so DOM elements and canvas elements exist in layout
          requestAnimationFrame(async () => {
            await fetchTokenAnalytics();
          });
        }
      } catch (err) {
        if (mount) {
          mount.innerHTML = `
            <div class="glass-panel p-10 text-center text-rose-600">
              <svg class="w-8 h-8 mx-auto mb-2"><use href="#icon-alert-triangle"></use></svg>
              <div class="font-extrabold text-base">Gagal Memuat Token Analytics</div>
              <p class="text-xs text-slate-500 mt-1">${escapeHtml(err.message)}</p>
              <button class="btn btn-sm btn-primary mt-4" onclick="loadAnalyticsPartial()">Coba Lagi</button>
            </div>
          `;
        }
      }
    }

    function toggleMobileNav() {
      const nav = document.getElementById('mainNavTabs');
      const label = document.getElementById('mobileNavLabel');
      if (nav) {
        nav.classList.toggle('open');
        if (label) {
          label.textContent = nav.classList.contains('open') ? 'Tutup' : 'Menu';
        }
      }
    }

    // Close mobile nav when clicking outside header
    document.addEventListener('click', function(e) {
      const header = document.querySelector('.header');
      const nav = document.getElementById('mainNavTabs');
      const label = document.getElementById('mobileNavLabel');
      if (header && nav && nav.classList.contains('open') && !header.contains(e.target)) {
        nav.classList.remove('open');
        if (label) label.textContent = 'Menu';
      }
    });

    // Dynamic Partial Tab Loader to keep index.html ultra-lean
    const loadedTabs = new Set(['dashboard']);

    async function ensureTabLoaded(tabId) {
      if (tabId === 'dashboard') return true;
      const target = document.getElementById('tab-' + tabId);
      if (!target) return false;

      // Special mount container for analytics
      if (tabId === 'analytics') {
        return await loadAnalyticsPartial();
      }

      if (loadedTabs.has(tabId)) return true;

      try {
        const res = await fetch(`/partials/${tabId}.html?v=0.1.1`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const html = await res.text();
        target.innerHTML = html;
        loadedTabs.add(tabId);
        return true;
      } catch (err) {
        target.innerHTML = `
          <div class="glass-panel p-8 text-center text-rose-600">
            <svg class="w-8 h-8 mx-auto mb-2"><use href="#icon-alert-triangle"></use></svg>
            <div class="font-extrabold text-base">Gagal Memuat Modul ${tabId}</div>
            <p class="text-xs text-slate-500 mt-1">${err.message}</p>
            <button class="btn btn-sm btn-primary mt-4" onclick="ensureTabLoaded('${tabId}').then(() => switchTab('${tabId}'))">Coba Lagi</button>
          </div>
        `;
        return false;
      }
    }

    async function switchTab(tabId, updateUrl = true) {
      // Auto close mobile collapsible nav on item click
      const mobileNav = document.getElementById('mainNavTabs');
      const mobileLabel = document.getElementById('mobileNavLabel');
      if (mobileNav && mobileNav.classList.contains('open')) {
        mobileNav.classList.remove('open');
        if (mobileLabel) mobileLabel.textContent = 'Menu';
      }
      // Compatibility redirection: jika tab apikeys, skills, atau integrations dipanggil, redirect ke seksi Home
      if (tabId === 'apikeys' || tabId === 'skills' || tabId === 'integrations') {
        currentTab = 'dashboard';
        if (updateUrl && history.replaceState) {
          history.replaceState(null, '', '#' + tabId);
        }
        document.querySelectorAll('.nav-tab').forEach(b => b.classList.remove('active'));
        const homeTabBtn = document.querySelector('.nav-tab[onclick*="dashboard"]');
        if (homeTabBtn) homeTabBtn.classList.add('active');
        document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
        const homeTab = document.getElementById('tab-dashboard');
        if (homeTab) homeTab.classList.add('active');

        fetchAccounts();
        fetchApiKeys();
        fetchSkills();

        setTimeout(() => {
          const targetSection = document.getElementById('section-' + tabId);
          if (targetSection) targetSection.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }, 80);
        return;
      }

      currentTab = tabId;

      // Update URL Hash hash-routing
      if (updateUrl) {
        if (tabId === 'dashboard') {
          if (window.location.hash) {
            history.pushState ? history.pushState(null, '', window.location.pathname) : (window.location.hash = '');
          }
        } else {
          if (window.location.hash !== '#' + tabId) {
            history.pushState ? history.pushState(null, '', '#' + tabId) : (window.location.hash = '#' + tabId);
          }
        }
      }

      document.querySelectorAll('.nav-tab').forEach(b => b.classList.remove('active'));
      const activeBtn = document.querySelector(`.nav-tab[onclick*="'${tabId}'"]`);
      if (activeBtn) activeBtn.classList.add('active');

      document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
      const target = document.getElementById('tab-' + tabId);
      if (target) target.classList.add('active');

      // Ensure partial component is fetched and injected FIRST
      if (tabId !== 'dashboard') {
        await ensureTabLoaded(tabId);
      }

      if (tabId === 'dashboard') {
        fetchAccounts();
        fetchApiKeys();
        fetchSkills();
      }
      if (tabId === 'usage') {
        if (accountsData.length === 0) {
          await fetchAccounts();
        } else {
          renderUsage();
          fetchAccounts();
        }
      }
      if (tabId === 'provider') {
        if (accountsData.length === 0) {
          await fetchAccounts();
        } else {
          renderProvider();
          fetchAccounts();
        }
        fetchProviderStrategy();
      }
      if (tabId === 'fallbacks') fetchFallbacks();
      if (tabId === 'inspector') fetchInspectorLogs();
      if (tabId === 'playground') {
        updatePgAccountsDropdown();
        renderComboSkillsPills();
        fetchAccounts();
        fetchSkills();
      }
      if (tabId === 'analytics') {
        loadAnalyticsPartial();
      }
    }

    async function fetchAccounts() {
      try {
        const res = await fetch('/api/accounts');
        const data = await res.json();
        accountsData = data.accounts || [];
        // Respect backend order (which respects user-customized order), fallback to email
        accountsData.sort((a, b) => {
          if ((a.order ?? 0) !== (b.order ?? 0)) {
            return (a.order ?? 0) - (b.order ?? 0);
          }
          return (a.email || '').localeCompare(b.email || '');
        });

        // Update stats
        const statTotalEl = document.getElementById('statTotalAccounts');
        if (statTotalEl) statTotalEl.textContent = accountsData.length;
        const activeCount = accountsData.filter(a => a.isActive && (!a.cooldownUntil || new Date(a.cooldownUntil) < new Date())).length;
        const statActiveEl = document.getElementById('statActiveAccounts');
        if (statActiveEl) statActiveEl.textContent = activeCount;

        renderUsage();
        renderProvider();
        updatePgAccountsDropdown();
        renderComboSkillsPills();

        // Jika ada akun baru yang kuota/reset time-nya masih dimuat dari Google, auto-refresh setelah 2 detik
        const hasPendingQuota = accountsData.some(a => a.isActive && (!a.families || !a.quotaWeeklyResetAt));
        if (hasPendingQuota && !window._quotaPollingTimer) {
          window._quotaPollingTimer = setTimeout(() => {
            window._quotaPollingTimer = null;
            fetchAccounts();
          }, 2000);
        }
      } catch (err) {
        console.error('fetchAccounts error:', err);
      }
    }

    function formatResetTime(isoStr) {
      if (!isoStr) return '';
      try {
        const d = new Date(isoStr);
        if (isNaN(d.getTime())) return '';
        const now = new Date();
        const diffMs = d.getTime() - now.getTime();
        const timeStr = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        if (diffMs <= 0) return `Reset: ${timeStr} (segera)`;
        const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
        const diffMins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60));
        let relStr = '';
        if (diffHours > 24) {
          const days = Math.floor(diffHours / 24);
          relStr = `${days}h ${diffHours % 24}j lagi`;
        } else if (diffHours > 0) {
          relStr = `${diffHours}j ${diffMins}m lagi`;
        } else {
          relStr = `${diffMins}m lagi`;
        }
        return `Reset: ${timeStr} (${relStr})`;
      } catch (e) {
        return '';
      }
    }

    function formatQuotaPercent(val) {
      if (val === undefined || val === null || isNaN(val)) return '0%';
      const n = Number(val);
      if (Number.isInteger(n)) return `${n}%`;
      // Pertahankan desimal presisi seperti pada Antigravity (hingga 2 digit di belakang desimal tanpa pembulatan integer)
      const formatted = parseFloat(n.toFixed(2));
      return `${formatted}%`;
    }

    function renderUsage() {
      const grid = document.getElementById('usageAccountsGrid');
      if (!grid) return;
      if (accountsData.length === 0) {
        grid.innerHTML = `
          <div class="col-span-full text-center py-10 text-slate-500 bg-white/60 rounded-2xl border border-dashed border-slate-300 space-y-3">
            <p class="text-sm font-semibold text-slate-600">Belum ada akun Google terdaftar.</p>
            <div>
              <button class="btn btn-sm btn-primary inline-flex items-center gap-1.5 shadow-sm" onclick="loginGoogle()">
                ${svgIcon('plus', 'icon-sm')} Login Akun Google Sekarang
              </button>
            </div>
          </div>
        `;
        return;
      }

      grid.innerHTML = accountsData.map(acc => {
        const families = acc.families || {};
        const gemini = families.gemini || null;
        const claude = families.claude || null;

        const gemini5h = gemini?.limit5h?.remainingPercent ?? (acc.quota5hPercent ?? 0);
        const geminiWk = gemini?.limitWeekly?.remainingPercent ?? (acc.quotaWeeklyPercent ?? 0);
        const claude5h = claude?.limit5h?.remainingPercent ?? (acc.quota5hPercent ?? 0);
        const claudeWk = claude?.limitWeekly?.remainingPercent ?? (acc.quotaWeeklyPercent ?? 0);

        const gemini5hReset = gemini?.limit5h?.resetAt || acc.quota5hResetAt || '';
        const geminiWkReset = gemini?.limitWeekly?.resetAt || acc.quotaWeeklyResetAt || '';
        const claude5hReset = claude?.limit5h?.resetAt || '';
        const claudeWkReset = claude?.limitWeekly?.resetAt || '';

        const gemini5hResetText = formatResetTime(gemini5hReset);
        const geminiWkResetText = formatResetTime(geminiWkReset);
        const claude5hResetText = formatResetTime(claude5hReset);
        const claudeWkResetText = formatResetTime(claudeWkReset);

        const geminiModels = gemini?.models || [];
        const claudeModels = claude?.models || [];

        return `
          <div class="item-card col-span-full">
            <div class="item-card-header">
              <div class="flex items-center gap-2.5">
                <div class="relative shrink-0">
                  <div class="item-avatar bg-slate-900 border border-white/10 flex items-center justify-center ${!acc.isActive ? 'opacity-60 grayscale' : ''}">${acc.picture ? `<img src="${acc.picture}" alt="" onerror="this.classList.add('hidden'); this.parentElement.innerHTML='<svg class=\'w-5 h-5\'><use href=\'#icon-antigravity\'></use></svg>';" />` : `<svg class="w-5 h-5"><use href="#icon-antigravity"></use></svg>`}</div>
                  ${!acc.isActive ? `
                    <span class="absolute -bottom-1 -right-1 w-5 h-5 rounded-full bg-red-600 text-white flex items-center justify-center shadow-sm ring-2 ring-white" title="Akun Dinonaktifkan">
                      <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round">
                        <line x1="18" y1="6" x2="6" y2="18"></line>
                        <line x1="6" y1="6" x2="18" y2="18"></line>
                      </svg>
                    </span>
                  ` : ''}
                </div>
                <div>
                  <div class="font-extrabold text-base text-slate-900">${escapeHtml(acc.name || acc.email)}</div>
                  <div class="text-xs text-slate-500">${escapeHtml(acc.email)}</div>
                </div>
              </div>
              <div class="flex gap-2 items-center">
                <span class="badge ${acc.isActive ? 'badge-success' : 'badge-danger'}">${acc.isActive ? 'Active' : 'Disabled'}</span>
              </div>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <!-- Gemini Family -->
              <div class="bg-sky-50/70 border border-sky-200/80 rounded-2xl p-4">
                <div class="flex justify-between items-center mb-2.5">
                  <strong class="text-sky-700 text-sm inline-flex items-center gap-1.5">${svgIcon('cpu')} Gemini Models (${geminiModels.length} models)</strong>
                  <span class="text-[11px] text-slate-500 font-semibold">Rolling &amp; Weekly</span>
                </div>
                <div class="mb-3">
                  <div class="flex justify-between text-xs mb-1">
                    <span class="text-slate-600 font-medium">Gemini 5 Jam (Session)</span>
                    <strong class="text-sky-800">${formatQuotaPercent(gemini5h)}</strong>
                  </div>
                  <div class="progress-bar"><div class="progress-fill bg-sky-600" style="width: ${gemini5h}%;"></div></div>
                  ${gemini5hResetText ? `<div class="text-[10px] text-sky-700/80 font-medium mt-1 flex items-center gap-1">${svgIcon('clock', 'w-3 h-3')} ${gemini5hResetText}</div>` : ''}
                </div>
                <div>
                  <div class="flex justify-between text-xs mb-1">
                    <span class="text-slate-600 font-medium">Gemini Mingguan</span>
                    <strong class="text-sky-800">${formatQuotaPercent(geminiWk)}</strong>
                  </div>
                  <div class="progress-bar"><div class="progress-fill bg-sky-600" style="width: ${geminiWk}%;"></div></div>
                  ${geminiWkResetText ? `<div class="text-[10px] text-sky-700/80 font-medium mt-1 flex items-center gap-1">${svgIcon('clock', 'w-3 h-3')} ${geminiWkResetText}</div>` : ''}
                </div>

                <!-- Individual Model Limits -->
                ${geminiModels.length > 0 ? `
                  <details class="mt-3 pt-2.5 border-t border-sky-200/50">
                    <summary class="text-[11px] font-bold text-sky-700 cursor-pointer select-none">Lihat Sisa Kuota ${geminiModels.length} Model Gemini</summary>
                    <div class="grid grid-cols-2 gap-1.5 mt-2 max-h-44 overflow-y-auto">
                      ${geminiModels.map(m => {
                        const name = (m.displayName || m.id || m.name || '').replace('models/', '');
                        return `
                        <div class="bg-white p-1.5 rounded-lg border border-slate-200/60 shadow-2xs">
                          <div class="font-semibold text-[11px] text-slate-800 truncate" title="${name}">${name}</div>
                          <div class="text-sky-600 font-extrabold text-xs">${formatQuotaPercent(m.remainingPercent ?? 0)}</div>
                        </div>
                      `;}).join('')}
                    </div>
                  </details>
                ` : ''}
              </div>

              <!-- Claude / Anthropic Family -->
              <div class="bg-purple-50/70 border border-purple-200/80 rounded-2xl p-4">
                <div class="flex justify-between items-center mb-2.5">
                  <strong class="text-purple-700 text-sm inline-flex items-center gap-1.5">${svgIcon('layers')} Claude &amp; Other (${claudeModels.length} models)</strong>
                  <span class="text-[11px] text-slate-500 font-semibold">Sonnet &amp; Opus</span>
                </div>
                <div class="mb-3">
                  <div class="flex justify-between text-xs mb-1">
                    <span class="text-slate-600 font-medium">Claude 5 Jam (Session)</span>
                    <strong class="text-purple-800">${formatQuotaPercent(claude5h)}</strong>
                  </div>
                  <div class="progress-bar"><div class="progress-fill bg-purple-600" style="width: ${claude5h}%;"></div></div>
                  ${claude5hResetText ? `<div class="text-[10px] text-purple-700/80 font-medium mt-1 flex items-center gap-1">${svgIcon('clock', 'w-3 h-3')} ${claude5hResetText}</div>` : ''}
                </div>
                <div>
                  <div class="flex justify-between text-xs mb-1">
                    <span class="text-slate-600 font-medium">Claude Mingguan</span>
                    <strong class="text-purple-800">${formatQuotaPercent(claudeWk)}</strong>
                  </div>
                  <div class="progress-bar"><div class="progress-fill bg-purple-600" style="width: ${claudeWk}%;"></div></div>
                  ${claudeWkResetText ? `<div class="text-[10px] text-purple-700/80 font-medium mt-1 flex items-center gap-1">${svgIcon('clock', 'w-3 h-3')} ${claudeWkResetText}</div>` : ''}
                </div>

                <!-- Individual Model Limits -->
                ${claudeModels.length > 0 ? `
                  <details class="mt-3 pt-2.5 border-t border-purple-200/50">
                    <summary class="text-[11px] font-bold text-purple-700 cursor-pointer select-none">Lihat Sisa Kuota ${claudeModels.length} Model Claude</summary>
                    <div class="grid grid-cols-2 gap-1.5 mt-2 max-h-44 overflow-y-auto">
                      ${claudeModels.map(m => {
                        const name = (m.displayName || m.id || m.name || '').replace('models/', '');
                        return `
                        <div class="bg-white p-1.5 rounded-lg border border-slate-200/60 shadow-2xs">
                          <div class="font-semibold text-[11px] text-slate-800 truncate" title="${name}">${name}</div>
                          <div class="text-purple-600 font-extrabold text-xs">${formatQuotaPercent(m.remainingPercent ?? 0)}</div>
                        </div>
                      `;}).join('')}
                    </div>
                  </details>
                ` : ''}
              </div>
            </div>

            <div class="text-xs text-slate-500 flex justify-between border-t border-slate-100 pt-2.5 mt-2.5">
              <span>Total Request: <strong class="text-slate-700">${acc.totalRequests || 0}</strong></span>
              <span class="inline-flex items-center gap-1.5">${acc.cooldownUntil && new Date(acc.cooldownUntil) > new Date() ? svgIcon('clock') + ' Cooldown sampai ' + new Date(acc.cooldownUntil).toLocaleTimeString() : svgIcon('check') + ' Status: Ready'}</span>
            </div>
          </div>
        `;
      }).join('');
    }
    async function refreshAccountQuota(email, btnEl) {
      let origHtml = '';
      if (btnEl) {
        origHtml = btnEl.innerHTML;
        btnEl.disabled = true;
        btnEl.innerHTML = `${svgIcon('refresh-cw', 'w-3 h-3 animate-spin')} Sinkron...`;
      }
      try {
        await fetch('/api/accounts', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ email, action: 'refresh' }),
        });
        await fetchAccounts();
      } finally {
        if (btnEl) {
          btnEl.disabled = false;
          btnEl.innerHTML = origHtml;
        }
      }
    }

    async function refreshAllAccounts(btnEl) {
      let origHtml = '';
      if (btnEl) {
        origHtml = btnEl.innerHTML;
        btnEl.disabled = true;
        btnEl.innerHTML = `${svgIcon('refresh-cw', 'w-3.5 h-3.5 animate-spin')} Menyinkronkan...`;
      }
      try {
        await Promise.all((accountsData || []).map(a => 
          fetch('/api/accounts', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email: a.email, action: 'refresh' }),
          })
        ));
        await fetchAccounts();
      } finally {
        if (btnEl) {
          btnEl.disabled = false;
          btnEl.innerHTML = origHtml;
        }
      }
    }

    function formatDateTime(isoString) {
      if (!isoString || isoString.startsWith('0001')) return 'Belum pernah';
      try {
        const d = new Date(isoString);
        return d.toLocaleDateString('id-ID', {
          day: '2-digit', month: 'short', year: 'numeric',
          hour: '2-digit', minute: '2-digit'
        }) + ' WIB';
      } catch {
        return isoString;
      }
    }

    async function clearAccountCooldown(email) {
      try {
        await fetch('/api/accounts', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ email, action: 'clear_cooldown' }),
        });
        fetchAccounts();
      } catch (err) {
        alert('Gagal mereset cooldown: ' + err.message);
      }
    }

    function toggleProviderAccordion() {
      isProviderCollapsed = !isProviderCollapsed;
      const body = document.getElementById('providerAccordionBody');
      const arrow = document.getElementById('providerAccordionArrow');
      if (body) {
        body.classList.toggle('collapsed', isProviderCollapsed);
      }
      if (arrow) {
        arrow.innerHTML = isProviderCollapsed ? svgIcon('chevron-down') + ' Buka Detail' : svgIcon('chevron-up') + ' Tutup Detail';
      }
    }

    function renderWeeklyModelLimitBox(acc) {
      const families = acc.families || {};
      const gemini = families.gemini || null;
      const claude = families.claude || null;

      // Extract Gemini limits & reset
      const gemini5h = gemini?.limit5h?.remainingPercent !== undefined ? Number(gemini.limit5h.remainingPercent) : (acc.quota5hPercent !== undefined ? Number(acc.quota5hPercent) : 100);
      const geminiWk = gemini?.limitWeekly?.remainingPercent !== undefined ? Number(gemini.limitWeekly.remainingPercent) : (acc.quotaWeeklyPercent !== undefined ? Number(acc.quotaWeeklyPercent) : 100);
      const gemini5hReset = gemini?.limit5h?.resetAt || acc.quota5hResetAt || '';
      const geminiWkReset = gemini?.limitWeekly?.resetAt || acc.quotaWeeklyResetAt || '';

      // Extract Claude & GPT limits & reset
      const claude5h = claude?.limit5h?.remainingPercent !== undefined ? Number(claude.limit5h.remainingPercent) : (acc.quota5hPercent !== undefined ? Number(acc.quota5hPercent) : 100);
      const claudeWk = claude?.limitWeekly?.remainingPercent !== undefined ? Number(claude.limitWeekly.remainingPercent) : (acc.quotaWeeklyPercent !== undefined ? Number(acc.quotaWeeklyPercent) : 100);
      const claude5hReset = claude?.limit5h?.resetAt || '';
      const claudeWkReset = claude?.limitWeekly?.resetAt || acc.quotaWeeklyResetAt || '';

      const isBothExhausted = gemini5h <= 0 && claude5h <= 0 && geminiWk <= 0 && claudeWk <= 0;

      return `
        <div class="bg-slate-50/80 border border-slate-200/80 rounded-xl p-3 mt-1.5 flex flex-col gap-2.5">
          <div class="flex items-center justify-between border-b border-slate-200/60 pb-1.5">
            <span class="font-extrabold text-xs text-slate-800 inline-flex items-center gap-1.5">
              <svg class="w-3.5 h-3.5"><use href="#icon-calendar"></use></svg> Status Kuota &amp; Limit Model
            </span>
            <span class="text-[11px] text-slate-500 font-semibold">5-Jam &amp; 7-Hari</span>
          </div>

          <!-- Gemini Family -->
          <div class="bg-white border border-slate-200 rounded-lg p-2.5 space-y-2">
            <div>
              <div class="flex justify-between items-center mb-0.5">
                <span class="font-bold text-xs text-sky-600 inline-flex items-center gap-1">
                  ${svgIcon('cpu', 'w-3 h-3')} Gemini Sesi (5 Jam)
                </span>
                <strong class="text-xs ${gemini5h <= 0 ? 'text-red-600' : (gemini5h < 20 ? 'text-amber-600' : 'text-sky-600')}">${formatQuotaPercent(gemini5h)}</strong>
              </div>
              <div class="progress-bar h-1.5 bg-slate-200 rounded-full overflow-hidden">
                <div class="progress-fill ${gemini5h <= 0 ? 'bg-red-600' : (gemini5h < 20 ? 'bg-amber-500' : 'bg-sky-600')} h-full" style="width: ${Math.min(100, Math.max(0, gemini5h))}%;"></div>
              </div>
            </div>
            <div>
              <div class="flex justify-between items-center mb-0.5">
                <span class="font-semibold text-[11px] text-slate-600">Gemini Mingguan (7 Hari)</span>
                <strong class="text-xs ${geminiWk <= 0 ? 'text-red-600' : (geminiWk < 20 ? 'text-amber-600' : 'text-slate-700')}">${formatQuotaPercent(geminiWk)}</strong>
              </div>
              <div class="progress-bar h-1 bg-slate-100 rounded-full overflow-hidden">
                <div class="progress-fill bg-sky-500 h-full" style="width: ${Math.min(100, Math.max(0, geminiWk))}%;"></div>
              </div>
              <div class="flex justify-between text-[10px] text-slate-500 mt-0.5">
                <span>Reset:</span>
                <strong class="text-slate-600">${geminiWkReset ? formatDateTime(geminiWkReset) : (gemini5hReset ? formatDateTime(gemini5hReset) : '<span class="text-amber-500 font-medium">Sedang memuat...</span>')}</strong>
              </div>
            </div>
          </div>

          <!-- Claude Family -->
          <div class="bg-white border border-slate-200 rounded-lg p-2.5 space-y-2">
            <div>
              <div class="flex justify-between items-center mb-0.5">
                <span class="font-bold text-xs text-purple-600 inline-flex items-center gap-1">
                  ${svgIcon('layers', 'w-3 h-3')} Claude Sesi (5 Jam)
                </span>
                <strong class="text-xs ${claude5h <= 0 ? 'text-red-600' : (claude5h < 20 ? 'text-amber-600' : 'text-purple-600')}">${formatQuotaPercent(claude5h)}</strong>
              </div>
              <div class="progress-bar h-1.5 bg-slate-200 rounded-full overflow-hidden">
                <div class="progress-fill ${claude5h <= 0 ? 'bg-red-600' : (claude5h < 20 ? 'bg-amber-500' : 'bg-purple-600')} h-full" style="width: ${Math.min(100, Math.max(0, claude5h))}%;"></div>
              </div>
            </div>
            <div>
              <div class="flex justify-between items-center mb-0.5">
                <span class="font-semibold text-[11px] text-slate-600">Claude Mingguan (7 Hari)</span>
                <strong class="text-xs ${claudeWk <= 0 ? 'text-red-600' : (claudeWk < 20 ? 'text-amber-600' : 'text-slate-700')}">${formatQuotaPercent(claudeWk)}</strong>
              </div>
              <div class="progress-bar h-1 bg-slate-100 rounded-full overflow-hidden">
                <div class="progress-fill bg-purple-500 h-full" style="width: ${Math.min(100, Math.max(0, claudeWk))}%;"></div>
              </div>
              <div class="flex justify-between text-[10px] text-slate-500 mt-0.5">
                <span>Reset:</span>
                <strong class="text-slate-600">${claudeWkReset ? formatDateTime(claudeWkReset) : (claude5hReset ? formatDateTime(claude5hReset) : '<span class="text-amber-500 font-medium">Sedang memuat...</span>')}</strong>
              </div>
            </div>
          </div>

          ${isBothExhausted && acc.isActive ? `
            <button class="btn btn-sm btn-danger mt-1 w-full text-xs py-1.5 font-bold" onclick="toggleAccount('${escapeHtml(acc.email)}', false)">
              <svg class="icon icon-sm"><use href="#icon-ban"></use></svg> Nonaktifkan Akun (Kedua Model Habis)
            </button>
          ` : ''}
        </div>
      `;
    }
    function renderProvider() {
      const container = document.getElementById('providerGroupingContainer');
      if (!container) return;

      if (!accountsData || accountsData.length === 0) {
        container.innerHTML = `
          <div class="text-center py-12 text-slate-500 bg-white/60 rounded-2xl border border-dashed border-slate-300 space-y-3">
            <p class="text-sm font-semibold text-slate-600">Belum ada akun Google terdaftar di sistem.</p>
            <div>
              <button class="btn btn-sm btn-primary inline-flex items-center gap-1.5 shadow-sm" onclick="loginGoogle()">
                ${svgIcon('plus', 'icon-sm')} Login Akun Google Sekarang
              </button>
            </div>
          </div>
        `;
        return;
      }

      const totalAccs = accountsData.length;
      const activeAccs = accountsData.filter(a => a.isActive).length;
      const inCooldownAccs = accountsData.filter(a => a.cooldownUntil && new Date(a.cooldownUntil) > new Date()).length;

      container.innerHTML = `
        <div class="provider-accordion">
          <div class="provider-header" onclick="toggleProviderAccordion()">
            <div class="flex items-center gap-3.5">
              <div class="w-11 h-11 rounded-xl bg-slate-900 flex items-center justify-center shadow-md border border-white/10 shrink-0">
                <img src="./assets/images/antigravity.png"/>
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <strong class="text-base font-extrabold text-slate-900">Google Antigravity</strong>
                </div>
                <div class="text-xs text-slate-500 mt-0.5">
                  Cloud Code Assist &amp; Gemini Code Companion &bull; ${totalAccs} Akun Terdaftar (${activeAccs} Aktif${inCooldownAccs > 0 ? ', ' + inCooldownAccs + ' Cooldown' : ''})
                </div>
              </div>
            </div>
            <div class="flex items-center gap-3">
              <button class="btn btn-sm btn-primary inline-flex items-center gap-1.5 shadow-xs" onclick="event.stopPropagation(); loginGoogle()">
                ${svgIcon('plus', 'icon-sm')} Login Akun Google
              </button>
              <span id="providerAccordionArrow" class="text-xs font-bold text-slate-500 inline-flex items-center gap-1 cursor-pointer">
                ${isProviderCollapsed ? svgIcon('chevron-down') + ' Buka Detail' : svgIcon('chevron-up') + ' Tutup Detail'}
              </span>
            </div>
          </div>

          <div class="provider-body ${isProviderCollapsed ? 'collapsed' : ''}" id="providerAccordionBody">
            <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              ${accountsData.map((acc, idx) => {
                const isCooldown = acc.cooldownUntil && new Date(acc.cooldownUntil) > new Date();
                return `
                  <div class="item-card ${isCooldown ? 'border-rose-400 bg-rose-50/30' : ''}">
                    <!-- Card Top Header -->
                    <div class="item-card-header">
                      <div class="flex items-center gap-2.5 min-w-0">
                        <div class="relative shrink-0">
                          <div class="item-avatar bg-slate-900 border border-white/10 flex items-center justify-center ${!acc.isActive ? 'opacity-60 grayscale' : ''}">${acc.picture ? `<img src="${acc.picture}" alt="" onerror="this.classList.add('hidden'); this.parentElement.textContent='${acc.email ? acc.email[0].toUpperCase() : 'G'}';" />` : (acc.email ? acc.email[0].toUpperCase() : 'G')}</div>
                          ${!acc.isActive ? `
                            <span class="absolute -bottom-1 -right-1 w-5 h-5 rounded-full bg-red-600 text-white flex items-center justify-center shadow-sm ring-2 ring-white" title="Akun Dinonaktifkan">
                              <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round">
                                <line x1="18" y1="6" x2="6" y2="18"></line>
                                <line x1="6" y1="6" x2="18" y2="18"></line>
                              </svg>
                            </span>
                          ` : ''}
                        </div>
                        <div class="min-w-0">
                          <div class="font-extrabold text-sm text-slate-900 truncate flex items-center gap-1.5">
                            <span class="truncate">${escapeHtml(acc.name || acc.email)}</span>
                            ${acc.isPrimary ? '<span class="badge badge-primary text-[10px] px-1.5 py-0.5">Primary</span>' : ''}
                          </div>
                          <div class="text-xs text-slate-500 truncate">${escapeHtml(acc.email)}</div>
                        </div>
                      </div>
                      <div class="flex items-center gap-1 shrink-0">
                        <button class="btn btn-sm px-2 py-0.5 text-xs" title="Pindahkan ke atas (Prioritas lebih tinggi)" ${idx === 0 ? 'disabled' : ''} onclick="moveAccountOrder(${idx}, -1)">&uarr;</button>
                        <button class="btn btn-sm px-2 py-0.5 text-xs" title="Pindahkan ke bawah (Prioritas lebih rendah)" ${idx === accountsData.length - 1 ? 'disabled' : ''} onclick="moveAccountOrder(${idx}, 1)">&darr;</button>
                      </div>
                    </div>

                    <!-- Meta -->
                    <div class="text-xs text-slate-600 flex flex-col gap-1.5 mt-1.5">
                      <div class="flex justify-between">
                        <span class="text-slate-500">Total Request:</span>
                        <span class="font-bold text-slate-800">${acc.totalRequests || 0}</span>
                      </div>
                      ${acc.tokenExpiresAt ? `
                        <div class="flex justify-between">
                          <span class="text-slate-500">Token Expire:</span>
                          <span class="text-slate-600 font-medium">${new Date(acc.tokenExpiresAt).toLocaleTimeString()}</span>
                        </div>
                      ` : ''}
                      ${acc.quotaFetchedAt ? `
                        <div class="flex justify-between">
                          <span class="text-slate-500">Terakhir Sync:</span>
                          <span class="text-slate-600 font-medium">${formatDateTime(acc.quotaFetchedAt)}</span>
                        </div>
                      ` : ''}

                      <!-- Weekly Limit Breakdown Component -->
                      ${renderWeeklyModelLimitBox(acc)}

                      ${acc.cooldownUntil && new Date(acc.cooldownUntil) > new Date() ? `
                        <div class="bg-rose-50 border border-rose-200 rounded-xl p-2.5 mt-1.5">
                          <div class="text-rose-600 font-bold text-xs inline-flex items-center gap-1.5">${svgIcon('alert-triangle')} Status / Cooldown:</div>
                          <div class="text-rose-900 text-xs mt-1 max-h-12 overflow-y-auto">${escapeHtml(acc.cooldownReason || 'Rate limit tercapai')}</div>
                          <button class="btn btn-sm mt-1.5 text-xs py-1 px-2.5 bg-white text-rose-600 border-rose-300 hover:bg-rose-50" onclick="clearAccountCooldown('${escapeHtml(acc.email)}')">${svgIcon("trash-2")} Reset Status Cooldown</button>
                        </div>
                      ` : ''}
                    </div>

                    <!-- Actions -->
                    <div class="flex gap-2 justify-end mt-3 pt-2.5 border-t border-slate-100 flex-wrap">
                      <button class="btn btn-sm inline-flex items-center gap-1.5" title="Refresh kuota & limit langsung dari Google" onclick="refreshAccountQuota('${escapeHtml(acc.email)}', this)">${svgIcon('refresh-cw', 'icon-sm')} Refresh Kuota</button>
                      <button class="btn btn-sm" onclick="toggleAccount('${escapeHtml(acc.email)}', ${!acc.isActive})">${acc.isActive ? 'Nonaktifkan' : 'Aktifkan'}</button>
                      <button class="btn btn-sm btn-danger" onclick="deleteAccount('${escapeHtml(acc.email)}')">Hapus</button>
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>
        </div>
      `;
    }
    async function toggleAccount(email, isActive) {
      await fetch('/api/accounts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, isActive }),
      });
      fetchAccounts();
    }

    async function moveAccountOrder(idx, direction) {
      const newIdx = idx + direction;
      if (newIdx < 0 || newIdx >= accountsData.length) return;

      const item = accountsData.splice(idx, 1)[0];
      accountsData.splice(newIdx, 0, item);
      renderProvider();

      try {
        const order = accountsData.map(a => a.email);
        await fetch('/api/accounts', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ order }),
        });
      } catch (err) {
        console.error('Failed to save account order:', err);
      }
    }

    async function saveProviderStrategy(strategy) {
      try {
        await fetch('/api/provider/strategy', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ strategy }),
        });
      } catch (err) {
        console.error('Failed to save provider strategy:', err);
      }
    }

    async function fetchProviderStrategy() {
      try {
        const res = await fetch('/api/provider/strategy');
        const data = await res.json();
        const sel = document.getElementById('providerStrategySelect');
        if (sel && data.strategy) {
          sel.value = data.strategy;
        }
      } catch (err) {
        console.error('Failed to fetch provider strategy:', err);
      }
    }

    function escapeHtml(str) {
      if (!str) return '';
      return String(str).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[m]);
    }

    let confirmModalAction = null;

    function showDestructiveConfirm({ title, subtitle, desc, verifyText, verifyLabel, onConfirm }) {
      const modal = document.getElementById('confirmModal');
      const titleEl = document.getElementById('confirmModalTitle');
      const subtitleEl = document.getElementById('confirmModalSubtitle');
      const descEl = document.getElementById('confirmModalDesc');
      const inputEl = document.getElementById('confirmModalInput');
      const labelEl = document.getElementById('confirmModalInputLabel');
      const btnEl = document.getElementById('confirmModalBtn');

      titleEl.textContent = title || 'Konfirmasi Tindakan';
      subtitleEl.textContent = subtitle || 'Aksi ini berdampak permanen.';
      descEl.innerHTML = desc || '';
      inputEl.value = '';

      if (verifyText) {
        labelEl.innerHTML = verifyLabel || `Ketik <strong>${escapeHtml(verifyText)}</strong> untuk konfirmasi:`;
        btnEl.disabled = true;
        inputEl.oninput = () => {
          btnEl.disabled = inputEl.value.trim() !== verifyText.trim();
        };
      } else {
        labelEl.innerHTML = 'Klik tombol di bawah untuk melanjutkan:';
        btnEl.disabled = false;
        inputEl.oninput = null;
      }

      confirmModalAction = onConfirm;
      btnEl.onclick = async () => {
        if (confirmModalAction) {
          btnEl.disabled = true;
          btnEl.textContent = 'Memproses...';
          try {
            await confirmModalAction();
          } finally {
            btnEl.textContent = 'Hapus Permanen';
            closeConfirmModal();
          }
        }
      };

      modal.classList.add('active');
      inputEl.focus();
    }

    async function importFromOmniRoute() {
      try {
        const res = await fetch('/api/accounts/import-omniroute', { method: 'POST' });
        const data = await res.json();
        alert(data.message || `Berhasil mengimpor ${data.count} akun dari OmniRoute.`);
        fetchAccounts();
      } catch (err) {
        alert('Gagal mengimpor dari OmniRoute: ' + err.message);
      }
    }

    async function importFromLocalSystem() {
      try {
        const res = await fetch('/api/accounts/import-local', { method: 'POST' });
        const data = await res.json();
        alert(data.message || `Berhasil mendeteksi ${data.count} kredensial dari gcloud/sistem lokal.`);
        fetchAccounts();
      } catch (err) {
        alert('Gagal mengimpor kredensial lokal: ' + err.message);
      }
    }

    function loginGoogle() {
      const width = 600;
      const height = 700;
      const left = window.screenX + (window.outerWidth - width) / 2;
      const top = window.screenY + (window.outerHeight - height) / 2;
      const popup = window.open(
        '/api/auth/login',
        'googleLoginPopup',
        `width=${width},height=${height},left=${left},top=${top},status=no,resizable=yes`
      );
      if (!popup || popup.closed || typeof popup.closed === 'undefined') {
        // Fallback jika browser memblokir popup
        window.location.href = '/api/auth/login';
        return;
      }

      // Pantau jika popup selesai / ditutup
      const prevCount = accountsData ? accountsData.length : 0;
      const timer = setInterval(async () => {
        if (popup.closed) {
          clearInterval(timer);
          setTimeout(async () => {
            await fetchAccounts();
            if (accountsData && accountsData.length > prevCount) {
              switchTab('provider');
            }
          }, 600);
        }
      }, 1000);
    }

    async function sendTestWebhook() {
      const urlInput = document.getElementById('webhookTestUrl');
      const resultDiv = document.getElementById('webhookTestResult');
      const url = urlInput ? urlInput.value.trim() : '';
      if (!url) {
        alert('Masukkan URL webhook terlebih dahulu!');
        return;
      }

      if (resultDiv) {
        resultDiv.textContent = 'Mengirim notifikasi...';
        resultDiv.className = 'text-xs text-center font-medium text-slate-500';
      }

      try {
        const res = await fetch('/api/notifications/webhook', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            webhookUrl: url,
            title: 'Antigravity Gateway Alert',
            message: 'Tes notifikasi berhasil terkirim dari dashboard Antigravity Gateway!'
          })
        });
        const data = await res.json();
        if (data.success) {
          if (resultDiv) {
            resultDiv.textContent = '✓ Berhasil terkirim!';
            resultDiv.className = 'text-xs text-center font-bold text-emerald-600';
          }
          alert('Berhasil! Notifikasi webhook telah terkirim.');
        } else {
          if (resultDiv) {
            resultDiv.textContent = '✗ ' + (data.error || 'Gagal terkirim');
            resultDiv.className = 'text-xs text-center font-bold text-rose-600';
          }
          alert('Gagal mengirim webhook: ' + (data.error || 'Unknown error'));
        }
      } catch (err) {
        if (resultDiv) {
          resultDiv.textContent = '✗ ' + err.message;
          resultDiv.className = 'text-xs text-center font-bold text-rose-600';
        }
        alert('Gagal menghubungi server: ' + err.message);
      }
    }


    let inspectorCurrentPage = 1;
    let inspectorPageSize = 10;

    function changeInspectorPageSize(newSize) {
      inspectorPageSize = parseInt(newSize, 10) || 10;
      inspectorCurrentPage = 1;
      renderInspectorTable();
    }

    function prevInspectorPage() {
      if (inspectorCurrentPage > 1) {
        inspectorCurrentPage--;
        renderInspectorTable();
      }
    }

    function nextInspectorPage() {
      const maxPage = Math.ceil(currentInspectorRecords.length / inspectorPageSize) || 1;
      if (inspectorCurrentPage < maxPage) {
        inspectorCurrentPage++;
        renderInspectorTable();
      }
    }

    function renderInspectorTable() {
      const tbody = document.getElementById('inspectorTableBody');
      const infoEl = document.getElementById('inspectorPaginationInfo');
      const pageIndicator = document.getElementById('inspectorPageIndicator');
      const prevBtn = document.getElementById('inspectorPrevBtn');
      const nextBtn = document.getElementById('inspectorNextBtn');

      if (!tbody) return;

      const totalRecords = currentInspectorRecords.length;
      if (totalRecords === 0) {
        tbody.innerHTML = '<tr><td colspan="8" class="py-10 text-center text-slate-400">Belum ada request terekam.</td></tr>';
        if (infoEl) infoEl.textContent = 'Menampilkan 0 - 0 dari 0 request';
        if (pageIndicator) pageIndicator.textContent = '1 / 1';
        if (prevBtn) prevBtn.disabled = true;
        if (nextBtn) nextBtn.disabled = true;
        return;
      }

      const totalPages = Math.ceil(totalRecords / inspectorPageSize) || 1;
      if (inspectorCurrentPage > totalPages) inspectorCurrentPage = totalPages;
      if (inspectorCurrentPage < 1) inspectorCurrentPage = 1;

      const startIdx = (inspectorCurrentPage - 1) * inspectorPageSize;
      const endIdx = Math.min(startIdx + inspectorPageSize, totalRecords);
      const pageRecords = currentInspectorRecords.slice(startIdx, endIdx);

      tbody.innerHTML = pageRecords.map((r, pageOffset) => {
        const actualIdx = startIdx + pageOffset;
        const t = new Date(r.timestamp).toLocaleTimeString();
        const badgeClass = r.statusCode === 200 ? 'badge-success' : 'badge-danger';
        const tokInfo = `${r.promptTokens || 0} in / ${r.outputTokens || 0} out`;

        return `
          <tr class="border-b border-slate-100 hover:bg-slate-50/80 transition-colors">
            <td class="px-4 py-3 font-mono text-xs text-slate-500">${t}</td>
            <td class="px-4 py-3 font-bold text-slate-800"><code>${escapeHtml(r.endpoint)}</code></td>
            <td class="px-4 py-3"><span class="badge badge-purple text-xs">${escapeHtml(r.model)}</span></td>
            <td class="px-4 py-3 text-slate-500 text-xs">${escapeHtml(r.account)}</td>
            <td class="px-4 py-3"><span class="badge ${badgeClass}">${r.statusCode}</span></td>
            <td class="px-4 py-3 font-semibold font-mono text-xs text-slate-700">${r.durationMs}ms</td>
            <td class="px-4 py-3 text-xs text-slate-500 font-mono">${tokInfo}</td>
            <td class="px-4 py-3 text-right">
              <button class="btn btn-sm btn-primary inline-flex items-center gap-1.5 shadow-xs" onclick="copyInspectorCurl(${actualIdx})">${svgIcon('clipboard')} Copy cURL</button>
            </td>
          </tr>
        `;
      }).join('');

      if (infoEl) {
        infoEl.textContent = `Menampilkan ${startIdx + 1} - ${endIdx} dari ${totalRecords} request`;
      }
      if (pageIndicator) {
        pageIndicator.textContent = `${inspectorCurrentPage} / ${totalPages}`;
      }
      if (prevBtn) {
        prevBtn.disabled = inspectorCurrentPage <= 1;
      }
      if (nextBtn) {
        nextBtn.disabled = inspectorCurrentPage >= totalPages;
      }
    }
    let inspectorAutoRefreshTimer = null;

    function initInspectorAutoRefresh() {
      if (inspectorAutoRefreshTimer) clearInterval(inspectorAutoRefreshTimer);
      inspectorAutoRefreshTimer = setInterval(() => {
        if (currentTab === 'inspector') {
          fetchInspectorLogs();
        }
      }, 5000); // Polling live logs every 5 seconds
    }

    async function fetchInspectorLogs() {
      try {
        const res = await fetch('/api/inspector');
        const data = await res.json();
        currentInspectorRecords = data.records || [];
        renderInspectorTable();
      } catch (e) {
        console.error('Failed to fetch inspector records', e);
      }
    }

    async function copyInspectorCurl(recordIdx) {
      const r = currentInspectorRecords[recordIdx];
      if (!r) return;

      let payload = r.requestBody;
      let prettyPayload = '';
      if (!payload || payload.trim() === '') {
        prettyPayload = JSON.stringify({
          model: r.model,
          messages: [{ role: 'user', content: 'Halo dari cURL replay' }]
        }, null, 2);
      } else {
        try {
          const parsed = JSON.parse(payload);
          prettyPayload = JSON.stringify(parsed, null, 2);
        } catch (e) {
          prettyPayload = payload;
        }
      }

      // Escape payload safely for bash single quotes
      const escapedPayload = prettyPayload.replace(/'/g, "'\\''");
      const curlCmd = `curl -X POST http://localhost:3456${r.endpoint} \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer sk-antigravity-local" \\\n  -d '${escapedPayload}'`;

      try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(curlCmd);
        } else {
          // Fallback textarea copy
          const ta = document.createElement('textarea');
          ta.value = curlCmd;
          document.body.appendChild(ta);
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
        }
        alert('Perintah cURL berhasil disalin ke clipboard!');
      } catch (err) {
        prompt('Salin perintah cURL berikut:', curlCmd);
      }
    }

    async function clearInspectorLogs() {
      showDestructiveConfirm({
        title: 'Bersihkan Buffer Inspector',
        subtitle: 'Konfirmasi Penghapusan Log Permintaan',
        desc: 'Seluruh riwayat log inspeksi live request yang tersimpan dalam buffer in-memory akan dihapus secara permanen.',
        verifyText: 'BERSIHKAN',
        verifyLabel: 'Ketik <strong>BERSIHKAN</strong> untuk mengonfirmasi:',
        onConfirm: async () => {
          await fetch('/api/inspector', { method: 'DELETE' });
          fetchInspectorLogs();
        }
      });
    }

    function closeConfirmModal() {
      const modal = document.getElementById('confirmModal');
      if (modal) modal.classList.remove('active');
      confirmModalAction = null;
    }

    function deleteAccount(email) {
      showDestructiveConfirm({
        title: 'Hapus Akun Google',
        subtitle: 'Konfirmasi Penghapusan Akun',
        desc: `Akun <strong>${escapeHtml(email)}</strong> akan dihapus dari antrian rotasi. Seluruh sesi aktif dan token OAuth akan dicabut permanen.`,
        verifyText: email,
        verifyLabel: `Ketik email akun <strong>${escapeHtml(email)}</strong> untuk konfirmasi:`,
        onConfirm: async () => {
          await fetch(`/api/accounts?email=${encodeURIComponent(email)}`, { method: 'DELETE' });
          fetchAccounts();
        }
      });
    }

    let revealedKeys = new Set();

    async function copyApiKey(key, btnEl) {
      try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(key);
        } else {
          const ta = document.createElement('textarea');
          ta.value = key;
          document.body.appendChild(ta);
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
        }
        if (btnEl) {
          const oldHtml = btnEl.innerHTML;
          btnEl.innerHTML = svgIcon('check', 'icon-sm') + ' Disalin!';
          setTimeout(() => { btnEl.innerHTML = oldHtml; }, 1500);
        }
      } catch (err) {
        alert('Gagal menyalin: ' + err.message);
      }
    }

    function toggleKeyVisibility(keyId) {
      if (revealedKeys.has(keyId)) {
        revealedKeys.delete(keyId);
      } else {
        revealedKeys.add(keyId);
      }
      fetchApiKeys();
    }

    async function fetchApiKeys() {
      try {
        const res = await fetch('/api/keys');
        const data = await res.json();
        keysData = data.keys || [];
        const grid = document.getElementById('apiKeysGrid');
        if (!grid) return;
        grid.innerHTML = keysData.map(k => {
          const isRevealed = revealedKeys.has(k.id);
          const displayKey = isRevealed ? k.key : '••••••••••••';

          return `
          <div class="item-card">
            <div class="item-card-header">
              <div class="font-extrabold text-sm text-slate-900">${escapeHtml(k.name)}</div>
            </div>

            <div class="flex items-center justify-between bg-slate-50 px-3 py-2 rounded-xl border border-slate-200 gap-2">
              <code id="keyText_${k.id}" class="font-mono text-xs text-slate-900 break-all select-all flex-1 ${isRevealed ? 'tracking-normal' : 'tracking-widest'}">${escapeHtml(displayKey)}</code>
              <div class="flex items-center gap-1 shrink-0">
                <button class="btn btn-sm bg-white border border-slate-200 text-slate-600 px-2.5 py-1" title="${isRevealed ? 'Sembunyikan API Key' : 'Tampilkan API Key'}" onclick="toggleKeyVisibility('${k.id}')">
                  ${svgIcon(isRevealed ? 'eye-off' : 'eye', 'icon-sm')}
                </button>
                <button class="btn btn-sm btn-primary px-2.5 py-1 shadow-xs" title="Salin API Key Asli" onclick="copyApiKey('${k.key}', this)">
                  ${svgIcon('clipboard', 'icon-sm')} Salin
                </button>
              </div>
            </div>

            <div class="flex justify-between items-center text-xs text-slate-500 pt-2 border-t border-slate-100">
              <span>Dibuat: <strong class="text-slate-700">${new Date(k.createdAt).toLocaleDateString()}</strong></span>
              <span>Total Request: <strong class="text-slate-800">${k.requests || 0}</strong></span>
              <button class="btn btn-sm btn-danger px-2 py-1 text-xs" onclick="deleteApiKey('${k.key}')">${svgIcon('trash-2', 'icon-sm')} Hapus</button>
            </div>
          </div>
          `;
        }).join('');
      } catch (err) {
        console.error(err);
      }
    }
    async function openCreateKeyModal() {
      const name = prompt('Nama API Key:', 'Production Key');
      if (!name) return;
      const defaultSkill = prompt('Combo Skills bawaan (pisahkan koma):', 'rtk,caveman,ponytail,context_mode,codegraph');
      await fetch('/api/keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, defaultSkill: defaultSkill || 'rtk,caveman,ponytail,context_mode,codegraph' }),
      });
      fetchApiKeys();
    }

    function deleteApiKey(key) {
      const shortKey = key.length > 16 ? key.slice(0, 14) + '...' : key;
      showDestructiveConfirm({
        title: 'Hapus API Key',
        subtitle: 'Konfirmasi Penghapusan Kunci',
        desc: `API Key <code>${escapeHtml(shortKey)}</code> akan dihapus permanen. Setiap aplikasi atau IDE agent yang menggunakan kunci ini tidak dapat lagi mengakses gateway.`,
        verifyText: 'HAPUS',
        verifyLabel: 'Ketik <strong>HAPUS</strong> untuk mengonfirmasi:',
        onConfirm: async () => {
          await fetch(`/api/keys?key=${encodeURIComponent(key)}`, { method: 'DELETE' });
          fetchApiKeys();
        }
      });
    }

    async function fetchSkills() {
      try {
        const res = await fetch('/api/skills');
        const data = await res.json();
        skillsData = data.skills || [];
        const grid = document.getElementById('skillsGrid');
        if (!grid) return;
        if (skillsData.length === 0) {
          grid.innerHTML = `
            <div class="col-span-full text-center py-8 text-slate-500 bg-white/60 rounded-2xl border border-dashed border-slate-300 space-y-3">
              <p class="text-sm font-semibold text-slate-600">Belum ada skill aktif terdaftar.</p>
              <div>
                <button class="btn btn-sm btn-primary inline-flex items-center gap-1.5" onclick="openCreateSkillModal()">
                  <svg class="w-3.5 h-3.5"><use href="#icon-plus"></use></svg> Tambah Skill
                </button>
              </div>
            </div>
          `;
          renderComboSkillsPills();
          return;
        }

        grid.innerHTML = skillsData.map(sk => `
          <div class="item-card flex flex-col justify-between ${!sk.enabled ? 'opacity-75 bg-slate-50/70 border-dashed' : ''}">
            <div class="space-y-2.5">
              <div class="item-card-header items-center">
                <div class="font-extrabold text-sm text-slate-900 flex items-center gap-2">
                  <span>${escapeHtml(sk.name)}</span>
                </div>
                <div class="flex items-center gap-1.5 shrink-0">
                  <span class="badge ${sk.isBuiltin ? 'badge-purple' : 'badge-blue'} text-[10px]">${sk.isBuiltin ? 'Builtin' : 'Custom'}</span>
                  <span class="badge ${sk.enabled ? 'badge-success' : 'badge-neutral opacity-80'} text-[10px] inline-flex items-center gap-1">
                    <span class="w-1.5 h-1.5 rounded-full ${sk.enabled ? 'bg-emerald-500' : 'bg-slate-400'}"></span>
                    ${sk.enabled ? 'Aktif' : 'Nonaktif'}
                  </span>
                </div>
              </div>
              <p class="text-xs text-slate-500 leading-relaxed">${escapeHtml(sk.description)}</p>
              <pre class="bg-slate-900 text-slate-100 p-3 rounded-xl text-[11px] max-h-36 overflow-y-auto whitespace-pre-wrap break-words font-mono"><code>${escapeHtml(sk.systemPrompt)}</code></pre>
            </div>
            <div class="flex items-center justify-between pt-3 border-t border-slate-100 mt-3 gap-2">
              <button class="btn btn-xs ${sk.enabled ? 'btn-outline text-amber-700 hover:bg-amber-50' : 'btn-primary'} inline-flex items-center gap-1" onclick="toggleSkill('${escapeHtml(sk.id)}')">
                <svg class="w-3 h-3"><use href="${sk.enabled ? '#icon-ban' : '#icon-check'}"></use></svg>
                ${sk.enabled ? 'Nonaktifkan' : 'Aktifkan'}
              </button>
              <button class="btn btn-xs btn-outline text-red-600 hover:bg-red-50 hover:text-red-700 inline-flex items-center gap-1" onclick="deleteSkillPrompt('${escapeHtml(sk.id)}', '${escapeHtml(sk.name)}', ${Boolean(sk.isBuiltin)})">
                <svg class="w-3 h-3"><use href="#icon-trash-2"></use></svg>
                Hapus
              </button>
            </div>
          </div>
        `).join('');
        renderComboSkillsPills();
      } catch (err) {
        console.error('fetchSkills error:', err);
      }
    }

    async function toggleSkill(id) {
      try {
        const res = await fetch(`/api/skills?action=toggle&id=${encodeURIComponent(id)}`, { method: 'POST' });
        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          alert(err.error || 'Gagal mengubah status skill');
          return;
        }
        await fetchSkills();
      } catch (e) {
        console.error('toggleSkill error:', e);
      }
    }

    function deleteSkillPrompt(id, name, isBuiltin) {
      showDestructiveConfirm({
        title: 'Hapus Skill',
        subtitle: 'Konfirmasi Penghapusan Permanen',
        desc: `Skill <strong>${escapeHtml(name)}</strong> (<code>${escapeHtml(id)}</code>) ${isBuiltin ? 'merupakan skill bawaan gateway' : 'adalah custom skill'}. Menghapus skill ini akan menghilangkannya secara permanen dari daftar prompt injection dan selector.`,
        verifyText: name,
        verifyLabel: `Ketik nama skill <strong>${escapeHtml(name)}</strong> untuk mengonfirmasi:`,
        onConfirm: async () => {
          try {
            const res = await fetch(`/api/skills?id=${encodeURIComponent(id)}`, { method: 'DELETE' });
            if (!res.ok) {
              const err = await res.json().catch(() => ({}));
              alert(err.error || 'Gagal menghapus skill');
              return;
            }
            await fetchSkills();
          } catch (e) {
            console.error('deleteSkill error:', e);
          }
        }
      });
    }

    function resetBuiltinSkills() {
      showDestructiveConfirm({
        title: 'Pulihkan Skill Bawaan',
        subtitle: 'Reset Preset Default',
        desc: 'Tindakan ini akan memulihkan dan mengaktifkan kembali seluruh Built-in Skills (RTK, Caveman, Ponytail, Context Mode, CodeGraph, Architect, Grill Me) ke konfigurasi bawaan.',
        verifyText: '',
        onConfirm: async () => {
          try {
            const res = await fetch('/api/skills?action=reset_builtins', { method: 'POST' });
            if (!res.ok) {
              const err = await res.json().catch(() => ({}));
              alert(err.error || 'Gagal memulihkan skill bawaan');
              return;
            }
            await fetchSkills();
          } catch (e) {
            console.error('resetBuiltinSkills error:', e);
          }
        }
      });
    }

    function openCreateSkillModal() {
      const modal = document.getElementById('createSkillModal');
      const form = document.getElementById('createSkillForm');
      if (form) form.reset();
      if (modal) {
        modal.classList.add('active');
        const nameInput = document.getElementById('skillModalName');
        if (nameInput) nameInput.focus();
      }
    }

    function closeCreateSkillModal() {
      const modal = document.getElementById('createSkillModal');
      if (modal) modal.classList.remove('active');
    }

    async function handleCreateSkillSubmit(e) {
      if (e) e.preventDefault();
      const name = document.getElementById('skillModalName')?.value?.trim();
      const description = document.getElementById('skillModalDesc')?.value?.trim() || '';
      const systemPrompt = document.getElementById('skillModalPrompt')?.value?.trim();
      const btn = document.getElementById('btnSaveSkill');

      if (!name || !systemPrompt) {
        alert('Nama Skill dan System Prompt wajib diisi.');
        return;
      }

      if (btn) {
        btn.disabled = true;
        btn.textContent = 'Menyimpan...';
      }

      try {
        const res = await fetch('/api/skills', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, description, systemPrompt, enabled: true }),
        });

        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          alert(err.error || 'Gagal menyimpan skill');
          return;
        }

        closeCreateSkillModal();
        await fetchSkills();
      } catch (err) {
        console.error('handleCreateSkillSubmit error:', err);
        alert('Terjadi kesalahan saat menyimpan skill');
      } finally {
        if (btn) {
          btn.disabled = false;
          btn.innerHTML = `<svg class="w-3.5 h-3.5"><use href="#icon-check"></use></svg> Simpan Skill`;
        }
      }
    }

    // Playground account selector & Combo skills state (Default: Combo All Active Skills)
    let selectedComboSkills = new Set(['rtk', 'caveman', 'ponytail', 'context_mode', 'codegraph']);

    function selectAllComboSkills() {
      const activeSkills = skillsData.length > 0
        ? skillsData.filter(sk => sk.enabled)
        : [
            { id: 'rtk' }, { id: 'caveman' }, { id: 'ponytail' }, { id: 'context_mode' }, { id: 'codegraph' }
          ];
      selectedComboSkills = new Set(activeSkills.map(s => s.id.toLowerCase()));
      renderComboSkillsPills();
    }

    function updatePgAccountsDropdown() {
      const select = document.getElementById('pgAccount');
      if (!select) return;
      const currentVal = select.value;
      let opts = '<option value="">Auto (Account Pool & Failover)</option>';
      accountsData.forEach(acc => {
        opts += `<option value="${acc.email}" ${currentVal === acc.email ? 'selected' : ''}>${acc.name || acc.email} (${acc.isActive ? 'Active' : 'Disabled'})</option>`;
      });
      select.innerHTML = opts;
      updatePgAccountBadge();
    }

    function updatePgAccountBadge() {
      const select = document.getElementById('pgAccount');
      const badge = document.getElementById('pgActiveAccountBadge');
      if (!select || !badge) return;
      if (select.value) {
        badge.innerHTML = svgIcon('key', 'icon-sm') + ' Target: ' + select.value.split('@')[0];
        badge.classList.remove('hidden');
        badge.classList.add('inline-flex');
      } else {
        badge.innerHTML = svgIcon('zap', 'icon-sm') + ' Pool Auto-Rotate';
        badge.classList.remove('hidden');
        badge.classList.add('inline-flex');
      }
    }

    function renderComboSkillsPills() {
      const container = document.getElementById('pgSkillsPills');
      if (!container) return;

      const defaultSkills = [
        { id: 'grill-me', name: 'Grill Me', desc: 'Autonomous Prompt Architect & Interview', enabled: true },
        { id: 'rtk', name: 'RTK', desc: 'Real-Time Knowledge & Reasoning', enabled: true },
        { id: 'context_mode', name: 'Context Mode', desc: 'Context Engineering', enabled: true },
        { id: 'codegraph', name: 'CodeGraph', desc: 'Symbol & Dependency Knowledge', enabled: true },
        { id: 'caveman', name: 'Caveman', desc: 'Terse & No Fluff', enabled: true },
        { id: 'ponytail', name: 'Ponytail', desc: 'Lazy Senior Dev', enabled: true },
        { id: 'architect', name: 'Architect', desc: 'Software Architect', enabled: true },
      ];

      // Use active skills from skillsData if loaded, otherwise fallback to default active list
      let activeSkills = [];
      if (skillsData.length > 0) {
        activeSkills = skillsData.filter(sk => sk.enabled);
      } else {
        activeSkills = defaultSkills;
      }

      // Automatically prune inactive or deleted skills from the user selection
      const activeIdSet = new Set(activeSkills.map(s => s.id.toLowerCase()));
      for (const id of Array.from(selectedComboSkills)) {
        if (!activeIdSet.has(id)) {
          selectedComboSkills.delete(id);
        }
      }

      container.innerHTML = activeSkills.map(sk => {
        const isChecked = selectedComboSkills.has(sk.id.toLowerCase());
        return `
          <div class="skill-pill ${isChecked ? 'active' : ''}" onclick="toggleComboSkill('${sk.id.toLowerCase()}')">
            <span>${isChecked ? svgIcon('check', 'icon-sm') : svgIcon('plus', 'icon-sm')}</span>
            <span>${escapeHtml(sk.name)}</span>
          </div>
        `;
      }).join('');
    }

    function toggleComboSkill(skillId) {
      if (selectedComboSkills.has(skillId)) {
        selectedComboSkills.delete(skillId);
      } else {
        selectedComboSkills.add(skillId);
      }
      renderComboSkillsPills();
    }

    function clearComboSkills() {
      selectedComboSkills.clear();
      renderComboSkillsPills();
    }

    let playgroundAbortController = null;

    function handlePlaygroundAction() {
      if (playgroundAbortController) {
        // Active stream: click stops it
        playgroundAbortController.abort();
        playgroundAbortController = null;
        updatePlaygroundButtonState(false);
      } else {
        sendPlaygroundMessage();
      }
    }

    function updatePlaygroundButtonState(isStreaming) {
      const btn = document.getElementById('pgSendBtn');
      const input = document.getElementById('pgInput');
      if (!btn) return;

      if (isStreaming) {
        btn.innerHTML = `<svg class="icon icon-sm"><use href="#icon-square"></use></svg><span>Stop</span>`;
        btn.className = 'btn btn-danger bg-red-600 hover:bg-red-700 text-white border-red-600';
        if (input) input.disabled = true;
      } else {
        btn.innerHTML = `<svg class="icon icon-sm"><use href="#icon-play"></use></svg><span>Kirim</span>`;
        btn.className = 'btn btn-primary';
        if (input) {
          input.disabled = false;
          input.focus();
        }
      }
    }

    async function sendPlaygroundMessage() {
      if (playgroundAbortController) return;

      const input = document.getElementById('pgInput');
      const text = input.value.trim();
      if (!text) return;

      input.value = '';
      const model = document.getElementById('pgModel').value;
      const targetAccount = document.getElementById('pgAccount')?.value || '';
      const skillsArray = Array.from(selectedComboSkills);

      userScrolledUpInPlayground = false;
      playgroundHistory.push({ role: 'user', content: text });
      renderPlaygroundMessages(true);

      const statusEl = document.getElementById('pgStatus');
      if (statusEl) {
        statusEl.textContent = 'Generating...';
        statusEl.className = 'badge badge-warning';
      }

      playgroundAbortController = new AbortController();
      updatePlaygroundButtonState(true);

      // Assistant streaming placeholder
      const assistantTurn = { role: 'assistant', content: '' };
      playgroundHistory.push(assistantTurn);

      try {
        const headers = {
          'Content-Type': 'application/json',
          'x-playground-request': 'true',
        };
        if (targetAccount) headers['x-target-account'] = targetAccount;

        const res = await fetch('/v1/chat/completions', {
          method: 'POST',
          headers: headers,
          signal: playgroundAbortController.signal,
          body: JSON.stringify({
            model: model,
            messages: playgroundHistory.slice(0, -1).map(m => ({ role: m.role, content: m.content })),
            stream: true,
            account: targetAccount,
            skills: skillsArray,
          }),
        });

        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          assistantTurn.content = 'Error: ' + (err.error?.message || res.statusText);
          renderPlaygroundMessages();
          return;
        }

        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          let hasNewContent = false;
          for (const line of lines) {
            if (line.startsWith('data: ')) {
              const dataStr = line.slice(6).trim();
              if (dataStr === '[DONE]') continue;
              try {
                const chunk = JSON.parse(dataStr);
                const delta = chunk.choices?.[0]?.delta?.content || '';
                if (delta) {
                  assistantTurn.content += delta;
                  hasNewContent = true;
                }
              } catch (e) {
                console.warn('SSE Chunk parse error:', e);
              }
            }
          }
          if (hasNewContent) {
            renderPlaygroundMessages();
          }
        }
        // Pastikan render akhir dipanggil saat streaming selesai ([DONE])
      } catch (err) {
        console.error('Playground fetch error:', err);
        if (err.name === 'AbortError') {
          assistantTurn.content += '\n\n*(Generasi dihentikan oleh pengguna)*';
        } else {
          assistantTurn.content = 'Network Error: ' + err.message;
        }
      } finally {
        playgroundAbortController = null;
        updatePlaygroundButtonState(false);
        const statusEl = document.getElementById('pgStatus');
        if (statusEl) {
          statusEl.textContent = 'Ready';
          statusEl.className = 'badge badge-success';
        }
        // Panggil render final SETELAH playgroundAbortController di-set ke null
        renderPlaygroundMessages();
      }
    }

    function formatChatContent(content, isStreaming = false) {
      if (!content) {
        return isStreaming ? '<span class="inline-flex items-center gap-1.5 text-xs text-slate-400 italic"><span class="w-2 h-2 rounded-full bg-blue-500 animate-pulse"></span> Sedang mengetik...</span>' : '';
      }

      // Check if message is an Interrogation / Grill-me format
      if (content.includes('❓') || content.includes('INTERROGATION') || content.includes('Q1')) {
        // Jika sedang streaming dan belum selesai menerima respons penuh, tampilkan preview teks rapi dulu
        if (isStreaming) {
          return cleanGrillMarkdown(content).replace(/\n/g, '<br/>') + ' <span class="inline-block w-1.5 h-3.5 bg-blue-500 animate-pulse align-middle ml-1"></span>';
        }
        return renderInteractiveGrillMe(content);
      }

      // Format Master Prompt output cleanly (preserve markdown headers, bold, and code blocks)
      let formatted = content
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');

      // Format code blocks
      formatted = formatted.replace(/```([a-zA-Z0-9_-]*)\n([\s\S]*?)```/g, (match, lang, code) => {
        return `<pre class="bg-slate-900 text-slate-100 p-4 rounded-xl overflow-x-auto font-mono text-xs my-3 shadow-inner"><code>${code}</code></pre>`;
      });

      // Headers and bolding
      formatted = formatted
        .replace(/^### (.*$)/gim, '<h4 class="text-sm font-extrabold text-slate-900 mt-3 mb-1.5">$1</h4>')
        .replace(/^## (.*$)/gim, '<h3 class="text-base font-extrabold text-slate-900 mt-4 mb-2 pb-1 border-b border-slate-200/80">$1</h3>')
        .replace(/^# (.*$)/gim, '<h2 class="text-lg font-extrabold text-slate-900 mt-4 mb-2 pb-1 border-b-2 border-slate-300">$1</h2>')
        .replace(/\*\*(.*?)\*\*/g, '<strong class="font-bold text-slate-900">$1</strong>')
        .replace(/\n/g, '<br/>');

      return formatted;
    }

    // Interactive Interrogation Form parser (Antigravity Style)
    function cleanGrillMarkdown(text) {
      if (!text) return '';
      return text
        .replace(/\*\*([^*]+)\*\*/g, '$1') // Buang bold **text** -> text
        .replace(/\*([^*]+)\*/g, '$1')     // Buang italic *text* -> text
        .replace(/`([^`]+)`/g, '$1')       // Buang backticks `code` -> code
        .replace(/\*\*/g, '')              // Buang sisa asterisks ganda yang tidak berpasangan
        .replace(/\*/g, '')                // Buang sisa asterisks tunggal
        .trim();
    }

    function renderInteractiveGrillMe(rawText) {
      console.log('[Grill-Me] renderInteractiveGrillMe called with text length:', rawText?.length);
      // Split into sections by question marker
      const lines = rawText.split('\n');
      const questions = [];
      let currentQ = null;
      let headerLines = [];

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i].trim();
        // Lewati garis horizontal pemisah markdown
        if (line === '---' || line === '***' || line === '___') {
          continue;
        }

        // Deteksi judul pertanyaan fleksibel: diawali '❓', '?', '## Q', 'Q1', dsb.
        const isQLine = /(?:❓|\?|\bQ\d+[:\s\-]|\*\*Q\d+)/i.test(line);
        if (isQLine && !/^[-*•\d.]*\s*\[[A-Z]\]/i.test(line)) {
          if (currentQ) questions.push(currentQ);
          const rawTitle = line.replace(/^[❓\?\*#\s\d.-]+/g, '');
          const cleanedTitle = cleanGrillMarkdown(rawTitle);
          const isMulti = /multi|ganda|pilih lebih dari satu|kombinasi/i.test(cleanedTitle);
          currentQ = {
            title: cleanedTitle,
            isMulti: isMulti,
            options: [],
            recommendation: ''
          };
        } else if (currentQ) {
          // Hanya tangkap baris yang diawali bullet/angka dan tanda kurung siku [A-Z] eksplisit (misal: - [A] Pilihan, [B] Pilihan)
          // Mencegah kata seperti "Berapa" di awal kalimat tertangkap sebagai opsi
          const optMatch = line.match(/^[-*•\d.]*\s*\[([A-Z])\]\s*[:-]?\s*(.*)/i);
          if (optMatch) {
            const rawOptText = cleanGrillMarkdown(optMatch[2]);
            const isWriteIn = /write[- ]?in|tentukan sendiri|isi sendiri|spesifikasi kustom/i.test(rawOptText);
            currentQ.options.push({
              key: optMatch[1].toUpperCase(),
              text: rawOptText,
              isWriteIn: isWriteIn
            });
          } else if (line.includes('➡️') || line.toLowerCase().includes('rekomendasi')) {
            currentQ.recommendation = cleanGrillMarkdown(line.replace(/^[➡️\s*#-]+/, ''));
          } else if (currentQ.options.length === 0 && line.length > 0 && !line.startsWith('---')) {
            currentQ.title += ' ' + cleanGrillMarkdown(line);
          }
        } else if (line.length > 0 && !line.startsWith('---') && !line.startsWith('```')) {
          headerLines.push(cleanGrillMarkdown(line));
        }
      }
      if (currentQ) questions.push(currentQ);

      console.log('[Grill-Me] Parsed questions count:', questions.length);
      // If parser failed to find structured questions, fallback to linebreaks
      if (questions.length === 0) {
        return rawText.replace(/\n/g, '<br/>');
      }

      const qFormId = 'grillForm_' + Math.floor(Math.random() * 100000);

      let html = `
        <div class="bg-white border border-slate-200 rounded-2xl p-5 mt-2 shadow-xs">
          <div class="flex items-center justify-between mb-4 border-b border-slate-100 pb-3">
            <div class="font-extrabold text-sm text-slate-900 flex items-center gap-2">
              <span class="w-6 h-6 rounded-lg bg-indigo-100 text-indigo-600 flex items-center justify-center text-xs">❓</span>
              <span>Interrogation / Clarification Cards</span>
            </div>
            <button class="btn btn-sm btn-primary text-xs px-3 py-1.5 shadow-xs" onclick="autoSelectGrillRecommendations('${qFormId}')">
              ${svgIcon('zap', 'icon-sm')} Pilih Semua Rekomendasi
            </button>
          </div>

          ${headerLines.length > 0 ? `<p class="text-xs text-slate-600 mb-4 leading-relaxed">${escapeHtml(headerLines.join(' '))}</p>` : ''}

          <form id="${qFormId}" class="flex flex-col gap-4">
      `;

      questions.forEach((q, qIdx) => {
        const qName = `q_${qIdx}`;
        // Ambil semua kunci rekomendasi (bisa single 'C' atau kombinasi 'A', 'D' pada rekomendasi: "[A] + [D]")
        const recKeys = new Set();
        if (q.recommendation) {
          const matches = q.recommendation.matchAll(/\[([A-Z])\]/gi);
          for (const m of matches) {
            recKeys.add(m[1].toUpperCase());
          }
          if (recKeys.size === 0) {
            const fallbackMatch = q.recommendation.match(/\b([A-Z])\b/g);
            if (fallbackMatch) {
              fallbackMatch.forEach(k => recKeys.add(k.toUpperCase()));
            }
          }
        }

        // Cek juga apakah teks opsi itu sendiri memuat kata "(Rekomendasi)"
        q.options.forEach(opt => {
          if (/rekomendasi/i.test(opt.text)) {
            recKeys.add(opt.key);
          }
        });

        // Wajib setidaknya 1 rekomendasi: jika AI tidak memberikan rekomendasi sama sekali, default ke opsi pertama [A]
        if (recKeys.size === 0 && q.options.length > 0) {
          recKeys.add(q.options[0].key);
        }

        // Jika rekomendasi memuat lebih dari 1 opsi atau judul mengandung multi-select, gunakan checkbox
        const isMultiChoice = q.isMulti || recKeys.size > 1;
        const inputType = isMultiChoice ? 'checkbox' : 'radio';

        html += `
          <div class="p-3.5 rounded-xl bg-slate-50 border border-slate-200/90 flex flex-col gap-2.5">
            <div class="flex items-center justify-between gap-2 flex-wrap">
              <div class="font-bold text-xs text-slate-900 leading-snug">
                ${qIdx + 1}. ${escapeHtml(q.title)}
              </div>
              ${isMultiChoice ? '<span class="text-[10px] font-bold px-2 py-0.5 rounded-full bg-blue-100 text-blue-700">Multi-Select</span>' : ''}
            </div>
            <div class="flex flex-col gap-2">
        `;

        const hasCustomWriteIn = q.options.some(opt => opt.isWriteIn);

        q.options.forEach(opt => {
          const isRec = recKeys.has(opt.key);
          const isChecked = isRec;
          // Bersihkan kata "(Rekomendasi)" dari label jika sudah ada di teks opsi agar rapi dengan badge
          const cleanOptText = opt.text.replace(/\(?rekomendasi\)?/gi, '').trim();

          html += `
            <label class="group flex flex-col p-2.5 rounded-lg border transition-all cursor-pointer ${isChecked ? 'border-blue-400 bg-blue-50/70' : 'border-slate-200 bg-white hover:border-blue-300'}" data-grill-opt="true">
              <div class="flex items-start gap-2.5">
                <input type="${inputType}" name="${qName}${isMultiChoice ? '[]' : ''}" value="[${opt.key}] ${escapeHtml(cleanOptText || opt.text)}" class="mt-0.5 text-blue-600 focus:ring-blue-500 rounded-${isMultiChoice ? 'md' : 'full'}" ${isChecked ? 'checked' : ''} onchange="handleGrillOptionChange(this)" />
                <div class="flex-1 text-xs text-slate-700 leading-relaxed">
                  <span class="font-extrabold text-blue-700 mr-1">[${opt.key}]</span>
                  <span>${escapeHtml(cleanOptText || opt.text)}</span>
                  ${isRec ? '<span class="ml-2 inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-bold bg-indigo-100 text-indigo-700">Rekomendasi</span>' : ''}
                </div>
              </div>
              ${opt.isWriteIn ? `
                <div class="mt-2 pl-6">
                  <input type="text" placeholder="Ketik spesifikasi kustom Anda di sini..." class="w-full text-xs px-3 py-1.5 bg-white border border-slate-300 rounded-lg focus:outline-none focus:ring-1 focus:ring-blue-500" oninput="handleGrillCustomInput(this)" onclick="event.stopPropagation()" />
                </div>
              ` : ''}
            </label>
          `;
        });

        // HANYA tambahkan Opsi Write-In jika pertanyaan BELUM memuat opsi write-in
        if (!hasCustomWriteIn) {
          const customKey = String.fromCharCode(65 + q.options.length);
          html += `
            <label class="group flex flex-col p-2.5 rounded-lg border border-slate-200 bg-white hover:border-blue-300 transition-all cursor-pointer" data-grill-opt="true">
              <div class="flex items-start gap-2.5">
                <input type="${inputType}" name="${qName}${isMultiChoice ? '[]' : ''}" value="[${customKey}] Kustom" class="mt-0.5 text-blue-600 focus:ring-blue-500 rounded-${isMultiChoice ? 'md' : 'full'}" onchange="handleGrillOptionChange(this)" />
                <div class="flex-1 text-xs text-slate-700 leading-relaxed">
                  <span class="font-extrabold text-slate-500 mr-1">[${customKey}]</span>
                  <span class="font-semibold text-slate-700">Tentukan Sendiri (Custom Write-in)</span>
                </div>
              </div>
              <div class="mt-2 pl-6">
                <input type="text" placeholder="Ketik spesifikasi kustom Anda di sini..." class="w-full text-xs px-3 py-1.5 bg-white border border-slate-300 rounded-lg focus:outline-none focus:ring-1 focus:ring-blue-500" oninput="handleGrillCustomInput(this)" onclick="event.stopPropagation()" />
              </div>
            </label>
          `;
        }

        html += `
            </div>
          </div>
        `;
      });

      html += `
          </form>
          <div class="flex justify-end pt-3 mt-2 border-t border-slate-100">
            <button class="btn btn-sm btn-primary text-xs px-4 py-2 shadow-xs flex items-center gap-2" onclick="submitGrillAnswers('${qFormId}')">
              ${svgIcon('play', 'icon-sm')} Kirim Pilihan &amp; Generate Master Prompt
            </button>
          </div>
        </div>
      `;

      return html;
    }

    function handleGrillOptionChange(inputEl) {
      const parentForm = inputEl.closest('form');
      if (!parentForm) return;
      const qGroup = inputEl.closest('.flex.flex-col.gap-2');
      if (!qGroup) return;

      qGroup.querySelectorAll('label[data-grill-opt="true"]').forEach(lbl => {
        const inp = lbl.querySelector('input[type="radio"], input[type="checkbox"]');
        if (inp && inp.checked) {
          lbl.classList.remove('border-slate-200', 'bg-white');
          lbl.classList.add('border-blue-400', 'bg-blue-50/70');
        } else {
          lbl.classList.remove('border-blue-400', 'bg-blue-50/70');
          lbl.classList.add('border-slate-200', 'bg-white');
        }
      });
    }

    function handleGrillCustomInput(inputEl) {
      const parentLabel = inputEl.closest('label');
      if (!parentLabel) return;
      const input = parentLabel.querySelector('input[type="radio"], input[type="checkbox"]');
      if (input) {
        input.checked = true;
        const matchKey = input.value.match(/^\[([A-Z])\]/);
        const prefix = matchKey ? matchKey[0] : '[Custom]';
        input.value = `${prefix} Custom: ${inputEl.value.trim()}`;
        handleGrillOptionChange(input);
      }
    }

    function autoSelectGrillRecommendations(formId) {
      const form = document.getElementById(formId);
      if (!form) return;
      const cards = form.querySelectorAll('.p-3\\.5');
      cards.forEach(card => {
        const recBadges = card.querySelectorAll('.bg-indigo-100');
        if (recBadges.length > 0) {
          recBadges.forEach(b => {
            const inp = b.closest('label').querySelector('input[type="radio"], input[type="checkbox"]');
            if (inp) {
              inp.checked = true;
              handleGrillOptionChange(inp);
            }
          });
        }
      });
    }

    function submitGrillAnswers(formId) {
      const form = document.getElementById(formId);
      if (!form) return;
      const formData = new FormData(form);
      const answersMap = new Map();

      for (const [key, val] of formData.entries()) {
        const cleanKey = key.replace('[]', '');
        if (!answersMap.has(cleanKey)) {
          answersMap.set(cleanKey, []);
        }
        answersMap.get(cleanKey).push(val);
      }

      const answers = [];
      let idx = 1;
      answersMap.forEach((vals) => {
        answers.push(`Q${idx}: ${vals.join(', ')}`);
        idx++;
      });

      const input = document.getElementById('pgInput');
      if (input) {
        input.value = `Berikut adalah preferensi saya:\n` + answers.join('\n') + `\n\nSilakan lanjutkan dan rumuskan Autonomous Master Prompt sekarang.`;
        sendPlaygroundMessage();
      }
    }

    function copyMessageContent(idx) {
      const m = playgroundHistory[idx];
      if (!m || !m.content) return;
      navigator.clipboard.writeText(m.content).then(() => {
        alert('Teks berhasil disalin ke clipboard!');
      }).catch(err => {
        console.error('Copy failed:', err);
      });
    }

    function exportMessageToMarkdown(idx) {
      const m = playgroundHistory[idx];
      if (!m || !m.content) return;
      const blob = new Blob([m.content], { type: 'text/markdown;charset=utf-8;' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.setAttribute('href', url);
      link.setAttribute('download', `antigravity-response-${Date.now()}.md`);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    }

    let userScrolledUpInPlayground = false;

    function initPlaygroundScrollListener() {
      const el = document.getElementById('pgMessages');
      if (!el || el._hasScrollListener) return;
      el._hasScrollListener = true;
      el.addEventListener('scroll', () => {
        // Toleransi 60px dari dasar chat
        const isNearBottom = (el.scrollHeight - el.scrollTop - el.clientHeight) <= 60;
        userScrolledUpInPlayground = !isNearBottom;
      }, { passive: true });
    }

    function renderPlaygroundMessages(forceScroll = false) {
      const el = document.getElementById('pgMessages');
      if (!el) return;
      initPlaygroundScrollListener();

      const wasAtBottom = (el.scrollHeight - el.scrollTop - el.clientHeight) <= 60;

      el.innerHTML = playgroundHistory.map((m, idx) => `
        <div class="flex flex-col w-full mb-3.5 ${m.role === 'user' ? 'items-end' : 'items-start'}">
          <div class="flex items-center justify-between ${m.role === 'user' ? 'max-w-[82%]' : 'w-full'} mb-1 px-1">
            <span class="text-xs font-bold text-slate-500">
              ${m.role === 'user' ? 'Anda' : (m.model ? `${m.model} (${m.role})` : 'Antigravity Assistant')}
            </span>
            ${m.role === 'assistant' ? `
              <div class="flex gap-1.5 items-center">
                <button class="btn btn-sm px-2 py-0.5 text-xs bg-slate-50 border border-slate-300 text-slate-600 flex items-center gap-1 hover:bg-slate-100" onclick="copyMessageContent(${idx})" title="Salin teks ke clipboard">
                  <svg class="icon icon-sm"><use href="#icon-copy"></use></svg> Salin
                </button>
                <button class="btn btn-sm px-2 py-0.5 text-xs bg-slate-50 border border-slate-300 text-slate-600 flex items-center gap-1 hover:bg-slate-100" onclick="exportMessageToMarkdown(${idx})" title="Unduh sebagai file Markdown (.md)">
                  <svg class="icon icon-sm"><use href="#icon-download"></use></svg> Export .md
                </button>
              </div>
            ` : ''}
          </div>
          <div class="chat-bubble ${m.role} w-full ${m.role === 'user' ? 'max-w-[82%]' : 'max-w-full'}">
            ${m.role === 'assistant' ? formatChatContent(m.content, !!playgroundAbortController && idx === playgroundHistory.length - 1) : m.content.replace(/\n/g, '<br/>')}
          </div>
        </div>
      `).join('');

      // Hanya auto-scroll jika diminta secara paksa (misal user kirim pesan baru)
      // atau jika user sedang berada di posisi paling bawah (belum sengaja scroll ke atas)
      if (forceScroll || (!userScrolledUpInPlayground && wasAtBottom)) {
        el.scrollTop = el.scrollHeight;
      }
    }
    function clearPlayground() {
      playgroundHistory = [];
      renderPlaygroundMessages();
    }

    // Token Analytics & Charts
    let barChartInstance = null;
    let lineChartInstance = null;

    async function fetchTokenAnalytics() {
      const daysSelect = document.getElementById('analyticsDaysSelect');
      let period = 'week';
      if (daysSelect) {
        const days = parseInt(daysSelect.value) || 7;
        period = days === 1 ? 'day' : (days === 30 ? 'month' : 'week');
      }

      const modelSelect = document.getElementById('analyticsModelSelect');
      const selectedModel = modelSelect ? modelSelect.value : '';

      let url = `/api/analytics/tokens?period=${encodeURIComponent(period)}`;
      if (selectedModel) {
        url += `&model=${encodeURIComponent(selectedModel)}`;
      }

      try {
        const res = await fetch(url);
        const data = await res.json();

        // 1. Update KPI Cards
        const sum = data.summary || {};
        const reqEl = document.getElementById('statTotalRequests');
        if (reqEl) reqEl.textContent = (sum.totalRequests || 0).toLocaleString();

        const savTokensEl = document.getElementById('statTotalTokensSaved');
        if (savTokensEl) savTokensEl.textContent = (sum.totalSavedOutput || 0).toLocaleString();

        const savPctEl = document.getElementById('statSavedEfficiency');
        if (savPctEl) savPctEl.textContent = (sum.overallSavedPercentage || 0).toFixed(1) + '%';

        const actEl = document.getElementById('statActualOutputTokens');
        if (actEl) actEl.textContent = (sum.totalActualOutput || 0).toLocaleString();

        const rawEl = document.getElementById('statRawOutputTokens');
        if (rawEl) rawEl.textContent = (sum.totalEstimatedRaw || 0).toLocaleString();

        const cachedEl = document.getElementById('statCachedTokens');
        if (cachedEl) cachedEl.textContent = (sum.totalCachedTokens || 0).toLocaleString();

        const inputEl = document.getElementById('statTotalInputTokens');
        if (inputEl) inputEl.textContent = 'Total input: ' + (sum.totalInputTokens || 0).toLocaleString();

        // Populate Model Select Options dynamically if empty or only 1 option
        if (modelSelect) {
          const currentVal = modelSelect.value;
          const knownModels = [
            'gemini-3.8-flash-tiered',
            'claude-sonnet-4-6',
            'claude-opus-4-6-thinking',
            'gpt-oss-120b-medium'
          ];
          const modelsSet = new Set(knownModels);
          if (data.byModel && Array.isArray(data.byModel)) {
            data.byModel.forEach(m => {
              if (m.model) modelsSet.add(m.model);
            });
          }
          if (modelSelect.options.length <= 1) {
            modelSelect.innerHTML = '<option value="">Semua Model</option>';
            modelsSet.forEach(m => {
              const opt = document.createElement('option');
              opt.value = m;
              opt.textContent = m;
              if (m === currentVal) opt.selected = true;
              modelSelect.appendChild(opt);
            });
          }
        }

        // 2. Render Charts
        renderAnalyticsCharts(data);

        // 3. Breakdown By Model Table
        const byModelBody = document.getElementById('analyticsModelBreakdownBody');
        if (byModelBody) {
          const byModelList = data.byModel || [];
          if (byModelList.length === 0) {
            byModelBody.innerHTML = '<tr><td colspan="7" class="text-center py-6 text-slate-400">Belum ada data untuk periode ini</td></tr>';
          } else {
            byModelBody.innerHTML = byModelList.map(m => `
              <tr class="border-b border-slate-100 hover:bg-slate-50/60 transition-colors">
                <td class="p-2.5 font-bold font-mono text-slate-900">${escapeHtml(m.model)}</td>
                <td class="p-2.5 font-semibold text-slate-800">${m.requests}</td>
                <td class="p-2.5 font-mono font-semibold text-purple-600">${m.inputTokens.toLocaleString()}</td>
                <td class="p-2.5 font-mono font-semibold text-amber-500">${m.cachedTokens.toLocaleString()}</td>
                <td class="p-2.5 font-mono font-bold text-blue-600">${m.actualOutput.toLocaleString()}</td>
                <td class="p-2.5 font-mono font-medium text-slate-400">${m.estimatedRaw.toLocaleString()}</td>
                <td class="p-2.5 font-mono font-bold text-emerald-600">+${m.savedOutput.toLocaleString()} (${(m.savedPercentage || 0).toFixed(0)}%)</td>
              </tr>
            `).join('');
          }
        }

        // 4. Render Recent Transactions Table
        const recentBody = document.getElementById('analyticsRecentLogsBody');
        if (recentBody) {
          const logs = data.recentLogs || data.recentTransactions || [];
          if (logs.length === 0) {
            recentBody.innerHTML = '<tr><td colspan="9" class="p-4 text-center text-slate-400">Belum ada log transaksi</td></tr>';
          } else {
            recentBody.innerHTML = logs.map(l => {
              const skillsList = l.skillsUsed || l.skills || [];
              const skillsBadges = skillsList.map(s => `<span class="inline-flex items-center gap-1 text-[10px] font-semibold px-2 py-0.5 rounded-full bg-purple-50 text-purple-700 border border-purple-200/60">${escapeHtml(s)}</span>`).join('');
              return `
                <tr class="border-b border-slate-100 text-xs hover:bg-slate-50/60 transition-colors">
                  <td class="p-2 font-mono text-slate-400 text-[11px] whitespace-nowrap">${new Date(l.timestamp).toLocaleTimeString()}</td>
                  <td class="p-2 font-bold font-mono text-slate-800 text-[11px]">${escapeHtml(l.model)}</td>
                  <td class="p-2 text-slate-600 text-[11px]">${escapeHtml((l.account || '').split('@')[0])}</td>
                  <td class="p-2 font-mono font-semibold text-purple-600">${l.promptTokens.toLocaleString()}</td>
                  <td class="p-2 font-mono font-semibold text-amber-500">${l.cachedTokens.toLocaleString()}</td>
                  <td class="p-2 font-mono font-bold text-blue-600">${l.actualOutputTokens.toLocaleString()}</td>
                  <td class="p-2 font-mono font-medium text-slate-400">${l.estimatedRawTokens.toLocaleString()}</td>
                  <td class="p-2 font-mono font-bold text-emerald-600">+${l.savedOutputTokens.toLocaleString()} (${(l.savedPercentage || 0).toFixed(0)}%)</td>
                </tr>
              `;
            }).join('');
          }
        }
      } catch (err) {
        console.error('Failed to fetch token analytics:', err);
      }
    }
    function renderAnalyticsCharts(data) {
      const ts = data.timeSeries || [];
      const labels = ts.map(p => p.label);
      const rawOutputs = ts.map(p => p.estimatedRaw);
      const actualOutputs = ts.map(p => p.actualOutput);
      const savedOutputs = ts.map(p => p.savedOutput);
      const inputTokens = ts.map(p => p.inputTokens);
      const cachedTokens = ts.map(p => p.cachedTokens);
      const savedPcts = ts.map(p => p.savedPct);

      // 1. Diagram Batang (Bar Chart)
      const barCanvas = document.getElementById('tokenSaverComparisonChart') || document.getElementById('barChartCanvas');
      if (barCanvas && typeof Chart !== 'undefined') {
        if (barChartInstance) barChartInstance.destroy();
        barChartInstance = new Chart(barCanvas, {
          type: 'bar',
          data: {
            labels: labels,
            datasets: [
              {
                label: 'Output Sebenarnya (Tanpa Skills)',
                data: rawOutputs,
                backgroundColor: 'rgba(148, 163, 184, 0.75)',
                borderColor: '#94a3b8',
                borderWidth: 1,
                borderRadius: 4,
              },
              {
                label: 'Output Setelah Skills (Token Saver)',
                data: actualOutputs,
                backgroundColor: 'rgba(37, 99, 235, 0.85)',
                borderColor: '#2563eb',
                borderWidth: 1,
                borderRadius: 4,
              },
              {
                label: 'Token Berhasil Dihemat',
                data: savedOutputs,
                backgroundColor: 'rgba(16, 185, 129, 0.88)',
                borderColor: '#10b981',
                borderWidth: 1,
                borderRadius: 4,
              }
            ]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
              legend: { position: 'top', labels: { boxWidth: 12, font: { size: 11, family: 'Plus Jakarta Sans' } } },
              tooltip: { padding: 10, cornerRadius: 8 }
            },
            scales: {
              x: { grid: { display: false } },
              y: { beginAtZero: true, grid: { color: 'rgba(0,0,0,0.05)' } }
            }
          }
        });
      }

      // 2. Diagram Garis (Daily Trend Chart)
      const lineCanvas = document.getElementById('dailyTokenTrendChart') || document.getElementById('lineChartCanvas');
      if (lineCanvas && typeof Chart !== 'undefined') {
        if (lineChartInstance) lineChartInstance.destroy();
        lineChartInstance = new Chart(lineCanvas, {
          type: 'line',
          data: {
            labels: labels,
            datasets: [
              {
                label: 'Input Tokens',
                data: inputTokens,
                borderColor: '#8b5cf6',
                backgroundColor: 'rgba(139, 92, 246, 0.12)',
                tension: 0.35,
                fill: true,
                borderWidth: 2,
                pointRadius: 3,
                yAxisID: 'y'
              },
              {
                label: 'Cached Input Tokens',
                data: cachedTokens,
                borderColor: '#06b6d4',
                backgroundColor: 'transparent',
                borderDash: [4, 4],
                tension: 0.35,
                borderWidth: 2,
                pointRadius: 3,
                yAxisID: 'y'
              },
              {
                label: 'Output Riil (Tokens)',
                data: actualOutputs,
                borderColor: '#f59e0b',
                backgroundColor: 'transparent',
                tension: 0.35,
                borderWidth: 2,
                pointRadius: 3,
                yAxisID: 'y'
              },
              {
                label: 'Efisiensi Hemat (%)',
                data: savedPcts,
                borderColor: '#10b981',
                backgroundColor: 'transparent',
                borderDash: [2, 2],
                tension: 0.35,
                borderWidth: 2,
                pointRadius: 3,
                yAxisID: 'y1'
              }
            ]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            plugins: {
              legend: { position: 'top', labels: { boxWidth: 12, font: { size: 11, family: 'Plus Jakarta Sans' } } },
              tooltip: { padding: 10, cornerRadius: 8 }
            },
            scales: {
              x: { grid: { display: false } },
              y: {
                type: 'linear',
                display: true,
                position: 'left',
                beginAtZero: true,
                grid: { color: 'rgba(0,0,0,0.05)' },
                title: { display: true, text: 'Tokens', font: { size: 10 } }
              },
              y1: {
                type: 'linear',
                display: true,
                position: 'right',
                min: 0,
                max: 100,
                grid: { drawOnChartArea: false },
                ticks: { callback: v => v + '%' },
                title: { display: true, text: 'Efisiensi %', font: { size: 10 } }
              }
            }
          }
        });
      }
    }

    // Auto-refresh Usage Tab State (30 seconds interval)
    let usageRefreshSeconds = 30;
    let isUsageRefreshPaused = false;
    let usageRefreshTimer = null;

    function initUsageAutoRefresh() {
      if (usageRefreshTimer) clearInterval(usageRefreshTimer);
      usageRefreshTimer = setInterval(() => {
        if (isUsageRefreshPaused) return;
        if (currentTab !== 'usage' && currentTab !== 'dashboard') return;

        usageRefreshSeconds--;
        const badge = document.getElementById('usageCountdownBadge');
        if (badge) {
          badge.innerHTML = svgIcon('refresh-cw', 'icon-sm') + ` Auto-refresh: ${usageRefreshSeconds}s`;
        }

        if (usageRefreshSeconds <= 0) {
          usageRefreshSeconds = 30;
          fetchAccounts();
        }
      }, 1000);
    }

    function toggleUsageAutoRefresh() {
      isUsageRefreshPaused = !isUsageRefreshPaused;
      const btn = document.getElementById('usagePauseBtn');
      const badge = document.getElementById('usageCountdownBadge');
      if (btn) btn.textContent = isUsageRefreshPaused ? 'Resume' : 'Pause';
      if (badge) {
        if (isUsageRefreshPaused) {
          badge.innerHTML = svgIcon('pause', 'icon-sm') + ' Auto-refresh: Dijeda';
          badge.classList.remove('text-blue-600');
          badge.classList.add('text-amber-500');
        } else {
          badge.innerHTML = svgIcon('refresh-cw', 'icon-sm') + ` Auto-refresh: ${usageRefreshSeconds}s`;
          badge.classList.remove('text-amber-500');
          badge.classList.add('text-blue-600');
        }
      }
    }

    function manualRefreshUsage() {
      usageRefreshSeconds = 30;
      const badge = document.getElementById('usageCountdownBadge');
      if (badge && !isUsageRefreshPaused) {
        badge.innerHTML = svgIcon('refresh-cw') + ' Auto-refresh: 30s';
      }
      fetchAccounts();
    }

    // ==================== MODEL FALLBACK MANAGEMENT ====================
    let fallbackConfig = { enabled: true, rules: {} };
    let fallbackAvailableModels = [];

    async function fetchFallbacks() {
      try {
        const [res, modelsRes] = await Promise.all([
          fetch('/api/fallbacks'),
          fetch('/v1/models').catch(() => null)
        ]);
        const data = await res.json();
        fallbackConfig = {
          enabled: data.enabled !== false,
          rules: data.rules || {}
        };
        const backendModels = data.availableModels || [];
        const setModels = new Set(backendModels);

        if (modelsRes && modelsRes.ok) {
          try {
            const mData = await modelsRes.json();
            const list = mData.data || mData.models || [];
            list.forEach(item => {
              if (item.id) setModels.add(item.id);
            });
          } catch (e) {}
        }
        fallbackAvailableModels = Array.from(setModels);
        renderFallbacksUI();
      } catch (err) {
        console.error('Failed to fetch fallbacks:', err);
      }
    }

    function renderFallbacksUI() {
      const toggle = document.getElementById('fallbackMasterToggle');
      if (toggle) toggle.checked = fallbackConfig.enabled;

      const badge = document.getElementById('fallbackStatusBadge');
      if (badge) {
        if (fallbackConfig.enabled) {
          badge.className = 'badge badge-success';
          badge.textContent = 'Chain Aktif';
        } else {
          badge.className = 'badge badge-danger';
          badge.textContent = 'Nonaktif';
        }
      }

      // Populate fallback model datalist
      const datalist = document.getElementById('fallbackModelOptions');
      if (datalist && fallbackAvailableModels && fallbackAvailableModels.length > 0) {
        datalist.innerHTML = fallbackAvailableModels.map(m => `<option value="${m}">${m}</option>`).join('');
      }

      const container = document.getElementById('fallbackChainsContainer');
      if (!container) return;

      const ruleKeys = Object.keys(fallbackConfig.rules || {});
      if (ruleKeys.length === 0) {
        container.innerHTML = `
          <div class="bg-white/80 p-8 rounded-2xl text-center border border-dashed border-slate-300 text-slate-500">
            Belum ada aturan model fallback. Tambahkan aturan baru di atas atau klik tombol <strong>Preset Cepat</strong>.
          </div>
        `;
        return;
      }

      container.innerHTML = ruleKeys.map(sourceModel => {
        const targets = fallbackConfig.rules[sourceModel] || [];
        const isDefault = sourceModel === 'default';

        return `
          <div class="item-card p-5 rounded-2xl bg-white/90">
            <div class="flex justify-between items-center flex-wrap gap-2.5 mb-3">
              <div class="flex items-center gap-2">
                <span class="text-base inline-flex items-center text-indigo-600">${isDefault ? svgIcon('globe') : svgIcon('cpu')}</span>
                <div>
                  <div class="font-extrabold text-sm text-slate-900">${isDefault ? 'Default Global Fallback' : escapeHtml(sourceModel)}</div>
                  <div class="text-xs text-slate-500">
                    ${isDefault ? 'Berlaku jika model yang diminta tidak memiliki aturan spesifik.' : `Dipicu jika request ke <code>${escapeHtml(sourceModel)}</code> gagal / 429.`}
                  </div>
                </div>
              </div>
              <div class="flex gap-1.5 items-center">
                <button class="btn btn-sm btn-danger px-2.5 py-1 text-xs" onclick="deleteFallbackRule('${escapeHtml(sourceModel)}')">${svgIcon('trash-2', 'icon-sm')} Hapus Rantai</button>
              </div>
            </div>

            <!-- Chain Nodes Flow -->
            <div class="flex items-center flex-wrap gap-2 p-3 bg-slate-50 rounded-xl border border-slate-200">
              <!-- Source Node -->
              <div class="bg-blue-50 text-blue-800 border border-blue-200 font-bold px-3 py-1.5 rounded-lg text-xs flex items-center gap-1.5">
                ${svgIcon('alert-circle', 'icon-sm')}
                <span>1. ${escapeHtml(sourceModel)}</span>
              </div>

              ${targets.map((tgt, idx) => `
                <span class="inline-flex items-center text-slate-400">${svgIcon('arrow-right', 'icon-sm')}</span>
                <div class="bg-white text-slate-800 border border-slate-200 shadow-2xs font-semibold px-3 py-1.5 rounded-lg text-xs flex items-center gap-2">
                  <span>${idx + 2}. ${escapeHtml(tgt)}</span>
                  <div class="flex items-center gap-1 pl-1 border-l border-slate-200">
                    <button class="btn btn-sm px-1.5 py-0.5 text-[11px]" title="Pindahkan ke depan" ${idx === 0 ? 'disabled' : ''} onclick="moveTargetInChain('${escapeHtml(sourceModel)}', ${idx}, -1)">&larr;</button>
                    <button class="btn btn-sm px-1.5 py-0.5 text-[11px]" title="Pindahkan ke belakang" ${idx === targets.length - 1 ? 'disabled' : ''} onclick="moveTargetInChain('${escapeHtml(sourceModel)}', ${idx}, 1)">&rarr;</button>
                    <button class="text-rose-500 hover:text-rose-700 ml-1 text-xs" title="Hapus step ini" onclick="removeTargetFromChain('${escapeHtml(sourceModel)}', ${idx})">${svgIcon('x')}</button>
                  </div>
                </div>
              `).join('')}

              <span class="inline-flex items-center text-slate-400">${svgIcon('arrow-right', 'icon-sm')}</span>
              <!-- Add next step in chain -->
              <div class="flex items-center gap-1.5">
                <input type="text" id="addTgtInput_${escapeHtml(sourceModel)}" list="fallbackModelOptions" placeholder="+ model cadangan" class="px-2.5 py-1 text-xs rounded-lg border border-slate-300 bg-white w-40 focus:ring-1 focus:ring-blue-500/20">
                <button class="btn btn-sm btn-primary px-2.5 py-1 text-xs shadow-xs" onclick="addTargetToChain('${escapeHtml(sourceModel)}')">+ Tambah</button>
              </div>
            </div>
          </div>
        `;
      }).join('');
    }
    function toggleFallbackEnabled() {
      const toggle = document.getElementById('fallbackMasterToggle');
      if (toggle) {
        fallbackConfig.enabled = toggle.checked;
        renderFallbacksUI();
      }
    }

    function addNewFallbackRule() {
      const srcInput = document.getElementById('newRuleSourceModel');
      const tgtInput = document.getElementById('newRuleTargetModels') || document.getElementById('newRuleTargetModel');
      const src = (srcInput ? srcInput.value : '').trim();
      const rawTgt = (tgtInput ? tgtInput.value : '').trim();

      if (!src) {
        alert('Silakan tentukan model utama (source model)');
        return;
      }
      if (!fallbackConfig.rules) fallbackConfig.rules = {};
      if (!fallbackConfig.rules[src]) fallbackConfig.rules[src] = [];

      if (rawTgt) {
        const tgts = rawTgt.split(',').map(s => s.trim()).filter(Boolean);
        tgts.forEach(t => {
          if (!fallbackConfig.rules[src].includes(t)) {
            fallbackConfig.rules[src].push(t);
          }
        });
      }

      if (srcInput) srcInput.value = '';
      if (tgtInput) tgtInput.value = '';
      renderFallbacksUI();
      saveAllFallbacks();
    }

    function deleteFallbackRule(sourceModel) {
      if (confirm(`Yakin ingin menghapus seluruh aturan rantai fallback untuk model "${sourceModel}"?`)) {
        delete fallbackConfig.rules[sourceModel];
        renderFallbacksUI();
        saveAllFallbacks();
      }
    }

    function addTargetToChain(sourceModel) {
      const input = document.getElementById(`addTgtInput_${sourceModel}`);
      const val = (input ? input.value : '').trim();
      if (!val) return;

      if (!fallbackConfig.rules[sourceModel]) {
        fallbackConfig.rules[sourceModel] = [];
      }
      if (!fallbackConfig.rules[sourceModel].includes(val)) {
        fallbackConfig.rules[sourceModel].push(val);
      }
      if (input) input.value = '';
      renderFallbacksUI();
      saveAllFallbacks();
    }

    function removeTargetFromChain(sourceModel, idx) {
      if (fallbackConfig.rules[sourceModel]) {
        fallbackConfig.rules[sourceModel].splice(idx, 1);
        renderFallbacksUI();
        saveAllFallbacks();
      }
    }

    function moveTargetInChain(sourceModel, idx, dir) {
      const arr = fallbackConfig.rules[sourceModel];
      if (!arr) return;
      const newIdx = idx + dir;
      if (newIdx < 0 || newIdx >= arr.length) return;
      const tmp = arr[idx];
      arr[idx] = arr[newIdx];
      arr[newIdx] = tmp;
      renderFallbacksUI();
    }

    function applyFallbackPreset(preset) {
      if (preset === 'safe-flash') {
        fallbackConfig.rules = {
          "gemini-3.8-flash-tiered": ["claude-sonnet-4-6", "gpt-oss-120b-medium"],
          "default": ["gemini-3.8-flash-tiered", "claude-sonnet-4-6"]
        };
        fallbackConfig.enabled = true;
        alert('Preset Aman (Gemini 3.8 Flash Tiered, Sonnet & OSS) telah dimuat. Klik "Simpan Semua Pengaturan" untuk menyimpan permanen.');
      } else if (preset === 'claude-flash') {
        fallbackConfig.rules = {
          "claude-opus-4-6-thinking": ["claude-sonnet-4-6", "gemini-3.8-flash-tiered"],
          "claude-sonnet-4-6": ["claude-opus-4-6-thinking", "gemini-3.8-flash-tiered"],
          "gpt-oss-120b-medium": ["gemini-3.8-flash-tiered", "claude-sonnet-4-6"],
          "gemini-3.8-flash-tiered": ["claude-sonnet-4-6", "gpt-oss-120b-medium"],
          "default": ["gemini-3.8-flash-tiered", "claude-sonnet-4-6"]
        };
        fallbackConfig.enabled = true;
        alert('Preset Claude, Gemini & OSS telah dimuat. Klik "Simpan Semua Pengaturan" untuk menyimpan permanen.');
      } else if (preset === 'reset-default') {
        resetDefaultFallbacks();
      }
      renderFallbacksUI();
    }

    function resetDefaultFallbacks() {
      showDestructiveConfirm({
        title: 'Reset Fallback Rules',
        subtitle: 'Kembalikan ke Konfigurasi Default',
        desc: 'Apakah Anda yakin ingin mereset rantai fallback ke daftar model aktif bawaan (Gemini 3.8 Flash Tiered, Claude Opus Thinking, Claude Sonnet, dan GPT-OSS 120B)?',
        verifyText: '',
        onConfirm: async () => {
          fallbackConfig.rules = {
            "claude-opus-4-6-thinking": ["claude-sonnet-4-6", "gemini-3.8-flash-tiered"],
            "claude-sonnet-4-6": ["claude-opus-4-6-thinking", "gemini-3.8-flash-tiered"],
            "gpt-oss-120b-medium": ["gemini-3.8-flash-tiered", "claude-sonnet-4-6"],
            "gemini-3.8-flash-tiered": ["claude-sonnet-4-6", "gpt-oss-120b-medium"],
            "default": ["gemini-3.8-flash-tiered", "claude-sonnet-4-6"]
          };
          fallbackConfig.enabled = true;
          renderFallbacksUI();
        }
      });
    }

    async function saveAllFallbacks(silent = false) {
      try {
        const res = await fetch('/api/fallbacks', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            enabled: fallbackConfig.enabled,
            rules: fallbackConfig.rules
          })
        });
        if (!res.ok) {
          const txt = await res.text();
          throw new Error(txt);
        }
        if (!silent) {
          alert('Pengaturan Model Fallback Chain berhasil disimpan secara permanen!');
        }
        fetchFallbacks();
      } catch (err) {
        alert('Gagal menyimpan aturan fallback: ' + err.message);
      }
    }

    // Initialize
    const savedWebhookUrl = localStorage.getItem('ag_webhook_url');
    if (savedWebhookUrl) {
      const urlInput = document.getElementById('webhookTestUrl');
      if (urlInput) urlInput.value = savedWebhookUrl;
    }
    // URL Hash Routing initialization & back/forward navigation support
    function handleHashChange() {
      const hash = (window.location.hash || '').replace(/^#\/?/, '').toLowerCase();
      const validTabs = ['dashboard', 'analytics', 'usage', 'provider', 'fallbacks', 'inspector', 'playground', 'apikeys', 'skills', 'integrations'];
      if (hash && validTabs.includes(hash)) {
        switchTab(hash, false);
      } else {
        switchTab('dashboard', false);
      }
    }
    window.addEventListener('hashchange', handleHashChange);

    // Window message, BroadcastChannel, and Storage listeners for OAuth completion
    function handleOAuthSuccessNotification() {
      alert('Autentikasi Google Berhasil! Akun baru telah terhubung.');
      fetchAccounts();
      switchTab('provider');
      setTimeout(fetchAccounts, 1200);
      setTimeout(fetchAccounts, 3000);
    }

    window.addEventListener('message', (event) => {
      if (event.data && event.data.type === 'GOOGLE_AUTH_SUCCESS') {
        handleOAuthSuccessNotification();
      }
    });

    window.addEventListener('storage', (event) => {
      if (event.key === 'ag_auth_event') {
        try {
          const data = JSON.parse(event.newValue);
          if (data && data.type === 'GOOGLE_AUTH_SUCCESS') {
            handleOAuthSuccessNotification();
          }
        } catch (e) {}
      }
    });

    try {
      if (window.BroadcastChannel) {
        const authBC = new BroadcastChannel('ag_auth_channel');
        authBC.onmessage = (event) => {
          if (event.data && event.data.type === 'GOOGLE_AUTH_SUCCESS') {
            handleOAuthSuccessNotification();
          }
        };
      }
    } catch (e) {}

    // Initial boot on DOM readiness to guarantee all partial containers exist
    async function bootApplication() {
      // 1. Check for OAuth callback status (?auth=success or ?auth=failed)
      const urlParams = new URLSearchParams(window.location.search);
      if (urlParams.get('auth') === 'success') {
        try {
          if (window.opener) {
            window.opener.postMessage({ type: 'GOOGLE_AUTH_SUCCESS' }, '*');
          }
        } catch (e) {}
        try {
          if (window.BroadcastChannel) {
            const authBC = new BroadcastChannel('ag_auth_channel');
            authBC.postMessage({ type: 'GOOGLE_AUTH_SUCCESS' });
          }
        } catch (e) {}
        try {
          localStorage.setItem('ag_auth_event', JSON.stringify({ type: 'GOOGLE_AUTH_SUCCESS', timestamp: Date.now() }));
        } catch (e) {}

        if (window.opener || window.name === 'googleLoginPopup') {
          try {
            window.close();
            return;
          } catch (e) {}
        }

        window.history.replaceState({}, document.title, window.location.pathname + '#provider');
        setTimeout(() => {
          alert('Autentikasi Google Berhasil! Akun baru telah terhubung.');
          switchTab('provider', false);
        }, 100);
      } else if (urlParams.get('auth') === 'failed') {
        if (window.opener || window.name === 'googleLoginPopup') {
          try {
            window.close();
            return;
          } catch (e) {}
        }
        window.history.replaceState({}, document.title, window.location.pathname + '#provider');
        setTimeout(() => {
          alert('Autentikasi Google Gagal! Silakan coba lagi.');
        }, 100);
      }

      // 2. Fetch initial background data so all tabs have fresh context
      await fetchAccounts();
      fetchApiKeys();
      fetchSkills();
      fetchInspectorLogs();
      initUsageAutoRefresh();
      initInspectorAutoRefresh();
      fetchFallbacks();

      // 3. Resolve initial active tab from URL hash
      handleHashChange();
    }

    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', bootApplication);
    } else {
      bootApplication();
    }

// Explicit global window bindings for inline HTML onclick handlers
window.addNewFallbackRule = addNewFallbackRule;
window.addTargetToChain = addTargetToChain;
window.applyFallbackPreset = applyFallbackPreset;
window.autoSelectGrillRecommendations = autoSelectGrillRecommendations;
window.changeInspectorPageSize = changeInspectorPageSize;
window.clearAccountCooldown = clearAccountCooldown;
window.clearComboSkills = clearComboSkills;
window.clearInspectorLogs = clearInspectorLogs;
window.clearPlayground = clearPlayground;
window.closeConfirmModal = closeConfirmModal;
window.closeCreateSkillModal = closeCreateSkillModal;
window.copyApiKey = copyApiKey;
window.copyInspectorCurl = copyInspectorCurl;
window.copyMessageContent = copyMessageContent;
window.deleteAccount = deleteAccount;
window.deleteApiKey = deleteApiKey;
window.deleteFallbackRule = deleteFallbackRule;
window.deleteSkillPrompt = deleteSkillPrompt;
window.ensureTabLoaded = ensureTabLoaded;
window.escapeHtml = escapeHtml;
window.exportMessageToMarkdown = exportMessageToMarkdown;
window.fetchAccounts = fetchAccounts;
window.fetchApiKeys = fetchApiKeys;
window.fetchFallbacks = fetchFallbacks;
window.fetchInspectorLogs = fetchInspectorLogs;
window.fetchProviderStrategy = fetchProviderStrategy;
window.fetchSkills = fetchSkills;
window.fetchTokenAnalytics = fetchTokenAnalytics;
window.formatChatContent = formatChatContent;
window.formatDateTime = formatDateTime;
window.handleCreateSkillSubmit = handleCreateSkillSubmit;
window.handleGrillCustomInput = handleGrillCustomInput;
window.handleGrillOptionChange = handleGrillOptionChange;
window.handlePlaygroundAction = handlePlaygroundAction;
window.importFromLocalSystem = importFromLocalSystem;
window.importFromOmniRoute = importFromOmniRoute;
window.initInspectorAutoRefresh = initInspectorAutoRefresh;
window.initUsageAutoRefresh = initUsageAutoRefresh;
window.loadAnalyticsPartial = loadAnalyticsPartial;
window.loginGoogle = loginGoogle;
window.manualRefreshUsage = manualRefreshUsage;
window.moveAccountOrder = moveAccountOrder;
window.moveTargetInChain = moveTargetInChain;
window.nextInspectorPage = nextInspectorPage;
window.openCreateKeyModal = openCreateKeyModal;
window.openCreateSkillModal = openCreateSkillModal;
window.prevInspectorPage = prevInspectorPage;
window.refreshAccountQuota = refreshAccountQuota;
window.refreshAllAccounts = refreshAllAccounts;
window.removeTargetFromChain = removeTargetFromChain;
window.renderAnalyticsCharts = renderAnalyticsCharts;
window.renderComboSkillsPills = renderComboSkillsPills;
window.renderFallbacksUI = renderFallbacksUI;
window.renderInspectorTable = renderInspectorTable;
window.renderInteractiveGrillMe = renderInteractiveGrillMe;
window.renderPlaygroundMessages = renderPlaygroundMessages;
window.renderProvider = renderProvider;
window.renderUsage = renderUsage;
window.renderWeeklyModelLimitBox = renderWeeklyModelLimitBox;
window.resetBuiltinSkills = resetBuiltinSkills;
window.resetDefaultFallbacks = resetDefaultFallbacks;
window.saveAllFallbacks = saveAllFallbacks;
window.saveProviderStrategy = saveProviderStrategy;
window.selectAllComboSkills = selectAllComboSkills;
window.sendPlaygroundMessage = sendPlaygroundMessage;
window.sendTestWebhook = sendTestWebhook;
window.showDestructiveConfirm = showDestructiveConfirm;
window.submitGrillAnswers = submitGrillAnswers;
window.svgIcon = svgIcon;
window.switchTab = switchTab;
window.toggleAccount = toggleAccount;
window.toggleComboSkill = toggleComboSkill;
window.toggleFallbackEnabled = toggleFallbackEnabled;
window.toggleKeyVisibility = toggleKeyVisibility;
window.toggleMobileNav = toggleMobileNav;
window.toggleProviderAccordion = toggleProviderAccordion;
window.toggleSkill = toggleSkill;
window.toggleUsageAutoRefresh = toggleUsageAutoRefresh;
window.updatePgAccountBadge = updatePgAccountBadge;
window.updatePgAccountsDropdown = updatePgAccountsDropdown;
window.updatePlaygroundButtonState = updatePlaygroundButtonState;
