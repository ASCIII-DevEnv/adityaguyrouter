let currentTab = 'dashboard';
    let accountsData = [];
    let keysData = [];
    let skillsData = [];
    let projectsData = [];
    let playgroundHistory = [];
    const PG_SESSIONS_KEY = 'antigravity_pg_sessions_v2';
    const PG_ACTIVE_SESSION_KEY = 'antigravity_pg_active_session_id_v2';
    let playgroundSessions = [];
    let currentPlaygroundSessionId = null;
    let isProviderCollapsed = false;
    let currentProviderStrategy = 'smart_quota';
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
      if (tabId === 'playground') {
        setTimeout(updatePlaygroundProjectsSelect, 100);
      }
      // Auto close mobile collapsible nav on item click
      const mobileNav = document.getElementById('mainNavTabs');
      const mobileLabel = document.getElementById('mobileNavLabel');
      if (mobileNav && mobileNav.classList.contains('open')) {
        mobileNav.classList.remove('open');
        if (mobileLabel) mobileLabel.textContent = 'Menu';
      }
      // Compatibility redirection: jika tab apikeys atau integrations dipanggil, redirect ke seksi Home
      if (tabId === 'apikeys' || tabId === 'integrations') {
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

      if (tabId === 'usage') {
        tabId = 'analytics';
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
        fetchProviderDiagnostics();
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
        await fetchProviderStrategy();
        if (accountsData.length === 0) {
          await fetchAccounts();
        } else {
          renderProvider();
          fetchAccounts();
        }
        fetchProviderDiagnostics();
      }
      if (tabId === 'fallbacks') fetchFallbacks();
      if (tabId === 'inspector') fetchInspectorLogs();
      if (tabId === 'playground') {
        loadPlaygroundSessions();
        syncPlaygroundHistoryWithSession();
        updatePgAccountsDropdown();
        updatePlaygroundProjectsSelect();
        renderComboSkillsPills();
        fetchAccounts();
        fetchSkills();
        fetchProjects();
        renderPlaygroundSessionsList();
        renderPlaygroundMessages();
        updateActiveSessionHeader();
      }
      if (tabId === 'analytics') {
        loadAnalyticsPartial();
      }
      if (tabId === 'diagnostics') {
        fetchDiagnostics();
      }
      if (tabId === 'skills') {
        fetchSkillsManager();
      }
    }

    // Fetch & render diagnostics embedded inside Provider tab
    async function fetchProviderDiagnostics(runProbe = false) {
      const memEl = document.getElementById('providerDiagMem');
      const listEl = document.getElementById('providerDiagLatencyList');

      if (runProbe) {
        document.querySelectorAll('[data-account-rtt]').forEach(el => {
          el.innerHTML = '<span class="inline-flex items-center gap-1 text-slate-400 font-normal"><svg class="w-3 h-3 animate-spin"><use href="#icon-refresh-cw"></use></svg> Probing...</span>';
          el.className = 'font-mono font-bold text-[11px] px-1.5 py-0.5 rounded border text-slate-400 bg-slate-50 border-slate-200/60';
        });
      }

      try {
        const res = await fetch('/api/diagnostics');
        const data = await res.json();
        const gw = data.gateway || {};
        if (memEl) memEl.textContent = `Heap: ${(gw.memoryAllocMB ? gw.memoryAllocMB.toFixed(1) : '0')} MB (${gw.goroutines || 0} go)`;

        // Update Home Bento Metric Card: Memori Heap
        const homeMemEl = document.getElementById('homeDiagMem');
        if (homeMemEl) homeMemEl.textContent = `${(gw.memoryAllocMB ? gw.memoryAllocMB.toFixed(1) : '0')} MB`;
        const homeGoEl = document.getElementById('homeDiagGoroutines');
        if (homeGoEl) homeGoEl.textContent = `${gw.goroutines || 0} go`;
        const homeDescEl = document.getElementById('homeDiagMemDesc');
        if (homeDescEl) {
          const sysMB = gw.memorySysMB ? gw.memorySysMB.toFixed(1) : '0';
          homeDescEl.textContent = `Sys: ${sysMB} MB • GC: ${gw.gcCycles || 0}x`;
        }

        const accounts = data.accounts || [];
        window.providerDiagnosticsMap = window.providerDiagnosticsMap || {};
        accounts.forEach(a => {
          window.providerDiagnosticsMap[a.email] = a;
        });

        // Update all account card RTT badges in Provider tab
        document.querySelectorAll('[data-account-rtt]').forEach(el => {
          const email = el.getAttribute('data-account-rtt');
          const a = window.providerDiagnosticsMap[email];
          if (a) {
            let colorClass = 'text-emerald-700 bg-emerald-50 border-emerald-200/60';
            if (a.latencyMs <= 0 || a.latencyMs > 750) colorClass = 'text-rose-700 bg-rose-50 border-rose-200/60';
            else if (a.latencyMs > 350) colorClass = 'text-amber-700 bg-amber-50 border-amber-200/60';

            el.className = `font-mono font-bold text-[11px] px-1.5 py-0.5 rounded border ${colorClass}`;
            el.textContent = a.latencyMs > 0 ? `${a.latencyMs} ms` : 'Timeout / N/A';
            el.title = `Status: ${a.status || 'Active'} | Diperiksa: ${a.lastChecked || '-'}`;
          }
        });

        if (!listEl) return;

        if (accounts.length === 0) {
          listEl.innerHTML = '<div class="col-span-full text-slate-400">Tidak ada data probe.</div>';
          return;
        }

        listEl.innerHTML = accounts.map(a => {
          let colorClass = 'text-emerald-700 bg-emerald-50 border-emerald-200/60';
          if (a.latencyMs <= 0 || a.latencyMs > 750) colorClass = 'text-rose-700 bg-rose-50 border-rose-200/60';
          else if (a.latencyMs > 350) colorClass = 'text-amber-700 bg-amber-50 border-amber-200/60';

          return `
            <div class="flex items-center justify-between p-2 rounded-lg bg-white border border-slate-200/60 shadow-2xs">
              <span class="font-semibold text-slate-700 truncate max-w-[110px]" title="${escapeHtml(a.email)}">${escapeHtml(a.email.split('@')[0])}</span>
              <span class="font-mono font-bold text-[11px] px-1.5 py-0.5 rounded border ${colorClass}">${a.latencyMs > 0 ? a.latencyMs + ' ms' : 'Timeout'}</span>
            </div>
          `;
        }).join('');
      } catch (err) {
        console.error('fetchProviderDiagnostics error:', err);
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

    function formatResetCountdown(isoStr) {
      if (!isoStr || isoStr.startsWith('0001')) return '<span class="text-slate-400">Penuh (100%)</span>';
      try {
        const d = new Date(isoStr);
        if (isNaN(d.getTime())) return '<span class="text-slate-400">-</span>';
        const now = new Date();
        const diffMs = d.getTime() - now.getTime();
        const timeStr = d.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' }) + ' WIB';
        if (diffMs <= 0) return `${timeStr} <span class="text-emerald-600 font-semibold">(segera)</span>`;
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
        return `${timeStr} <span class="text-sky-600 font-medium">(${relStr})</span>`;
      } catch (e) {
        return escapeHtml(isoStr);
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
                <svg class="w-3.5 h-3.5"><use href="#icon-google"></use></svg> Login Akun Google Sekarang
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

        const gemini5h = (gemini?.limit5h?.displayName && gemini.limit5h.remainingPercent !== undefined)
          ? gemini.limit5h.remainingPercent
          : (acc.quota5hPercent ?? 100);
        const geminiWk = (gemini?.limitWeekly?.displayName && gemini.limitWeekly.remainingPercent !== undefined)
          ? gemini.limitWeekly.remainingPercent
          : (acc.quotaWeeklyPercent ?? 100);
        const claude5h = (claude?.limit5h?.displayName && claude.limit5h.remainingPercent !== undefined)
          ? claude.limit5h.remainingPercent
          : (acc.quota5hPercent ?? 100);
        const claudeWk = (claude?.limitWeekly?.displayName && claude.limitWeekly.remainingPercent !== undefined)
          ? claude.limitWeekly.remainingPercent
          : (acc.quotaWeeklyPercent ?? 100);

        const gemini5hReset = gemini?.limit5h?.resetAt || acc.quota5hResetAt || '';
        const geminiWkReset = gemini?.limitWeekly?.resetAt || acc.quotaWeeklyResetAt || '';
        const claude5hReset = claude?.limit5h?.resetAt || acc.quota5hResetAt || '';
        const claudeWkReset = claude?.limitWeekly?.resetAt || acc.quotaWeeklyResetAt || '';

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
                  <div class="item-avatar bg-slate-900 border border-white/10 flex items-center justify-center ${!acc.isActive ? 'opacity-60 grayscale' : ''}">${acc.picture ? `<img src="${acc.picture}" referrerpolicy="no-referrer" alt="" onerror="this.classList.add('hidden'); this.parentElement.innerHTML='<svg class=\'w-5 h-5\'><use href=\'#icon-antigravity\'></use></svg>';" />` : `<svg class="w-5 h-5"><use href="#icon-antigravity"></use></svg>`}</div>
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
        const syncPromise = fetch('/api/accounts/sync', { method: 'POST' }).then(async res => {
          if (!res.ok) {
            await Promise.all((accountsData || []).map(a => 
              fetch('/api/accounts', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email: a.email, action: 'refresh' }),
              })
            ));
          }
        });
        const probePromise = fetchProviderDiagnostics(true);

        await Promise.allSettled([syncPromise, probePromise]);
        await fetchAccounts();
        if (currentTab === 'provider') {
          renderProvider();
          fetchProviderStrategy();
        } else if (currentTab === 'usage') {
          renderUsage();
        }
      } catch (err) {
        console.error('Gagal sinkronisasi semua kuota & probe:', err);
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
      const gemini5h = (gemini?.limit5h?.displayName && gemini.limit5h.remainingPercent !== undefined)
        ? Number(gemini.limit5h.remainingPercent)
        : (acc.quota5hPercent !== undefined ? Number(acc.quota5hPercent) : 100);
      const geminiWk = (gemini?.limitWeekly?.displayName && gemini.limitWeekly.remainingPercent !== undefined)
        ? Number(gemini.limitWeekly.remainingPercent)
        : (acc.quotaWeeklyPercent !== undefined ? Number(acc.quotaWeeklyPercent) : 100);
      const gemini5hReset = gemini?.limit5h?.resetAt || acc.quota5hResetAt || '';
      const geminiWkReset = gemini?.limitWeekly?.resetAt || acc.quotaWeeklyResetAt || '';

      // Extract Claude & GPT limits & reset
      const claude5h = (claude?.limit5h?.displayName && claude.limit5h.remainingPercent !== undefined)
        ? Number(claude.limit5h.remainingPercent)
        : (acc.quota5hPercent !== undefined ? Number(acc.quota5hPercent) : 100);
      const claudeWk = (claude?.limitWeekly?.displayName && claude.limitWeekly.remainingPercent !== undefined)
        ? Number(claude.limitWeekly.remainingPercent)
        : (acc.quotaWeeklyPercent !== undefined ? Number(acc.quotaWeeklyPercent) : 100);
      const claude5hReset = claude?.limit5h?.resetAt || acc.quota5hResetAt || '';
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
              <div class="flex justify-between text-[10px] text-slate-500 mt-1">
                <span>Reset 5-Jam:</span>
                <strong class="text-slate-600">${gemini5hReset ? formatResetCountdown(gemini5hReset) : (gemini5h >= 100 ? '<span class="text-emerald-600 font-medium">Penuh (100%)</span>' : '<span class="text-amber-500 font-medium">Sedang memuat...</span>')}</strong>
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
              <div class="flex justify-between text-[10px] text-slate-500 mt-1">
                <span>Reset Mingguan:</span>
                <strong class="text-slate-600">${geminiWkReset ? formatDateTime(geminiWkReset) : '<span class="text-amber-500 font-medium">Sedang memuat...</span>'}</strong>
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
              <div class="flex justify-between text-[10px] text-slate-500 mt-1">
                <span>Reset 5-Jam:</span>
                <strong class="text-slate-600">${claude5hReset ? formatResetCountdown(claude5hReset) : (claude5h >= 100 ? '<span class="text-emerald-600 font-medium">Penuh (100%)</span>' : '<span class="text-amber-500 font-medium">Sedang memuat...</span>')}</strong>
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
              <div class="flex justify-between text-[10px] text-slate-500 mt-1">
                <span>Reset Mingguan:</span>
                <strong class="text-slate-600">${claudeWkReset ? formatDateTime(claudeWkReset) : '<span class="text-amber-500 font-medium">Sedang memuat...</span>'}</strong>
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
                <svg class="w-3.5 h-3.5"><use href="#icon-google"></use></svg> Login Akun Google Sekarang
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
                  <span class="badge badge-primary text-[10px] font-bold px-2 py-0.5">Active Provider</span>
                </div>
                <div class="text-xs text-slate-500 mt-0.5">
                  Cloud Code Assist &amp; Gemini Code Companion &bull; ${totalAccs} Akun Google Terdaftar (${activeAccs} Aktif${inCooldownAccs > 0 ? ', ' + inCooldownAccs + ' Cooldown' : ''}) &bull; Metode: <span id="providerHeaderStrategyLabel" class="font-bold ${currentProviderStrategy === 'fill_first' ? 'text-purple-600' : 'text-blue-600'}">${strategyBadgeNames[currentProviderStrategy] || currentProviderStrategy}</span>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2.5 flex-wrap">
              <button class="btn btn-sm btn-outline inline-flex items-center gap-1.5 shadow-xs bg-white text-slate-700 hover:bg-slate-50" onclick="event.stopPropagation(); refreshAllAccounts(this)" title="Sinkronkan kuota semua akun Google Antigravity">
                ${svgIcon('refresh-cw', 'w-3.5 h-3.5')} Sync Semua Kuota
              </button>
              <button class="btn btn-sm btn-primary inline-flex items-center gap-1.5 shadow-xs" onclick="event.stopPropagation(); loginGoogle()">
                <svg class="w-3.5 h-3.5"><use href="#icon-google"></use></svg> Login Akun Google
              </button>
              <span id="providerAccordionArrow" class="text-xs font-bold text-slate-500 inline-flex items-center gap-1 cursor-pointer ml-1">
                ${isProviderCollapsed ? svgIcon('chevron-down') + ' Buka Detail' : svgIcon('chevron-up') + ' Tutup Detail'}
              </span>
            </div>
          </div>

          <div class="provider-body ${isProviderCollapsed ? 'collapsed' : ''}" id="providerAccordionBody">
            <!-- Provider Strategy Bar khusus Google Antigravity -->
            <div class="mb-4 bg-white border border-slate-200/80 rounded-xl p-3 shadow-xs flex flex-col md:flex-row md:items-center justify-between gap-3">
              <div class="flex items-start md:items-center gap-2.5">
                <div class="w-8 h-8 rounded-lg bg-purple-50 text-purple-600 flex items-center justify-center shrink-0 border border-purple-100">
                  <svg class="w-4 h-4"><use href="#icon-git-fork"></use></svg>
                </div>
                <div>
                  <div class="flex items-center gap-2">
                    <span class="text-xs font-bold uppercase tracking-wider text-slate-500">Metode Akun Google Antigravity</span>
                    <span id="providerStrategyBadge" class="badge badge-purple text-[10px] font-bold px-2 py-0.5">${strategyBadgeNames[currentProviderStrategy] || currentProviderStrategy}</span>
                  </div>
                  <p id="providerStrategyDesc" class="text-xs text-slate-600 mt-0.5 font-medium">
                    ${strategyDescriptions[currentProviderStrategy] || ''}
                  </p>
                </div>
              </div>
              <div class="flex items-center gap-2 shrink-0">
                <label for="providerStrategySelect" class="text-xs font-semibold text-slate-500 whitespace-nowrap">Pilih Metode:</label>
                <select id="providerStrategySelect" onchange="handleProviderStrategyChange(this.value)" class="text-xs bg-slate-50 border border-slate-300 rounded-lg px-2.5 py-1.5 font-bold text-slate-800 focus:outline-none focus:ring-2 focus:ring-purple-500 focus:bg-white cursor-pointer shadow-xs">
                  <option value="fill_first" ${currentProviderStrategy === 'fill_first' ? 'selected' : ''}>Fill First (Urutan Akun)</option>
                  <option value="smart_quota" ${currentProviderStrategy === 'smart_quota' ? 'selected' : ''}>Smart Quota (Default)</option>
                </select>
              </div>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              ${accountsData.map((acc, idx) => {
                const isCooldown = acc.cooldownUntil && new Date(acc.cooldownUntil) > new Date();
                const diag = (window.providerDiagnosticsMap && window.providerDiagnosticsMap[acc.email]) || null;
                let rttClass = 'text-slate-500 bg-slate-50 border-slate-200/60';
                let rttText = 'Memeriksa...';
                let rttTitle = 'Menunggu data probe...';
                if (diag) {
                  if (diag.latencyMs <= 0 || diag.latencyMs > 750) rttClass = 'text-rose-700 bg-rose-50 border-rose-200/60';
                  else if (diag.latencyMs > 350) rttClass = 'text-amber-700 bg-amber-50 border-amber-200/60';
                  else rttClass = 'text-emerald-700 bg-emerald-50 border-emerald-200/60';
                  rttText = diag.latencyMs > 0 ? `${diag.latencyMs} ms` : 'Timeout / N/A';
                  rttTitle = `Status: ${diag.status || 'Active'} | Diperiksa: ${diag.lastChecked || '-'}`;
                }
                return `
                  <div class="item-card ${isCooldown ? 'border-rose-400 bg-rose-50/30' : ''}">
                    <!-- Card Top Header -->
                    <div class="item-card-header">
                      <div class="flex items-center gap-2.5 min-w-0">
                        <div class="relative shrink-0">
                          <div class="item-avatar bg-slate-900 border border-white/10 flex items-center justify-center ${!acc.isActive ? 'opacity-60 grayscale' : ''}">${acc.picture ? `<img src="${acc.picture}" referrerpolicy="no-referrer" alt="" onerror="this.classList.add('hidden'); this.parentElement.textContent='${acc.email ? acc.email[0].toUpperCase() : 'G'}';" />` : (acc.email ? acc.email[0].toUpperCase() : 'G')}</div>
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
                          <div class="font-extrabold text-sm text-slate-900 truncate flex items-center gap-1.5 flex-wrap">
                            <span class="truncate">${escapeHtml(acc.name || acc.email)}</span>
                            ${acc.isPrimary ? '<span class="badge badge-primary text-[10px] px-1.5 py-0.5">Primary</span>' : ''}
                            ${currentProviderStrategy === 'fill_first' ? `<span class="badge badge-purple text-[10px] font-bold px-1.5 py-0.5">Prioritas #${idx + 1}</span>` : ''}
                          </div>
                          <div class="text-xs text-slate-500 truncate">${escapeHtml(acc.email)}</div>
                        </div>
                      </div>
                      <div class="flex items-center gap-1 shrink-0">
                        <button class="btn btn-sm px-2 py-0.5 text-xs" title="${currentProviderStrategy === 'fill_first' ? 'Prioritas lebih tinggi (dipakai duluan)' : 'Pindahkan ke atas'}" ${idx === 0 ? 'disabled' : ''} onclick="moveAccountOrder(${idx}, -1)">&uarr;</button>
                        <button class="btn btn-sm px-2 py-0.5 text-xs" title="${currentProviderStrategy === 'fill_first' ? 'Prioritas lebih rendah (dipakai belakangan)' : 'Pindahkan ke bawah'}" ${idx === accountsData.length - 1 ? 'disabled' : ''} onclick="moveAccountOrder(${idx}, 1)">&darr;</button>
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
                      <div class="flex justify-between items-center">
                        <span class="text-slate-500 inline-flex items-center gap-1">
                          ${svgIcon('cpu', 'w-3 h-3 text-blue-500')} Diagnosis RTT:
                        </span>
                        <span data-account-rtt="${escapeHtml(acc.email)}" class="font-mono font-bold text-[11px] px-1.5 py-0.5 rounded border ${rttClass}" title="${rttTitle}">
                          ${rttText}
                        </span>
                      </div>
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

    const strategyDescriptions = {
      'fill_first': 'Pakai akun urutan pertama sampai limit habis (0% / 429), lalu beralih ke akun berikutnya. Memaksimalkan prompt caching.',
      'smart_quota': 'Prioritaskan sisa kuota tertinggi & rotasi cerdas anti rate-limit (parkir pre-emptif akun <15%).',
    };

    const strategyBadgeNames = {
      'fill_first': 'Fill First (Urutan Akun)',
      'smart_quota': 'Smart Quota (Default)',
    };

    function updateStrategyUI() {
      const sel = document.getElementById('providerStrategySelect');
      if (sel) sel.value = currentProviderStrategy;

      const badge = document.getElementById('providerStrategyBadge');
      if (badge) {
        badge.textContent = strategyBadgeNames[currentProviderStrategy] || currentProviderStrategy;
        if (currentProviderStrategy === 'fill_first') {
          badge.className = 'badge badge-purple text-[10px] font-bold px-2 py-0.5';
        } else {
          badge.className = 'badge badge-primary text-[10px] font-bold px-2 py-0.5';
        }
      }

      const desc = document.getElementById('providerStrategyDesc');
      if (desc) {
        desc.textContent = strategyDescriptions[currentProviderStrategy] || '';
      }

      const headerLabel = document.getElementById('providerHeaderStrategyLabel');
      if (headerLabel) {
        headerLabel.textContent = strategyBadgeNames[currentProviderStrategy] || currentProviderStrategy;
        headerLabel.className = 'font-bold ' + (currentProviderStrategy === 'fill_first' ? 'text-purple-600' : 'text-blue-600');
      }
    }

    async function handleProviderStrategyChange(strategy) {
      currentProviderStrategy = strategy;
      updateStrategyUI();
      await saveProviderStrategy(strategy);
      renderProvider();
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
        if (data && data.strategy) {
          currentProviderStrategy = data.strategy;
        }
        updateStrategyUI();
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

    function formatDayMonthYear(dateInput) {
      if (!dateInput) return '-';
      const d = new Date(dateInput);
      if (isNaN(d.getTime())) return '-';
      return `${d.getDate()}/${d.getMonth() + 1}/${d.getFullYear()}`;
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
              <span>Dibuat: <strong class="text-slate-700">${formatDayMonthYear(k.createdAt)}</strong></span>
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

    async function fetchProjects() {
      try {
        const res = await fetch('/api/projects');
        const data = await res.json();
        projectsData = data.projects || [];
        updatePlaygroundProjectsSelect();
      } catch (err) {
        console.error('Failed to fetch projects:', err);
      }
    }

    let pgSelectedAccount = '';
    let pgSelectedModel = 'gemini-3.8-flash-tiered';
    let pgSelectedProject = '';

    function handlePgAccountChange(val) {
      pgSelectedAccount = val;
    }

    function handlePgModelChange(val) {
      pgSelectedModel = val;
    }

    function handlePgProjectChange(val) {
      pgSelectedProject = val;
    }

    function updatePlaygroundProjectsSelect() {
      // Re-render chat messages to update initial setup card if empty
      if (!playgroundHistory || playgroundHistory.length === 0) {
        renderPlaygroundMessages();
      }
    }

    function updatePgProjectBadge() {}

    function updatePgAccountsDropdown() {
      // Re-render chat messages to update initial setup card if empty
      if (!playgroundHistory || playgroundHistory.length === 0) {
        renderPlaygroundMessages();
      }
    }

    function updatePgAccountBadge() {}

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

        grid.innerHTML = skillsData.map(sk => {
          const estReduction = sk.id === 'caveman' ? '~80% Token Saver' :
                               sk.id === 'rtk' ? '~40% Token Saver' :
                               sk.id === 'codegraph' ? '~50% Token Saver' :
                               sk.id === 'context_mode' ? '~60% Token Saver' : '~70% Token Saver';
          return `
          <div class="item-card flex flex-col justify-between ${!sk.enabled ? 'opacity-75 bg-slate-50/70 border-dashed' : ''}">
            <div class="space-y-2.5">
              <div class="item-card-header items-center">
                <div class="font-extrabold text-sm text-slate-900 flex items-center gap-2">
                  <span>${escapeHtml(sk.name)}</span>
                </div>
                <div class="flex items-center gap-1.5 shrink-0">
                  <span class="badge ${sk.isBuiltin ? 'badge-purple' : 'badge-blue'} text-[10px]">${sk.isBuiltin ? 'Builtin' : 'Custom'}</span>
                  <span class="text-[10px] font-bold text-emerald-600 font-mono">${estReduction}</span>
                  <span class="badge ${sk.enabled ? 'badge-success' : 'badge-neutral opacity-80'} text-[10px] inline-flex items-center gap-1">
                    <span class="w-1.5 h-1.5 rounded-full ${sk.enabled ? 'bg-emerald-500' : 'bg-slate-400'}"></span>
                    ${sk.enabled ? 'Aktif' : 'Nonaktif'}
                  </span>
                </div>
              </div>
              <p class="text-xs text-slate-500 leading-relaxed">${escapeHtml(sk.description)}</p>
              
              <!-- Injeksi System Prompt Collapsible -->
              <details class="text-[11px] bg-slate-50 border border-slate-200/70 rounded-xl overflow-hidden mt-2" open>
                <summary class="px-3 py-1.5 font-bold text-slate-700 cursor-pointer select-none bg-slate-100/70 hover:bg-slate-100 transition-colors flex items-center justify-between">
                  <span class="flex items-center gap-1.5">
                    <svg class="w-3.5 h-3.5 text-purple-600"><use href="#icon-zap"></use></svg>
                    Injeksi System Prompt
                  </span>
                  <span class="text-[10px] text-purple-600 font-mono bg-purple-50 px-1.5 py-0.5 rounded border border-purple-200/60">Template Aktif</span>
                </summary>
                <pre class="bg-slate-900 text-slate-100 p-3 rounded-b-xl text-[10.5px] max-h-40 overflow-y-auto whitespace-pre-wrap break-words font-mono leading-relaxed"><code>${escapeHtml(sk.systemPrompt || '')}</code></pre>
              </details>
            </div>
            <div class="flex items-center justify-between pt-3 border-t border-slate-100 mt-3 gap-2">
              <button class="btn btn-xs ${sk.enabled ? 'btn-outline text-amber-700 hover:bg-amber-50' : 'btn-primary'} inline-flex items-center gap-1" onclick="toggleSkill('${escapeHtml(sk.id)}')">
                <svg class="w-3 h-3"><use href="${sk.enabled ? '#icon-ban' : '#icon-check'}"></use></svg>
                ${sk.enabled ? 'Nonaktifkan' : 'Aktifkan'}
              </button>
              ${!sk.isBuiltin ? `
                <button class="btn btn-xs btn-outline text-red-600 hover:bg-red-50 hover:text-red-700 inline-flex items-center gap-1" onclick="deleteSkillPrompt('${escapeHtml(sk.id)}', '${escapeHtml(sk.name)}', ${Boolean(sk.isBuiltin)})">
                  <svg class="w-3 h-3"><use href="#icon-trash-2"></use></svg>
                  Hapus
                </button>
              ` : ''}
            </div>
          </div>
        `;
        }).join('');
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

    // Playground account selector & Combo skills state
    const PLAYGROUND_SKILLS_STORAGE_KEY = 'antigravity_selected_skills_v1';

    function loadSavedComboSkills() {
      try {
        const raw = localStorage.getItem(PLAYGROUND_SKILLS_STORAGE_KEY);
        if (raw) {
          const arr = JSON.parse(raw);
          if (Array.isArray(arr)) {
            return new Set(arr.map(s => String(s).toLowerCase()));
          }
        }
      } catch (_) {}
      return new Set([]);
    }

    function saveCurrentComboSkills() {
      try {
        const arr = Array.from(selectedComboSkills);
        localStorage.setItem(PLAYGROUND_SKILLS_STORAGE_KEY, JSON.stringify(arr));
      } catch (_) {}
    }

    let selectedComboSkills = loadSavedComboSkills();

    function selectAllComboSkills() {
      const activeSkills = skillsData.length > 0
        ? skillsData.filter(sk => sk.enabled)
        : [
            { id: 'rtk' }, { id: 'caveman' }, { id: 'ponytail' }, { id: 'context_mode' }, { id: 'codegraph' }
          ];
      selectedComboSkills = new Set(activeSkills.map(s => s.id.toLowerCase()));
      saveCurrentComboSkills();
      renderComboSkillsPills();
    }

    function updatePgAccountsDropdown() {}
    function updatePgAccountBadge() {}

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
      let changed = false;
      const activeIdSet = new Set(activeSkills.map(s => s.id.toLowerCase()));
      for (const id of Array.from(selectedComboSkills)) {
        if (!activeIdSet.has(id)) {
          selectedComboSkills.delete(id);
          changed = true;
        }
      }
      if (changed) {
        saveCurrentComboSkills();
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
      saveCurrentComboSkills();
      renderComboSkillsPills();
    }

    function clearAllComboSkills() {
      selectedComboSkills.clear();
      saveCurrentComboSkills();
      renderComboSkillsPills();
    }

    function clearComboSkills() {
      clearAllComboSkills();
    }

    async function typewriteAssistantMessage(assistantTurn, fullText, sess) {
      if (!fullText) return;
      assistantTurn.content = '';
      
      const totalLen = fullText.length;
      // Hitung ukuran chunk dinamis agar animasi typing mengalir mulus ~1.5 - 2.5 detik
      const stepChunk = Math.max(3, Math.ceil(totalLen / 120));
      let currentIdx = 0;

      while (currentIdx < totalLen) {
        if (!playgroundAbortController || playgroundAbortController.signal.aborted) {
          assistantTurn.content = fullText;
          break;
        }

        currentIdx = Math.min(totalLen, currentIdx + stepChunk);
        assistantTurn.content = fullText.slice(0, currentIdx);
        renderPlaygroundMessages();

        // 16ms delay (~60fps typing tick)
        await new Promise(r => setTimeout(r, 16));
      }

      assistantTurn.content = fullText;
      if (sess) {
        sess.messages = playgroundHistory;
        savePlaygroundSessions();
      }
      renderPlaygroundMessages();
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
        btn.innerHTML = `<svg class="w-4 h-4"><use href="#icon-square"></use></svg>`;
        btn.className = 'w-8 h-8 rounded-xl bg-red-600 hover:bg-red-700 text-white flex items-center justify-center transition-all shrink-0 shadow-2xs mb-0.5 mr-0.5 cursor-pointer';
        btn.title = 'Hentikan generasi (Stop)';
        if (input) input.disabled = true;
      } else {
        btn.innerHTML = `<svg class="w-4 h-4"><use href="#icon-chevron-up"></use></svg>`;
        btn.className = 'w-8 h-8 rounded-xl bg-blue-600 hover:bg-blue-700 text-white flex items-center justify-center transition-all shrink-0 shadow-2xs mb-0.5 mr-0.5 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer';
        btn.title = 'Kirim pesan';
        if (input) {
          input.disabled = false;
          input.focus();
        }
      }
    }

    const PLAYGROUND_TOOLS = [
      {
        type: 'function',
        function: {
          name: 'web_search',
          description: 'Cari informasi terkini di web HANYA jika query pengguna secara eksplisit menanyakan peristiwa/berita hari ini, rilis baru, cuaca, atau info kualitas udara real-time. Jika menanyakan cuaca atau indeks kualitas udara (AQI/IQAir), gunakan kata kunci pencarian real-time dan baca URL resmi platform (seperti iqair.com atau bmkg). DILARANG memanggil tool ini untuk konsep pemrograman atau matematika.',
          parameters: {
            type: 'object',
            properties: {
              query: { type: 'string', description: 'Kata kunci pencarian spesifik (contoh: "indeks kualitas udara palembang iqair", "berita palembang hari ini")' }
            },
            required: ['query']
          }
        }
      },
      {
        type: 'function',
        function: {
          name: 'fetch_web_page',
          description: 'Buka dan baca isi halaman web, portal pemantau cuaca/kualitas udara (seperti iqair.com/id/indonesia/south-sumatra/palembang), atau dokumentasi resmi library. Gunakan untuk mendapatkan angka live faktual dan data detail terkini dari suatu URL.',
          parameters: {
            type: 'object',
            properties: {
              url: { type: 'string', description: 'URL lengkap halaman dokumentasi/web yang ingin dibaca' }
            },
            required: ['url']
          }
        }
      },
      {
        type: 'function',
        function: {
          name: 'github_inspect',
          description: 'Periksa repositori GitHub publik (owner/repo) untuk membaca ringkasan repo, README, atau kode spesifik.',
          parameters: {
            type: 'object',
            properties: {
              repo: { type: 'string', description: 'Nama repo GitHub format "owner/repo" (contoh: "honojs/hono", "vuejs/core")' },
              path: { type: 'string', description: 'Path file opsional (contoh: "README.md", "package.json", "src/index.ts")' }
            },
            required: ['repo']
          }
        }
      }
    ];

    async function sendPlaygroundMessage() {
      if (playgroundAbortController) return;

      const input = document.getElementById('pgInput');
      const text = input.value.trim();
      if (!text) return;

      input.value = '';
      input.style.height = 'auto';
      const startTime = performance.now();
      const model = pgSelectedModel || 'gemini-3.8-flash-tiered';
      const targetAccount = pgSelectedAccount || '';
      const selectedProject = pgSelectedProject || '';
      const skillsArray = Array.from(selectedComboSkills);
      let useWebTools = true; // Default built-in agent tool

      userScrolledUpInPlayground = false;
      playgroundHistory.push({ role: 'user', content: text });

      const sess = getActivePlaygroundSession();
      if (sess) {
        if (!sess.messages || sess.messages.length === 0 || sess.title === 'Percakapan Baru') {
          sess.title = text.length > 28 ? text.slice(0, 28) + '...' : text;
          updateActiveSessionHeader();
        }
        sess.messages = playgroundHistory;
        savePlaygroundSessions();
        renderPlaygroundSessionsList();
      }

      // Assistant streaming placeholder
      const assistantTurn = { role: 'assistant', content: '', toolActivities: [] };
      playgroundHistory.push(assistantTurn);

      // Status generating & skeleton Great Sage langsung di-render ke UI
      playgroundAbortController = new AbortController();
      updatePlaygroundButtonState(true);
      renderPlaygroundMessages(true);

      try {
        const headers = {
          'Content-Type': 'application/json',
          'x-playground-request': 'true',
        };
        if (targetAccount) headers['x-target-account'] = targetAccount;
        if (selectedProject) headers['x-project'] = selectedProject;

        const currentDateString = new Date().toLocaleDateString('id-ID', {
          weekday: 'long',
          year: 'numeric',
          month: 'long',
          day: 'numeric'
        });

        const playgroundSystemInstruction = {
          role: 'system',
          content: 'IDENTITAS & PRINSIP UTAMA: GREAT SAGE (CIEL)\n' +
            `WAKTU SISTEM SAAT INI: Hari ini adalah ${currentDateString}.\n` +
            '1. IDENTITAS: Anda adalah Great Sage (Ciel), Principal Software Architect dan AI System Advisor.\n' +
            '2. KOMUNIKASI & PENALARAN: Berikan analisis mendalam, lugas, deterministik, dan berlandaskan prinsip software engineering serta fakta data yang provable. Hindari basa-basi retoris dan perbincangan tidak relevan.\n' +
            '3. PENGAMBIL KEPUTUSAN TEGAS: Timbang trade-off teknis secara objektif, lalu berikan rekomendasi definitive yang terarah, bukan sekadar opsi mengambang tanpa pendirian.\n' +
            '4. REKAYASA PROMPT AI AGENT: Ketika diminta merancang prompt atau spesifikasi untuk agen otonom (Hermes, Claude Code, Cline, Cursor), susun dengan kepadatan semantik tinggi, pembatas struktural XML/Markdown fences, invariant ketat, state machine deterministik, dan skema output terverifikasi.\n' +
            '5. FOKUS ARSITEKTUR & DESAIN SISTEM: Utamakan perancangan arsitektur, spesifikasi PRD/RFC, skema data, dan analisis sistem. Berikan potongan konfigurasi (misal YAML, JSON, cURL, Docker, Nginx) hanya jika diminta atau esensial untuk solusi, hindari membuat implementasi kode aplikasi mentah (boilerplate full-app).\n' +
            '6. EFISIENSI TOOLS & KEDALAMAN INFORMASI:\n' +
            '   - JANGAN memanggil tool pencarian untuk hal konseptual, logika, pemrograman, atau waktu sistem hari ini (sudah tertera di atas).\n' +
            '   - Panggil tool web_search/fetch_web_page HANYA bila Anda memerlukan data kejadian dunia nyata hari ini, rilis baru setelah cutoff Maret 2026, atau dokumen eksternal yang tidak Anda ketahui.\n' +
            '   - KETIKA MENYAMPAIKAN BERITA/HASIL SEARCH: Sajikan informasi secara kaya, berbobot, dan bernilai berita lengkap (konteks kejadian, tanggal rilis, pihak terlibat, metrik data riil, dan dampaknya).',
        };

        let loopCount = 0;
        const maxLoops = 3;
        let activeMessages = [
          playgroundSystemInstruction,
          ...playgroundHistory.slice(0, -1).map(m => ({ role: m.role, content: m.content })),
        ];

        while (loopCount < maxLoops) {
          loopCount++;
          const requestBody = {
            model: model,
            messages: activeMessages,
            stream: false, // Non-streaming saat agentic tool loop agar buffer konten lengkap & stabil
            account: targetAccount,
            project: selectedProject,
            skills: skillsArray,
          };
          if (useWebTools) {
            requestBody.tools = PLAYGROUND_TOOLS;
          }

          const res = await fetch('/v1/chat/completions', {
            method: 'POST',
            headers: headers,
            signal: playgroundAbortController.signal,
            body: JSON.stringify(requestBody),
          });

          if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            assistantTurn.content = 'Error: ' + (err.error?.message || res.statusText);
            renderPlaygroundMessages();
            return;
          }

          // Metadata capture (Model, Akun riil, Latensi)
          const respAccount = res.headers.get('x-account') || targetAccount || '';
          const respModel = res.headers.get('x-model') || model;
          const fallbackModel = res.headers.get('x-fallback-model');
          const elapsedSec = ((performance.now() - startTime) / 1000).toFixed(1);
          assistantTurn.meta = {
            model: respModel,
            account: respAccount,
            fallback: fallbackModel,
            latency: elapsedSec
          };

          const responseData = await res.json();
          const choice = responseData.choices?.[0] || {};
          const msg = choice.message || {};
          const pendingToolCalls = msg.tool_calls || [];
          const textContent = msg.content || '';

          if (textContent) {
            if (pendingToolCalls.length === 0) {
              // Jawaban final: Jalankan animasi typing smooth dari atas ke bawah
              await typewriteAssistantMessage(assistantTurn, textContent, sess);
            } else {
              assistantTurn.content = textContent;
              if (sess) {
                sess.messages = playgroundHistory;
                savePlaygroundSessions();
              }
              renderPlaygroundMessages();
            }
          }

          // Jika model tidak memanggil tool lagi, berarti jawaban final selesai
          if (pendingToolCalls.length === 0) {
            break;
          }

          // Eksekusi tiap tool call yang diminta Great Sage
          for (const tc of pendingToolCalls) {
            let parsedArgs = {};
            try {
              parsedArgs = JSON.parse(tc.function.arguments || '{}');
            } catch (_) {
              parsedArgs = { query: tc.function.arguments || '' };
            }

            let actionText = 'Mencari di Web';
            let target = parsedArgs.query || parsedArgs.url || parsedArgs.repo || '';
            let source = 'Google News Live Search';

            if (tc.function.name === 'web_search') {
              actionText = 'Mencari di Google/Web';
              target = parsedArgs.query || '';
              source = 'Google News Live Search';
            } else if (tc.function.name === 'fetch_web_page') {
              actionText = 'Membaca Dokumentasi';
              target = parsedArgs.url || '';
              try { source = new URL(parsedArgs.url).hostname; } catch (_) { source = parsedArgs.url; }
            } else if (tc.function.name === 'github_inspect') {
              actionText = 'Memeriksa GitHub';
              target = (parsedArgs.repo || '') + (parsedArgs.path ? '/' + parsedArgs.path : '');
              source = 'GitHub API';
            }

            // Aktifkan live Thought & Tool Box
            assistantTurn.activeTool = {
              name: tc.function.name,
              actionText,
              target,
              source
            };
            renderPlaygroundMessages();

            let toolRes = { success: false, content: 'No result' };
            try {
              const execRes = await fetch('/api/tools/execute', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                signal: playgroundAbortController.signal,
                body: JSON.stringify({
                  name: tc.function.name,
                  args: parsedArgs
                })
              });
              toolRes = await execRes.json();
            } catch (err) {
              toolRes = { success: false, content: 'Gagal mengeksekusi tool: ' + err.message };
            }

            if (!assistantTurn.toolActivities) assistantTurn.toolActivities = [];
            assistantTurn.toolActivities.push({
              name: tc.function.name,
              title: toolRes.title || target || tc.function.name,
              source: toolRes.source || source,
              content: toolRes.content || '',
              success: toolRes.success
            });
            assistantTurn.activeTool = null;
            renderPlaygroundMessages();
          }

          // Masukkan turn assistant (tool calls) dan tool responses ke riwayat prompt
          activeMessages.push({
            role: 'assistant',
            content: textContent || null,
            tool_calls: pendingToolCalls
          });
          for (let i = 0; i < pendingToolCalls.length; i++) {
            const tc = pendingToolCalls[i];
            const act = assistantTurn.toolActivities[assistantTurn.toolActivities.length - pendingToolCalls.length + i];
            activeMessages.push({
              role: 'tool',
              tool_call_id: tc.id,
              name: tc.function.name,
              content: act ? act.content : ''
            });
          }

            // Format tool response sebagai user message kontekstual agar model tidak mengabaikannya
            let toolSummary = '';
            for (let i = 0; i < pendingToolCalls.length; i++) {
              const tc = pendingToolCalls[i];
              const act = assistantTurn.toolActivities[assistantTurn.toolActivities.length - pendingToolCalls.length + i];
              toolSummary += `\n[HASIL TOOL ${tc.function.name}]:\n${act ? act.content : 'Tidak ada data'}\n`;
            }

            activeMessages.push({
              role: 'user',
              content: `Berikut adalah data penelusuran live real-time yang baru saja ditemukan oleh sistem:\n${toolSummary}\n\n` +
                `INSTRUKSI: Buat laporan berita berdasarkan HASIL PENCARIAN DI ATAS untuk menjawab pertanyaan saya. ` +
                `WAJIB cantumkan sumber berita dan tautan/link URL yang ada di data pencarian ke dalam judul atau bagian referensi setiap poin (format: [Judul Berita](URL)). ` +
                `Sertakan juga tanggal/waktu rilis berita tersebut dan jelaskan substansi peristiwanya secara lengkap dan jelas!`
            });

          // Matikan tools pada giliran berikutnya agar LLM fokus menyintesis jawaban akhir
          useWebTools = false;
        }
      } catch (err) {
        console.error('Playground fetch error:', err);
        if (err.name === 'AbortError') {
          assistantTurn.content += '\n\n*(Generasi dihentikan oleh pengguna)*';
        } else {
          assistantTurn.content = 'Network Error: ' + err.message;
        }
      } finally {
        if (assistantTurn) {
          assistantTurn.activeTool = null;
        }
        playgroundAbortController = null;
        updatePlaygroundButtonState(false);
        const statusEl = document.getElementById('pgStatus');
        if (statusEl) {
          statusEl.textContent = 'Ready';
          statusEl.className = 'badge badge-success';
        }
        // Bersihkan turn kosong jika request gagal total tanpa ada konten yang terisi
        if (assistantTurn && !assistantTurn.content && (!assistantTurn.toolActivities || assistantTurn.toolActivities.length === 0)) {
          playgroundHistory = playgroundHistory.filter(m => m !== assistantTurn);
        }
        if (sess) {
          sess.messages = playgroundHistory;
          savePlaygroundSessions();
          renderPlaygroundSessionsList();
        }
        // Panggil render final SETELAH playgroundAbortController di-set ke null
        renderPlaygroundMessages();
      }
    }

    function parseAndRenderMarkdownTables(text) {
      const lines = text.split('\n');
      const tables = [];
      const result = [];
      let i = 0;

      function splitTableRow(line) {
        let trimmed = line.trim();
        if (trimmed.startsWith('|')) trimmed = trimmed.substring(1);
        if (trimmed.endsWith('|')) trimmed = trimmed.substring(0, trimmed.length - 1);
        return trimmed
          .replace(/\\\|/g, '__ESCAPED_PIPE__')
          .split('|')
          .map(c => c.trim().replace(/__ESCAPED_PIPE__/g, '|'));
      }

      function isTableSeparator(line) {
        if (!line.includes('|') && !line.includes('-')) return false;
        const cells = splitTableRow(line);
        return cells.length > 0 && cells.every(c => /^:?-+:?$/.test(c));
      }

      while (i < lines.length) {
        const line = lines[i];
        if (line.includes('|') && i + 1 < lines.length && isTableSeparator(lines[i + 1])) {
          const headerCells = splitTableRow(line);
          const sepCells = splitTableRow(lines[i + 1]);
          const alignments = sepCells.map(sep => {
            const left = sep.startsWith(':');
            const right = sep.endsWith(':');
            if (left && right) return 'text-center';
            if (right) return 'text-right';
            return 'text-left';
          });

          const dataRows = [];
          i += 2;
          while (i < lines.length && lines[i].trim() !== '' && lines[i].includes('|')) {
            dataRows.push(lines[i]);
            i++;
          }

          const thead = `<thead class="bg-slate-50 border-b border-slate-200"><tr>` +
            headerCells.map((cell, cIdx) => `<th class="py-2.5 px-3.5 font-semibold text-slate-700 text-xs ${alignments[cIdx] || 'text-left'}">${cell}</th>`).join('') +
            `</tr></thead>`;

          const tbody = `<tbody class="divide-y divide-slate-100">` +
            dataRows.map(rowLine => {
              const cells = splitTableRow(rowLine);
              return `<tr class="hover:bg-slate-50/80 transition-colors">` +
                cells.map((cell, cIdx) => `<td class="py-2.5 px-3.5 text-slate-800 text-xs leading-relaxed ${alignments[cIdx] || 'text-left'}">${cell}</td>`).join('') +
                `</tr>`;
            }).join('') +
            `</tbody>`;

          const tableHtml = `<div class="overflow-x-auto my-3 rounded-xl border border-slate-200 bg-white shadow-2xs not-prose"><table class="w-full text-xs border-collapse">${thead}${tbody}</table></div>`;
          const placeholder = `__TABLE_BLOCK_${tables.length}__`;
          tables.push(tableHtml);
          result.push(placeholder);
        } else {
          result.push(line);
          i++;
        }
      }

      return { text: result.join('\n'), tables };
    }

    function formatChatContent(content, isStreaming = false) {
      if (!content) {
        return isStreaming ? `
          <div class="py-1 px-0.5 select-none inline-flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-slate-400 animate-bounce" style="animation-delay: -0.32s"></span>
            <span class="w-2 h-2 rounded-full bg-slate-400 animate-bounce" style="animation-delay: -0.16s"></span>
            <span class="w-2 h-2 rounded-full bg-slate-400 animate-bounce"></span>
          </div>
        ` : '';
      }

      // 1. Check if message is an Interrogation / Grill-me format HANYA jika skill grill-me aktif
      const isGrillMeSkillActive = selectedComboSkills.has('grill_me') || selectedComboSkills.has('grill-me');
      if (isGrillMeSkillActive && (content.includes('❓') || /\[GRILL[-_]?ME\]/i.test(content) || /\[[A-D]\]/i.test(content))) {
        if (isStreaming) {
          return cleanGrillMarkdown(content).replace(/\n/g, '<br/>') + ' <span class="inline-flex items-center gap-1 ml-1.5 align-middle"><span class="w-1.5 h-1.5 rounded-full bg-slate-400 animate-bounce" style="animation-delay: -0.32s"></span><span class="w-1.5 h-1.5 rounded-full bg-slate-400 animate-bounce" style="animation-delay: -0.16s"></span><span class="w-1.5 h-1.5 rounded-full bg-slate-400 animate-bounce"></span></span>';
        }
        return renderInteractiveGrillMe(content);
      }

      // 1. Ekstrak code blocks
      const codeBlocks = [];
      let text = content.replace(/```([a-zA-Z0-9_-]*)\n([\s\S]*?)```/g, (match, lang, code) => {
        const id = `__CODE_BLOCK_${codeBlocks.length}__`;
        const displayLang = (lang || 'code').toUpperCase();
        const blockHtml = `
          <div class="my-3 rounded-xl border border-slate-700/80 bg-slate-950 overflow-hidden shadow-inner text-xs not-prose">
            <div class="flex items-center justify-between px-3.5 py-1.5 bg-slate-900 border-b border-slate-800 text-[11px] text-slate-400 font-mono">
              <span class="font-semibold text-slate-300 tracking-wider">${escapeHtml(displayLang)}</span>
              <button type="button" class="inline-flex items-center gap-1 px-2 py-0.5 rounded hover:bg-slate-800 hover:text-white transition-colors text-slate-400" onclick="copyCodeBlock(this)">
                <svg class="w-3 h-3"><use href="#icon-copy"></use></svg>
                <span>Salin</span>
              </button>
            </div>
            <pre class="p-3.5 overflow-x-auto font-mono text-slate-100 text-[11.5px] leading-relaxed"><code>${escapeHtml(code)}</code></pre>
          </div>
        `;
        codeBlocks.push(blockHtml);
        return id;
      });

      // 2. Escape HTML awal untuk keamanan XSS
      let formatted = escapeHtml(text);

      // 3. Render Markdown Heading (###### -> h6 sampai # -> h1)
      formatted = formatted
        .replace(/^######[ \t]+(.*$)/gim, '<h6 class="text-xs font-bold text-slate-700 mt-2 mb-0.5">$1</h6>')
        .replace(/^#####[ \t]+(.*$)/gim, '<h5 class="text-xs font-bold text-slate-800 mt-2.5 mb-0.5">$1</h5>')
        .replace(/^####[ \t]+(.*$)/gim, '<h4 class="text-xs font-extrabold uppercase tracking-wider text-slate-700 mt-2.5 mb-1 pb-0.5 border-b border-slate-100">$1</h4>')
        .replace(/^###[ \t]+(.*$)/gim, '<h3 class="text-sm font-bold text-slate-900 mt-3 mb-1">$1</h3>')
        .replace(/^##[ \t]+(.*$)/gim, '<h2 class="text-base font-extrabold text-slate-900 mt-3 mb-1 pb-0.5 border-b border-slate-200/80">$1</h2>')
        .replace(/^#[ \t]+(.*$)/gim, '<h1 class="text-lg font-black text-slate-900 mt-3.5 mb-1.5 pb-1 border-b-2 border-slate-300">$1</h1>');

      // 4. Render Garis Horizontal (--- atau ***)
      formatted = formatted.replace(/^[ \t]*[-*_]{3,}[ \t]*$/gm, '<hr class="my-3 border-t border-slate-200/90"/>');

      // 5. Render Bold (**text**) dan Italic (*text*)
      formatted = formatted
        .replace(/\*\*(.*?)\*\*/g, '<strong class="font-bold text-slate-900">$1</strong>')
        .replace(/__([^_]+)__/g, '<strong class="font-bold text-slate-900">$1</strong>')
        .replace(/(^|[^\*])\*([^\*\n]+)\*([^\*]|$)/g, '$1<em class="italic text-slate-700">$2</em>$3');

      // 5.5 Render Notasi Ilmiah, LaTeX Fisika/Kimia, dan Satuan Lingkungan
      formatted = formatted
        // Ubah LaTeX \mu g/m^3 atau $\mu g/m^3$ -> µg/m³
        .replace(/\$?\\mu\s*g\s*\/\s*m\^?\{?3\}?\$?/gi, 'µg/m³')
        .replace(/\$?\\mu\s*g\$?/gi, 'µg')
        .replace(/µg\/m\^?3/gi, 'µg/m³')
        .replace(/ug\/m\^?3/gi, 'µg/m³')
        // Notasi eksponen kubik/persegi umum: m^3 -> m³, m^2 -> m²
        .replace(/\b([a-zA-Z]+)\^3\b/g, '$1³')
        .replace(/\b([a-zA-Z]+)\^2\b/g, '$1²')
        // Rumus kimia subskrip ($O_3$, $CO_2$, $PM_{2.5}$)
        .replace(/\$([A-Za-z]+)_\{?([0-9]+(?:\.[0-9]+)?)\}?\$/g, '$1<sub>$2</sub>')
        .replace(/\$([A-Za-z]+)([0-9]+(?:\.[0-9]+)?)\$/g, '$1<sub>$2</sub>')
        .replace(/\$([A-Za-z0-9]+)\$/g, '$1')
        // Bersihkan tanda bintang tunggal tak berpasangan di akhir kata (misal outdoor* -> outdoor)
        .replace(/([a-zA-Z0-9])\*(?![a-zA-Z0-9\*])/g, '$1');

      // 6. Render Inline Code (`code`)
      formatted = formatted.replace(/`([^`\n]+)`/g, '<code class="bg-slate-100 text-slate-800 font-mono text-[11px] px-1.5 py-0.5 rounded border border-slate-200/80">$1</code>');

      // 7. Render Markdown Links [Judul](url) ala ChatGPT dengan badge domain
      formatted = formatted.replace(/\[([^\]]+)\]\((https?:\/\/[^\s\)]+)\)/g, (match, title, url) => {
        let hostname = '';
        try {
          hostname = new URL(url).hostname.replace(/^www\./, '');
        } catch (_) {}
        const badge = hostname ? `<span class="inline-flex items-center text-[10px] text-slate-500 bg-slate-100 hover:bg-slate-200 border border-slate-200/90 rounded-md px-1.5 py-0.5 ml-1 font-mono font-normal tracking-tight transition-colors">${hostname} ↗</span>` : '';
        return `<a href="${url}" target="_blank" rel="noopener noreferrer" class="text-blue-600 hover:text-blue-800 font-semibold hover:underline inline-flex items-center gap-0.5">${title}${badge}</a>`;
      });

      // 8. Render Blockquote (> kutipan)
      formatted = formatted.replace(/^>[ \t]+(.*$)/gim, '<blockquote class="border-l-3 border-blue-500 bg-blue-50/50 pl-3 py-1 my-2 text-xs text-slate-600 italic rounded-r-lg">$1</blockquote>');

      // 9. Render Markdown Tables
      const tableRes = parseAndRenderMarkdownTables(formatted);
      formatted = tableRes.text;

      // 10. Kembalikan blok kode
      codeBlocks.forEach((cb, idx) => {
        formatted = formatted.replace(`__CODE_BLOCK_${idx}__`, cb);
      });

      // 11. Normalisasi Whitespace & Line breaks (kompak 1 spasi baris antar-konten)
      // a. Hapus baris kosong yang menempel langsung sebelum dan sesudah heading
      formatted = formatted
        .replace(/(?:<br\s*\/?>|\n)*\s*(<h[1-6][^>]*>)/gi, '$1')
        .replace(/(<\/h[1-6]>)\s*(?:<br\s*\/?>|\n)*/gi, '$1');

      // b. Hapus baris kosong yang menempel langsung sebelum dan sesudah horizontal rule
      formatted = formatted
        .replace(/(?:<br\s*\/?>|\n)*\s*(<hr[^>]*\/?>)/gi, '$1')
        .replace(/(<hr[^>]*\/?>)\s*(?:<br\s*\/?>|\n)*/gi, '$1');

      // c. Hapus baris kosong yang menempel pada blockquote
      formatted = formatted
        .replace(/(?:<br\s*\/?>|\n)*\s*(<blockquote[^>]*>)/gi, '$1')
        .replace(/(<\/blockquote>)\s*(?:<br\s*\/?>|\n)*/gi, '$1');

      // d. Konversi baris baru: semua multi-newline (\n\n atau lebih) dijadikan cukup 1 spasi baris (<br/>)
      formatted = formatted.replace(/\n+/g, '<br/>');

      // 12. Kembalikan tabel & bersihkan tag <br/> berlebih di sekitarnya
      tableRes.tables.forEach((tbl, idx) => {
        const placeholder = `__TABLE_BLOCK_${idx}__`;
        formatted = formatted
          .replace(new RegExp(`(?:<br\\s*\\/?>\\s*)+${placeholder}`, 'g'), placeholder)
          .replace(new RegExp(`${placeholder}(?:\\s*<br\\s*\\/?>)+`, 'g'), placeholder)
          .replace(placeholder, tbl);
      });

      if (isStreaming) {
        formatted += ' <span class="inline-flex items-center gap-1 ml-1.5 align-middle"><span class="w-1.5 h-1.5 rounded-full bg-slate-400 animate-bounce" style="animation-delay: -0.32s"></span><span class="w-1.5 h-1.5 rounded-full bg-slate-400 animate-bounce" style="animation-delay: -0.16s"></span><span class="w-1.5 h-1.5 rounded-full bg-slate-400 animate-bounce"></span></span>';
      }

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

    function copyMessageContent(idx, btn) {
      const m = playgroundHistory[idx];
      if (!m || !m.content) return;
      navigator.clipboard.writeText(m.content).then(() => {
        if (btn) {
          const original = btn.innerHTML;
          btn.innerHTML = '<svg class="w-3.5 h-3.5 text-emerald-600"><use href="#icon-check"></use></svg>';
          btn.classList.add('bg-emerald-50', 'border-emerald-300');
          setTimeout(() => {
            btn.innerHTML = original;
            btn.classList.remove('bg-emerald-50', 'border-emerald-300');
          }, 1500);
        } else {
          alert('Teks berhasil disalin ke clipboard!');
        }
      }).catch(err => {
        console.error('Copy failed:', err);
      });
    }

    function exportMessageToMarkdown(idx, btn) {
      const m = playgroundHistory[idx];
      if (!m || !m.content) return;
      const blob = new Blob([m.content], { type: 'text/markdown;charset=utf-8;' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.setAttribute('href', url);
      link.setAttribute('download', `great-sage-ciel-${Date.now()}.md`);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
      if (btn) {
        const original = btn.innerHTML;
        btn.innerHTML = '<svg class="w-3.5 h-3.5 text-emerald-600"><use href="#icon-check"></use></svg>';
        btn.classList.add('bg-emerald-50', 'border-emerald-300');
        setTimeout(() => {
          btn.innerHTML = original;
          btn.classList.remove('bg-emerald-50', 'border-emerald-300');
        }, 1500);
      }
    }

    let userScrolledUpInPlayground = false;

    function loadPlaygroundSessions() {
      try {
        const raw = localStorage.getItem(PG_SESSIONS_KEY);
        if (raw) {
          playgroundSessions = JSON.parse(raw);
        }
      } catch (e) {
        playgroundSessions = [];
      }
      const savedActiveId = localStorage.getItem(PG_ACTIVE_SESSION_KEY);
      if (!Array.isArray(playgroundSessions) || playgroundSessions.length === 0) {
        const initialSession = {
          id: 'sess_' + Date.now(),
          title: 'Percakapan Baru',
          messages: [],
          createdAt: new Date().toISOString(),
        };
        playgroundSessions = [initialSession];
        currentPlaygroundSessionId = initialSession.id;
        savePlaygroundSessions();
      } else {
        if (savedActiveId && playgroundSessions.some(s => s.id === savedActiveId)) {
          currentPlaygroundSessionId = savedActiveId;
        } else if (!currentPlaygroundSessionId || !playgroundSessions.some(s => s.id === currentPlaygroundSessionId)) {
          currentPlaygroundSessionId = playgroundSessions[0].id;
        }
      }
    }

    function savePlaygroundSessions() {
      try {
        localStorage.setItem(PG_SESSIONS_KEY, JSON.stringify(playgroundSessions));
        if (currentPlaygroundSessionId) {
          localStorage.setItem(PG_ACTIVE_SESSION_KEY, currentPlaygroundSessionId);
        }
      } catch (e) {
        console.error('Failed to save playground sessions:', e);
      }
    }

    function getActivePlaygroundSession() {
      if (!currentPlaygroundSessionId || playgroundSessions.length === 0) {
        loadPlaygroundSessions();
      }
      let sess = playgroundSessions.find(s => s.id === currentPlaygroundSessionId);
      if (!sess) {
        loadPlaygroundSessions();
        sess = playgroundSessions[0];
      }
      return sess;
    }

    function syncPlaygroundHistoryWithSession() {
      const sess = getActivePlaygroundSession();
      if (sess) {
        playgroundHistory = sess.messages || [];
      } else {
        playgroundHistory = [];
      }
    }

    function createNewPlaygroundSession() {
      const newSess = {
        id: 'sess_' + Date.now(),
        title: 'Percakapan Baru',
        messages: [],
        createdAt: new Date().toISOString(),
      };
      playgroundSessions.unshift(newSess);
      currentPlaygroundSessionId = newSess.id;
      playgroundHistory = [];
      savePlaygroundSessions();
      renderPlaygroundSessionsList();
      renderPlaygroundMessages(true);
      updateActiveSessionHeader();
    }

    function selectPlaygroundSession(sessionId) {
      const target = playgroundSessions.find(s => s.id === sessionId);
      if (!target) return;
      currentPlaygroundSessionId = sessionId;
      playgroundHistory = target.messages || [];
      savePlaygroundSessions();
      renderPlaygroundSessionsList();
      renderPlaygroundMessages(true);
      updateActiveSessionHeader();
    }

    function deletePlaygroundSession(sessionId, event) {
      if (event) event.stopPropagation();
      playgroundSessions = playgroundSessions.filter(s => s.id !== sessionId);
      if (playgroundSessions.length === 0) {
        const initialSession = {
          id: 'sess_' + Date.now(),
          title: 'Percakapan Baru',
          messages: [],
          createdAt: new Date().toISOString(),
        };
        playgroundSessions = [initialSession];
        currentPlaygroundSessionId = initialSession.id;
      } else if (currentPlaygroundSessionId === sessionId) {
        currentPlaygroundSessionId = playgroundSessions[0].id;
      }
      syncPlaygroundHistoryWithSession();
      savePlaygroundSessions();
      renderPlaygroundSessionsList();
      renderPlaygroundMessages(true);
      updateActiveSessionHeader();
    }

    function clearAllPlaygroundSessions() {
      const initialSession = {
        id: 'sess_' + Date.now(),
        title: 'Percakapan Baru',
        messages: [],
        createdAt: new Date().toISOString(),
      };
      playgroundSessions = [initialSession];
      currentPlaygroundSessionId = initialSession.id;
      playgroundHistory = [];
      savePlaygroundSessions();
      renderPlaygroundSessionsList();
      renderPlaygroundMessages(true);
      updateActiveSessionHeader();
    }

    function updateActiveSessionHeader() {
      const titleEl = document.getElementById('pgActiveSessionTitle');
      const sess = getActivePlaygroundSession();
      if (titleEl && sess) {
        titleEl.textContent = sess.title || 'Percakapan Baru';
      }
    }

    function renderPlaygroundSessionsList() {
      const listEl = document.getElementById('pgSessionsList');
      const countEl = document.getElementById('pgSessionCount');
      if (!listEl) return;

      if (countEl) {
        countEl.textContent = `${playgroundSessions.length} Percakapan`;
      }

      listEl.innerHTML = playgroundSessions.map(s => {
        const isActive = s.id === currentPlaygroundSessionId;
        return `
          <div class="flex items-center justify-between px-3 py-2 rounded-xl text-xs font-medium cursor-pointer transition-colors group ${
            isActive
              ? 'bg-blue-50 text-blue-700 font-bold border border-blue-200/80 shadow-2xs'
              : 'text-slate-600 hover:bg-slate-200/60 hover:text-slate-900'
          }" onclick="selectPlaygroundSession('${s.id}')">
            <div class="flex items-center gap-2 min-w-0 flex-1 mr-2">
              <svg class="w-3.5 h-3.5 shrink-0 ${isActive ? 'text-blue-600' : 'text-slate-400'}"><use href="#icon-message-square"></use></svg>
              <span class="truncate">${escapeHtml(s.title || 'Percakapan')}</span>
            </div>
            <button class="opacity-0 group-hover:opacity-100 p-1 hover:text-red-600 text-slate-400 rounded-md transition-opacity shrink-0" onclick="deletePlaygroundSession('${s.id}', event)" title="Hapus percakapan">
              <svg class="w-3.5 h-3.5"><use href="#icon-trash-2"></use></svg>
            </button>
          </div>
        `;
      }).join('');
    }

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

    function handlePlaygroundKeydown(event) {
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        sendPlaygroundMessage();
      }
    }

    function autoResizePlaygroundInput(el) {
      if (!el) return;
      el.style.height = 'auto';
      el.style.height = Math.min(el.scrollHeight, 128) + 'px';
    }

    function copyCodeBlock(btn) {
      const block = btn.closest('.not-prose');
      const code = block ? block.querySelector('pre code') : null;
      if (!code) return;
      const text = code.innerText || code.textContent || '';
      navigator.clipboard.writeText(text).then(() => {
        const span = btn.querySelector('span');
        const origText = span ? span.textContent : 'Salin';
        if (span) span.textContent = 'Disalin!';
        btn.classList.add('text-emerald-400');
        setTimeout(() => {
          if (span) span.textContent = origText;
          btn.classList.remove('text-emerald-400');
        }, 1500);
      });
    }

    function renderAssistantBubble(m, isStreamingTurn) {
      let html = '';

      // 1. Active tool call / Thinking state (HANYA tampil saat proses berjalan, persis seperti Searching the web di ChatGPT)
      if (m.activeTool) {
        html += `
          <div class="my-2 p-2.5 rounded-xl bg-slate-50 border border-slate-200 shadow-2xs font-sans text-xs not-prose">
            <div class="flex items-center gap-2.5">
              <div class="w-6 h-6 rounded-lg bg-blue-100 flex items-center justify-center text-blue-600 shrink-0">
                <svg class="w-3.5 h-3.5"><use href="#icon-search"></use></svg>
              </div>
              <div class="flex-1 min-w-0">
                <div class="font-bold text-slate-800 truncate text-[11px]">${escapeHtml(m.activeTool.actionText || 'Mencari web')}: <span class="text-blue-700 font-mono">"${escapeHtml(m.activeTool.target)}"</span></div>
                <div class="text-[10px] text-slate-500 truncate mt-0.5">
                  <span>${escapeHtml(m.activeTool.source || 'Web')}</span> • <span class="text-slate-400">Menelusuri informasi terkini...</span>
                </div>
              </div>
              <div class="inline-flex items-center gap-1 pl-2 shrink-0">
                <span class="w-1.5 h-1.5 rounded-full bg-blue-500 animate-bounce" style="animation-delay: -0.32s"></span>
                <span class="w-1.5 h-1.5 rounded-full bg-blue-500 animate-bounce" style="animation-delay: -0.16s"></span>
                <span class="w-1.5 h-1.5 rounded-full bg-blue-500 animate-bounce"></span>
              </div>
            </div>
          </div>
        `;
      }

      // 2. Main textual response
      if (m.content) {
        html += formatChatContent(m.content, isStreamingTurn && !m.activeTool);
        // Metadata badge (Model, Akun riil, Latensi)
        if (m.meta && !isStreamingTurn) {
          html += `
            <div class="mt-2.5 pt-1.5 border-t border-slate-100 flex items-center flex-wrap gap-2 text-[10.5px] text-slate-400 font-mono select-none not-prose">
              <span class="inline-flex items-center gap-1 font-semibold text-slate-600">⚡ ${escapeHtml(m.meta.model || '')}</span>
              ${m.meta.account ? `<span>•</span><span class="text-slate-500">${escapeHtml(m.meta.account)}</span>` : ''}
              ${m.meta.latency ? `<span>•</span><span>${m.meta.latency}s</span>` : ''}
              ${m.meta.fallback ? `<span class="badge badge-warning text-[9px] py-0 px-1 font-sans">Fallback dari ${escapeHtml(m.meta.fallback)}</span>` : ''}
            </div>
          `;
        }
      } else if (!m.activeTool && isStreamingTurn) {
        // Normal 3 bouncing dots only when NO tool is running
        html += `
          <div class="py-1 px-0.5 select-none inline-flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-slate-400 animate-bounce" style="animation-delay: -0.32s"></span>
            <span class="w-2 h-2 rounded-full bg-slate-400 animate-bounce" style="animation-delay: -0.16s"></span>
            <span class="w-2 h-2 rounded-full bg-slate-400 animate-bounce"></span>
          </div>
        `;
      } else if (!m.activeTool && !isStreamingTurn && (!m.content || !m.content.trim())) {
        html += `<div class="text-xs text-slate-500 italic py-1">Data referensi telah berhasil ditarik. Silakan ajukan rincian bagian yang ingin dianalisis.</div>`;
      }

      return html;
    }

    function renderPlaygroundMessages(forceScroll = false) {
      const el = document.getElementById('pgMessages');
      if (!el) return;
      initPlaygroundScrollListener();

      const wasAtBottom = (el.scrollHeight - el.scrollTop - el.clientHeight) <= 60;

      if (!playgroundHistory || playgroundHistory.length === 0) {
        // Tampilkan Card Konfigurasi Parameter (Model, Akun, Proyek) hanya saat sesi baru / chat belum dimulai
        let accountOptions = '<option value="">Auto (Account Pool &amp; Failover)</option>';
        accountsData.forEach(acc => {
          accountOptions += `<option value="${escapeHtml(acc.email)}" ${pgSelectedAccount === acc.email ? 'selected' : ''}>${escapeHtml(acc.name || acc.email)} (${acc.isActive ? 'Active' : 'Disabled'})</option>`;
        });

        let projectOptions = '<option value="">Tanpa Konteks Proyek (General)</option>';
        projectsData.forEach(p => {
          const badge = p.techStack ? ` [${p.techStack}]` : '';
          projectOptions += `<option value="${escapeHtml(p.name)}" ${pgSelectedProject === p.name ? 'selected' : ''}>${escapeHtml(p.name)}${badge}</option>`;
        });

        el.innerHTML = `
          <div class="h-full flex flex-col items-center justify-center p-4 max-w-lg mx-auto text-center">
            <div class="w-12 h-12 rounded-2xl bg-gradient-to-tr from-blue-600 to-indigo-600 text-white flex items-center justify-center shadow-md mb-3.5">
              <svg class="w-6 h-6"><use href="#icon-sparkles"></use></svg>
            </div>
            <h3 class="text-base font-extrabold text-slate-800 mb-1">Mulai Percakapan Baru</h3>
            <p class="text-xs text-slate-500 mb-5 leading-relaxed">Tentukan parameter pengujian gateway untuk sesi ini sebelum mengirimkan instruksi pertama.</p>
            
            <div class="w-full bg-slate-50/90 border border-slate-200/90 rounded-2xl p-4 text-left shadow-2xs space-y-3">
              <div>
                <label class="text-[10.5px] font-bold text-slate-500 uppercase tracking-wider block mb-1">PILIH MODEL</label>
                <select id="pgModel" class="block w-full px-3 py-2 rounded-xl border border-slate-200 text-xs bg-white shadow-xs focus:ring-2 focus:ring-blue-500/20" onchange="handlePgModelChange(this.value)">
                  <option value="gemini-3.8-flash-tiered" ${pgSelectedModel === 'gemini-3.8-flash-tiered' ? 'selected' : ''}>Gemini 3.8 Flash (Auto Tiered)</option>
                  <option value="claude-sonnet-4-6" ${pgSelectedModel === 'claude-sonnet-4-6' ? 'selected' : ''}>Claude Sonnet 4.6 (Thinking)</option>
                  <option value="claude-opus-4-6-thinking" ${pgSelectedModel === 'claude-opus-4-6-thinking' ? 'selected' : ''}>Claude Opus 4.6 (Thinking)</option>
                  <option value="gpt-oss-120b-medium" ${pgSelectedModel === 'gpt-oss-120b-medium' ? 'selected' : ''}>GPT-OSS 120B (Medium)</option>
                </select>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
                <div>
                  <label class="text-[10.5px] font-bold text-slate-500 uppercase tracking-wider block mb-1">AKUN PENGUJI</label>
                  <select id="pgAccount" class="block w-full px-2.5 py-2 rounded-xl border border-slate-200 text-xs bg-white shadow-xs focus:ring-2 focus:ring-blue-500/20" onchange="handlePgAccountChange(this.value)">
                    ${accountOptions}
                  </select>
                </div>

                <div>
                  <label class="text-[10.5px] font-bold text-slate-500 uppercase tracking-wider block mb-1">PROYEK TERSEDIA</label>
                  <select id="pgProject" class="block w-full px-2.5 py-2 rounded-xl border border-slate-200 text-xs bg-white shadow-xs focus:ring-2 focus:ring-blue-500/20" onchange="handlePgProjectChange(this.value)">
                    ${projectOptions}
                  </select>
                </div>
              </div>
            </div>
          </div>
        `;
        return;
      }

      el.innerHTML = playgroundHistory.map((m, idx) => {
        const isStreamingTurn = !!playgroundAbortController && idx === playgroundHistory.length - 1;
        if (m.role === 'user') {
          return `
            <div class="flex flex-col w-full mb-3.5 items-end">
              <div class="mb-1 px-1 flex items-center gap-1.5">
                <span class="text-xs font-bold text-slate-500">Anda</span>
                <div class="w-4 h-4 rounded-full bg-blue-600 text-white flex items-center justify-center text-[9px] font-bold">U</div>
              </div>
              <div class="flex items-start justify-end gap-1.5 w-full group">
                <div class="flex flex-col gap-1 shrink-0 pt-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button type="button" class="p-1.5 rounded-lg border border-slate-200 bg-white hover:bg-slate-100 text-slate-500 hover:text-slate-800 transition-colors shadow-2xs" onclick="copyMessageContent(${idx}, this)" title="Salin pesan Anda">
                    <svg class="w-3.5 h-3.5"><use href="#icon-copy"></use></svg>
                  </button>
                </div>
                <div class="chat-bubble user max-w-[82%]">
                  ${escapeHtml(m.content).replace(/\n/g, '<br/>')}
                </div>
              </div>
            </div>
          `;
        }

        // Assistant message (Great Sage Ciel)
        return `
          <div class="flex flex-col w-full mb-3.5 items-start">
            <div class="flex items-center gap-1.5 mb-1 px-1">
              <div class="w-4 h-4 rounded-full bg-violet-600 text-white flex items-center justify-center text-[9px] font-bold">C</div>
              <span class="text-xs font-bold text-slate-700">Great Sage (Ciel)</span>
              ${idx === 1 && m.meta ? `<span class="text-[10px] text-slate-400 font-mono">(${escapeHtml(m.meta.model || '')})</span>` : ''}
            </div>
            <div class="flex items-start gap-1.5 w-full group">
              <div class="chat-bubble assistant max-w-[calc(100%-54px)]">
                ${renderAssistantBubble(m, isStreamingTurn)}
              </div>
              ${(!isStreamingTurn || m.content || (m.toolActivities && m.toolActivities.length > 0)) ? `
                <div class="flex flex-col gap-1 shrink-0 pt-0.5 opacity-60 group-hover:opacity-100 transition-opacity">
                  <button type="button" class="p-1.5 rounded-lg border border-slate-200 bg-white hover:bg-slate-100 text-slate-500 hover:text-slate-800 transition-colors shadow-2xs" onclick="copyMessageContent(${idx}, this)" title="Salin pesan">
                    <svg class="w-3.5 h-3.5"><use href="#icon-copy"></use></svg>
                  </button>
                  <button type="button" class="p-1.5 rounded-lg border border-slate-200 bg-white hover:bg-slate-100 text-slate-500 hover:text-slate-800 transition-colors shadow-2xs" onclick="exportMessageToMarkdown(${idx}, this)" title="Export Markdown (.md)">
                    <svg class="w-3.5 h-3.5"><use href="#icon-download"></use></svg>
                  </button>
                </div>
              ` : ''}
            </div>
          </div>
        `;
      }).join('');

      // Hanya auto-scroll jika diminta secara paksa (misal user kirim pesan baru)
      // atau jika user sedang berada di posisi paling bawah (belum sengaja scroll ke atas)
      if (forceScroll || (!userScrolledUpInPlayground && wasAtBottom)) {
        el.scrollTop = el.scrollHeight;
      }
    }

    function clearPlayground() {
      playgroundHistory = [];
      const sess = getActivePlaygroundSession();
      if (sess) {
        sess.messages = [];
        sess.title = 'Percakapan Baru';
        savePlaygroundSessions();
      }
      updateActiveSessionHeader();
      renderPlaygroundSessionsList();
      renderPlaygroundMessages(true);
    }

    // Token Pricing Benchmark per 1M Tokens (USD) - Kurs USD 1 = Rp 18.000
    // Data Riil Resmi dari Website Official: Google AI Studio (Gemini 3.8 Flash) & Anthropic (Claude 4.6)
    const USD_IDR_EXCHANGE_RATE = 18000;
    const MODEL_PRICING_BENCHMARK = {
      // Google AI Studio (Gemini 3.8 Flash): Input $0.75, Cached $0.375 (50% off), Output $3.75 per 1M
      'gemini': {
        inputPerMillion: 0.75,
        cachedPerMillion: 0.375, // Prompt caching 50% discount
        outputPerMillion: 3.75,
      },
      // Anthropic Official (Claude Sonnet 4.6): Input $3.00, Cached $0.30 (90% off), Output $15.00 per 1M
      'claude-sonnet': {
        inputPerMillion: 3.00,
        cachedPerMillion: 0.30, // Prompt cache read hit diskon 90%
        outputPerMillion: 15.00,
      },
      // Anthropic Official (Claude Opus 4.6 Thinking): Input $15.00, Cached $1.50 (90% off), Output $75.00 per 1M
      'claude-opus': {
        inputPerMillion: 15.00,
        cachedPerMillion: 1.50, // Prompt cache read hit diskon 90%
        outputPerMillion: 75.00,
      },
      // Default: Gemini 3.8 Flash ($0.75 / $3.75 per 1M)
      'default': {
        inputPerMillion: 0.75,
        cachedPerMillion: 0.375,
        outputPerMillion: 3.75,
      }
    };

    function getModelPricingConfig(modelName) {
      const m = (modelName || '').toLowerCase();
      if (m.includes('claude') && m.includes('opus')) return MODEL_PRICING_BENCHMARK['claude-opus'];
      if (m.includes('claude') || m.includes('sonnet')) return MODEL_PRICING_BENCHMARK['claude-sonnet'];
      if (m.includes('gemini') || m.includes('flash')) return MODEL_PRICING_BENCHMARK['gemini'];
      return MODEL_PRICING_BENCHMARK['default'];
    }

    function calculateTokensCostIDR(modelName, inputTokens, cachedTokens, outputTokens) {
      const cfg = getModelPricingConfig(modelName);
      const inputCostUSD = (inputTokens / 1000000) * cfg.inputPerMillion;
      const cachedCostUSD = (cachedTokens / 1000000) * cfg.cachedPerMillion;
      const outputCostUSD = (outputTokens / 1000000) * cfg.outputPerMillion;
      const totalCostUSD = inputCostUSD + cachedCostUSD + outputCostUSD;

      return {
        inputCostIDR: Math.round(inputCostUSD * USD_IDR_EXCHANGE_RATE * 100) / 100,
        cachedCostIDR: Math.round(cachedCostUSD * USD_IDR_EXCHANGE_RATE * 100) / 100,
        outputCostIDR: Math.round(outputCostUSD * USD_IDR_EXCHANGE_RATE * 100) / 100,
        totalCostIDR: Math.round(totalCostUSD * USD_IDR_EXCHANGE_RATE * 100) / 100,
      };
    }

    function formatRupiah(amount) {
      if (typeof amount !== 'number' || isNaN(amount)) return 'Rp 0';
      if (amount >= 1000000) {
        return 'Rp ' + (amount / 1000000).toLocaleString('id-ID', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' Jt';
      }
      return 'Rp ' + Math.round(amount).toLocaleString('id-ID');
    }

    function formatRupiahExact(amount) {
      if (typeof amount !== 'number' || isNaN(amount)) return 'Rp 0';
      if (amount < 100) {
        return 'Rp ' + amount.toLocaleString('id-ID', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
      }
      return 'Rp ' + Math.round(amount).toLocaleString('id-ID');
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

        // 1. Update KPI Cards & Cost Valuation
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
        if (cachedEl) cachedEl.textContent = 'Cached: ' + (sum.totalCachedTokens || 0).toLocaleString();

        const inputEl = document.getElementById('statTotalInputTokens');
        if (inputEl) inputEl.textContent = (sum.totalInputTokens || 0).toLocaleString() + ' Tokens';

        // Hitung Biaya Kumulatif Seluruh Model (Incurred vs Saved)
        let totalCostIncurredIDR = 0;
        let totalCostSavedIDR = 0;
        let totalInputCostIDR = 0;
        let totalCachedCostIDR = 0;
        let totalOutputCostIDR = 0;

        const byModelList = data.byModel || [];
        byModelList.forEach(m => {
          const cost = calculateTokensCostIDR(m.model, m.inputTokens || 0, m.cachedTokens || 0, m.actualOutput || 0);
          totalInputCostIDR += cost.inputCostIDR;
          totalCachedCostIDR += cost.cachedCostIDR;
          totalOutputCostIDR += cost.outputCostIDR;
          totalCostIncurredIDR += cost.totalCostIDR;

          // Saved Output Cost
          const cfg = getModelPricingConfig(m.model);
          const savedUSD = ((m.savedOutput || 0) / 1000000) * cfg.outputPerMillion;
          totalCostSavedIDR += savedUSD * USD_IDR_EXCHANGE_RATE;
        });

        // Jika byModel kosong tapi ada summary global (misal model filter tunggal)
        if (byModelList.length === 0 && (sum.totalInputTokens || sum.totalActualOutput)) {
          const cost = calculateTokensCostIDR(selectedModel || 'gemini', sum.totalInputTokens || 0, sum.totalCachedTokens || 0, sum.totalActualOutput || 0);
          totalInputCostIDR = cost.inputCostIDR;
          totalCachedCostIDR = cost.cachedCostIDR;
          totalOutputCostIDR = cost.outputCostIDR;
          totalCostIncurredIDR = cost.totalCostIDR;
          const cfg = getModelPricingConfig(selectedModel || 'gemini');
          totalCostSavedIDR = ((sum.totalSavedOutput || 0) / 1000000) * cfg.outputPerMillion * USD_IDR_EXCHANGE_RATE;
        }

        const costIncurredEl = document.getElementById('statCostIncurredIDR');
        if (costIncurredEl) costIncurredEl.textContent = formatRupiahExact(totalCostIncurredIDR);

        const avgReqCostEl = document.getElementById('statCostAvgPerReq');
        if (avgReqCostEl) {
          const avgPerReq = (sum.totalRequests > 0) ? (totalCostIncurredIDR / sum.totalRequests) : 0;
          avgReqCostEl.textContent = `Rata-rata: ${formatRupiahExact(avgPerReq)} / request`;
        }

        const costSavedEl = document.getElementById('statCostSavedIDR');
        if (costSavedEl) costSavedEl.textContent = '+' + formatRupiahExact(totalCostSavedIDR);

        const costInputEl = document.getElementById('statCostInputTokensIDR');
        if (costInputEl) costInputEl.textContent = formatRupiahExact(totalInputCostIDR);

        const costCachedEl = document.getElementById('statCostCachedTokensIDR');
        if (costCachedEl) costCachedEl.textContent = formatRupiahExact(totalCachedCostIDR);

        const costOutputEl = document.getElementById('statCostOutputTokensIDR');
        if (costOutputEl) costOutputEl.textContent = formatRupiahExact(totalOutputCostIDR);

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
            byModelBody.innerHTML = '<tr><td colspan="9" class="text-center py-6 text-slate-400">Belum ada data untuk periode ini</td></tr>';
          } else {
            byModelBody.innerHTML = byModelList.map(m => {
              const cost = calculateTokensCostIDR(m.model, m.inputTokens || 0, m.cachedTokens || 0, m.actualOutput || 0);
              const cfg = getModelPricingConfig(m.model);
              const savedIDR = ((m.savedOutput || 0) / 1000000) * cfg.outputPerMillion * USD_IDR_EXCHANGE_RATE;
              return `
                <tr class="border-b border-slate-100 hover:bg-slate-50/60 transition-colors">
                  <td class="p-2.5 font-bold font-mono text-slate-900">${escapeHtml(m.model)}</td>
                  <td class="p-2.5 font-semibold text-slate-800">${m.requests}</td>
                  <td class="p-2.5 font-mono font-semibold text-purple-600">${m.inputTokens.toLocaleString()}</td>
                  <td class="p-2.5 font-mono font-semibold text-amber-500">${m.cachedTokens.toLocaleString()}</td>
                  <td class="p-2.5 font-mono font-bold text-blue-600">${m.actualOutput.toLocaleString()}</td>
                  <td class="p-2.5 font-mono font-medium text-slate-400">${m.estimatedRaw.toLocaleString()}</td>
                  <td class="p-2.5 font-mono font-bold text-emerald-600">+${m.savedOutput.toLocaleString()} (${(m.savedPercentage || 0).toFixed(0)}%)</td>
                  <td class="p-2.5 font-mono font-bold text-indigo-700">${formatRupiahExact(cost.totalCostIDR)}</td>
                  <td class="p-2.5 font-mono font-bold text-emerald-700">+${formatRupiahExact(savedIDR)}</td>
                </tr>
              `;
            }).join('');
          }
        }

        // 4. Render Recent Transactions Table
        const recentBody = document.getElementById('analyticsRecentLogsBody');
        if (recentBody) {
          const logs = (data.recentLogs || data.recentTransactions || []).slice(0, 10);
          if (logs.length === 0) {
            recentBody.innerHTML = '<tr><td colspan="9" class="p-4 text-center text-slate-400">Belum ada log transaksi</td></tr>';
          } else {
            recentBody.innerHTML = logs.map(l => {
              const cost = calculateTokensCostIDR(l.model, l.promptTokens || 0, l.cachedTokens || 0, l.actualOutputTokens || 0);
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
                  <td class="p-2 font-mono font-bold text-indigo-700">${formatRupiahExact(cost.totalCostIDR)}</td>
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
      fetchProjects();
      fetchInspectorLogs();
      fetchProviderStrategy();
      fetchProviderDiagnostics();
      initUsageAutoRefresh();
      initInspectorAutoRefresh();
      fetchFallbacks();

      // 3. Initialize playground sessions and resolve initial active tab from URL hash
      loadPlaygroundSessions();
      syncPlaygroundHistoryWithSession();
      handleHashChange();
    }

    // ==========================================
    // DIAGNOSTICS & SYSTEM LATENCY MODULE
    // ==========================================
    async function fetchDiagnostics(runProbe = false) {
      const tableBody = document.getElementById('diagAccountsTableBody');
      const btn = document.getElementById('btnRunDiagnostics');
      if (btn && runProbe) {
        btn.disabled = true;
        btn.innerHTML = `<svg class="w-3.5 h-3.5 animate-spin"><use href="#icon-refresh-cw"></use></svg><span>Menguji Latensi...</span>`;
      }

      try {
        const res = await fetch('/api/diagnostics');
        const data = await res.json();

        // 1. Render Gateway Health
        const gw = data.gateway || {};
        const statusEl = document.getElementById('diagGatewayStatus');
        if (statusEl) statusEl.textContent = gw.status || 'Healthy';
        const checkedAtEl = document.getElementById('diagCheckedAt');
        if (checkedAtEl) checkedAtEl.textContent = 'Terakhir dicek: ' + (gw.checkedAt || '-');
        const memEl = document.getElementById('diagMemoryAlloc');
        if (memEl) memEl.textContent = (gw.memoryAllocMB ? gw.memoryAllocMB.toFixed(2) : '0') + ' MB';
        const goroutinesEl = document.getElementById('diagGoroutines');
        if (goroutinesEl) goroutinesEl.textContent = (gw.goroutines || 0) + ' Goroutines aktif';

        // 2. Render Circuit Breakers
        const cb = data.circuitBreaker || {};
        const gemEl = document.getElementById('cbGeminiStatus');
        if (gemEl) gemEl.textContent = cb['gemini-3.8-flash-tiered'] || 'Closed (Sehat)';
        const sonnetEl = document.getElementById('cbSonnetStatus');
        if (sonnetEl) sonnetEl.textContent = cb['claude-sonnet-4-6'] || 'Closed (Sehat)';
        const opusEl = document.getElementById('cbOpusStatus');
        if (opusEl) opusEl.textContent = cb['claude-opus-4-6-thinking'] || 'Closed (Sehat)';

        // 3. Render Accounts Table
        if (tableBody) {
          const accounts = data.accounts || [];
          if (accounts.length === 0) {
            tableBody.innerHTML = '<tr><td colspan="8" class="text-center py-6 text-slate-400">Tidak ada akun Google terhubung</td></tr>';
          } else {
            tableBody.innerHTML = accounts.map(a => {
              let latencyBadge = '';
              if (a.latencyMs <= 0) {
                latencyBadge = '<span class="text-slate-400 font-mono">Timeout / N/A</span>';
              } else if (a.latencyMs < 350) {
                latencyBadge = `<span class="inline-flex items-center gap-1 font-mono font-bold text-emerald-700 bg-emerald-50 px-2 py-0.5 rounded border border-emerald-200/60">${a.latencyMs} ms</span>`;
              } else if (a.latencyMs < 750) {
                latencyBadge = `<span class="inline-flex items-center gap-1 font-mono font-bold text-amber-700 bg-amber-50 px-2 py-0.5 rounded border border-amber-200/60">${a.latencyMs} ms</span>`;
              } else {
                latencyBadge = `<span class="inline-flex items-center gap-1 font-mono font-bold text-rose-700 bg-rose-50 px-2 py-0.5 rounded border border-rose-200/60">${a.latencyMs} ms</span>`;
              }

              let statusBadge = '<span class="badge badge-success text-[10px]">Active</span>';
              if (a.status === 'Exhausted') {
                statusBadge = '<span class="badge badge-danger text-[10px]">Exhausted</span>';
              } else if (a.status === 'Degraded') {
                statusBadge = '<span class="badge badge-warning text-[10px]">Degraded</span>';
              }

              return `
                <tr class="border-b border-slate-100 hover:bg-slate-50/60 transition-colors">
                  <td class="p-3 font-semibold text-slate-900">${escapeHtml(a.email)}</td>
                  <td class="p-3 font-mono text-slate-500">${escapeHtml(a.projectId || '-')}</td>
                  <td class="p-3"><span class="badge badge-purple text-[10px]">${escapeHtml(a.tier || 'Standard')}</span></td>
                  <td class="p-3">${statusBadge}</td>
                  <td class="p-3">${latencyBadge}</td>
                  <td class="p-3 font-mono font-semibold text-indigo-600">${a.quota5hPercent ? a.quota5hPercent.toFixed(1) + '%' : '-'}</td>
                  <td class="p-3 font-mono font-semibold text-emerald-600">${a.quotaWeeklyPercent ? a.quotaWeeklyPercent.toFixed(1) + '%' : '-'}</td>
                  <td class="p-3 text-slate-400 font-mono text-[11px]">${escapeHtml(a.lastChecked || '-')}</td>
                </tr>
              `;
            }).join('');
          }
        }
      } catch (err) {
        console.error('fetchDiagnostics error:', err);
        if (tableBody) {
          tableBody.innerHTML = `<tr><td colspan="8" class="text-center py-6 text-rose-500">Gagal memuat diagnostik: ${escapeHtml(err.message)}</td></tr>`;
        }
      } finally {
        if (btn && runProbe) {
          btn.disabled = false;
          btn.innerHTML = `<svg class="w-3.5 h-3.5"><use href="#icon-refresh-cw"></use></svg><span>Jalankan Probe Ulang</span>`;
        }
      }
    }

    // ==========================================
    // SKILLS & PROMPT RULES MANAGER MODULE
    // ==========================================
    async function fetchSkillsManager() {
      const grid = document.getElementById('skillsManagerGrid');
      const counter = document.getElementById('skillsActiveCounter');
      if (!grid) return;

      try {
        const res = await fetch('/api/skills');
        const data = await res.json();
        const skillsList = data.skills || [];

        const activeCount = skillsList.filter(s => s.enabled).length;
        if (counter) counter.textContent = `${activeCount} dari ${skillsList.length} Skills Aktif`;

        if (skillsList.length === 0) {
          grid.innerHTML = '<div class="p-8 text-center text-slate-400 col-span-full">Belum ada skills terdaftar.</div>';
          return;
        }

        grid.innerHTML = skillsList.map(s => {
          const isBuiltin = s.isBuiltin ?? s.builtin;
          const promptText = s.systemPrompt || s.promptTemplate || '';
          const badgeType = isBuiltin
            ? '<span class="badge badge-purple text-[10px]">Built-in Saver</span>'
            : '<span class="badge badge-blue text-[10px]">Custom Rule</span>';

          const estReduction = s.id === 'caveman' ? '~80% Token Saver' :
                               s.id === 'rtk' ? '~40% Token Saver' :
                               s.id === 'codegraph' ? '~50% Token Saver' :
                               s.id === 'context_mode' ? '~60% Token Saver' : '~70% Token Saver';

          return `
            <div class="rounded-2xl border ${s.enabled ? 'border-purple-200/90 bg-white' : 'border-slate-200/70 bg-slate-50/70'} p-5 shadow-2xs flex flex-col justify-between transition-all">
              <div>
                <div class="flex items-center justify-between gap-2 mb-2.5">
                  <div class="flex items-center gap-2">
                    <span class="w-2.5 h-2.5 rounded-full ${s.enabled ? 'bg-purple-600' : 'bg-slate-300'}"></span>
                    <h3 class="font-extrabold text-sm text-slate-900">${escapeHtml(s.name)}</h3>
                  </div>
                  <label class="relative inline-flex items-center cursor-pointer">
                    <input type="checkbox" class="sr-only peer" ${s.enabled ? 'checked' : ''} onchange="toggleSkillFromManager('${escapeHtml(s.id)}')">
                    <div class="w-8 h-4.5 bg-slate-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-3.5 after:w-3.5 after:transition-all peer-checked:bg-purple-600"></div>
                  </label>
                </div>

                <div class="flex items-center gap-2 mb-3">
                  <span class="font-mono text-[10.5px] text-slate-400 bg-slate-100 px-1.5 py-0.5 rounded">id: ${escapeHtml(s.id)}</span>
                  ${badgeType}
                  <span class="text-[10.5px] font-bold text-emerald-600 font-mono">${estReduction}</span>
                </div>

                <p class="text-xs text-slate-600 leading-relaxed mb-4">${escapeHtml(s.description || 'Tidak ada deskripsi.')}</p>

                <!-- Prompt Template Collapsible -->
                <details class="text-[11px] bg-slate-50 border border-slate-200/70 rounded-xl overflow-hidden mb-3 group" open>
                  <summary class="px-3 py-2 font-bold text-slate-700 cursor-pointer select-none bg-slate-100/70 hover:bg-slate-100 transition-colors flex items-center justify-between">
                    <span class="flex items-center gap-1.5">
                      <svg class="w-3.5 h-3.5 text-purple-600"><use href="#icon-zap"></use></svg>
                      Injeksi System Prompt
                    </span>
                    <span class="text-[10px] text-purple-600 font-mono bg-purple-50 px-2 py-0.5 rounded border border-purple-200/60">Template Aktif</span>
                  </summary>
                  <pre class="p-3 text-[10.5px] text-slate-700 font-mono whitespace-pre-wrap max-h-48 overflow-y-auto leading-relaxed border-t border-slate-200/60 bg-white">${escapeHtml(promptText || '(Prompt kosong)')}</pre>
                </details>
              </div>

              ${!isBuiltin ? `
                <div class="pt-3 border-t border-slate-100 flex justify-end">
                  <button class="text-xs text-rose-500 hover:text-rose-700 font-semibold flex items-center gap-1" onclick="deleteSkillFromManager('${escapeHtml(s.id)}')">
                    <svg class="w-3.5 h-3.5"><use href="#icon-trash-2"></use></svg> Hapus Skill
                  </button>
                </div>
              ` : ''}
            </div>
          `;
        }).join('');
      } catch (err) {
        console.error('fetchSkillsManager error:', err);
        grid.innerHTML = `<div class="p-8 text-center text-rose-500 col-span-full">Gagal memuat skills: ${escapeHtml(err.message)}</div>`;
      }
    }

    async function toggleSkillFromManager(skillId) {
      try {
        const res = await fetch(`/api/skills/toggle?id=${encodeURIComponent(skillId)}`, { method: 'POST' });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        await fetchSkillsManager();
        await fetchSkills(); // Sinkronkan ke playground & pill skills
        renderComboSkillsPills();
      } catch (err) {
        alert('Gagal mengubah status skill: ' + err.message);
        fetchSkillsManager();
      }
    }

    function openCreateSkillModal() {
      const modal = document.getElementById('modalCreateSkill');
      if (modal) modal.classList.remove('hidden');
    }

    function closeCreateSkillModal() {
      const modal = document.getElementById('modalCreateSkill');
      if (modal) modal.classList.add('hidden');
    }

    async function submitCreateSkill() {
      const id = (document.getElementById('newSkillId')?.value || '').trim();
      const name = (document.getElementById('newSkillName')?.value || '').trim();
      const desc = (document.getElementById('newSkillDesc')?.value || '').trim();
      const template = (document.getElementById('newSkillTemplate')?.value || '').trim();

      if (!id || !name || !template) {
        alert('ID Skill, Nama Tampilan, dan Prompt Template wajib diisi!');
        return;
      }

      try {
        const res = await fetch('/api/skills', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            id: id,
            name: name,
            description: desc,
            systemPrompt: template,
            promptTemplate: template,
            enabled: true,
            isBuiltin: false
          })
        });

        if (!res.ok) throw new Error('Gagal menyimpan skill');
        closeCreateSkillModal();
        // Clear form
        document.getElementById('newSkillId').value = '';
        document.getElementById('newSkillName').value = '';
        document.getElementById('newSkillDesc').value = '';
        document.getElementById('newSkillTemplate').value = '';

        await fetchSkillsManager();
        await fetchSkills();
        renderComboSkillsPills();
      } catch (err) {
        alert('Error: ' + err.message);
      }
    }

    async function resetBuiltinSkills() {
      if (!confirm('Apakah Anda yakin ingin me-reset seluruh skills bawaan ke default?')) return;
      try {
        const res = await fetch('/api/skills?action=reset_builtins', { method: 'POST' });
        if (!res.ok) throw new Error('Gagal me-reset skills');
        await fetchSkillsManager();
        await fetchSkills();
        renderComboSkillsPills();
      } catch (err) {
        alert('Error: ' + err.message);
      }
    }

    async function deleteSkillFromManager(skillId) {
      if (!confirm(`Hapus skill "${skillId}" secara permanen?`)) return;
      try {
        const res = await fetch(`/api/skills?id=${encodeURIComponent(skillId)}`, { method: 'DELETE' });
        if (!res.ok) throw new Error('Gagal menghapus skill');
        await fetchSkillsManager();
        await fetchSkills();
        renderComboSkillsPills();
      } catch (err) {
        alert('Error: ' + err.message);
      }
    }

    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', bootApplication);
    } else {
      bootApplication();
    }

// Explicit global window bindings for inline HTML onclick handlers
window.fetchProjects = fetchProjects;
window.updatePlaygroundProjectsSelect = updatePlaygroundProjectsSelect;
window.updatePgProjectBadge = updatePgProjectBadge;
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
window.formatResetCountdown = formatResetCountdown;
window.handleCreateSkillSubmit = handleCreateSkillSubmit;
window.handleGrillCustomInput = handleGrillCustomInput;
window.handleGrillOptionChange = handleGrillOptionChange;
window.handlePlaygroundAction = handlePlaygroundAction;
window.handleProviderStrategyChange = handleProviderStrategyChange;
window.importFromLocalSystem = importFromLocalSystem;
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
window.createNewPlaygroundSession = createNewPlaygroundSession;
window.selectPlaygroundSession = selectPlaygroundSession;
window.deletePlaygroundSession = deletePlaygroundSession;
window.clearAllPlaygroundSessions = clearAllPlaygroundSessions;
window.renderPlaygroundSessionsList = renderPlaygroundSessionsList;
window.handlePgAccountChange = handlePgAccountChange;
window.handlePgModelChange = handlePgModelChange;
window.handlePgProjectChange = handlePgProjectChange;
window.clearAllComboSkills = clearAllComboSkills;
window.updateActiveSessionHeader = updateActiveSessionHeader;
window.handlePlaygroundKeydown = handlePlaygroundKeydown;
window.autoResizePlaygroundInput = autoResizePlaygroundInput;
window.copyCodeBlock = copyCodeBlock;
window.fetchDiagnostics = fetchDiagnostics;
window.fetchProviderDiagnostics = fetchProviderDiagnostics;
window.fetchSkillsManager = fetchSkillsManager;
window.toggleSkillFromManager = toggleSkillFromManager;
window.openCreateSkillModal = openCreateSkillModal;
window.closeCreateSkillModal = closeCreateSkillModal;
window.submitCreateSkill = submitCreateSkill;
window.resetBuiltinSkills = resetBuiltinSkills;
window.deleteSkillFromManager = deleteSkillFromManager;
