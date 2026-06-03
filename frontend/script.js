let theme = localStorage.getItem('theme') || 'light';

function applyTheme() {
  document.documentElement.setAttribute('data-theme', theme);
  const btn = document.getElementById('themeToggle');
  if (btn) btn.textContent = theme === 'bloomberg' ? 'LIT' : 'BBG';
}

function toggleTheme() {
  theme = theme === 'bloomberg' ? 'light' : 'bloomberg';
  localStorage.setItem('theme', theme);
  applyTheme();
}

let autoRefresh = true;
let refreshInterval = null;
let statusInterval = null;
let allData = { nasdaq: [], sp500: [] };
let isMarketOpen = false;
let watchlist = new Set();
let chartInstance = null;
let currentChartCode = null;

const API = '/api/etfs';
const HISTORY_API = '/api/daily';
const WATCHLIST_API = '/api/watchlist';

function handleError(ctx, err, msg) {
  console.error(ctx, err);
  if (msg) showToast(msg);
}

function showToast(msg, duration = 3000) {
  let container = document.getElementById('toastContainer');
  if (!container) {
    container = document.createElement('div');
    container.id = 'toastContainer';
    container.style.cssText = 'position:fixed;top:70px;right:20px;z-index:9999;display:flex;flex-direction:column;gap:8px;pointer-events:none;';
    document.body.appendChild(container);
  }
  const toast = document.createElement('div');
  toast.textContent = msg;
  toast.style.cssText = 'padding:10px 18px;border-radius:6px;background:#dc2626;color:#fff;font-size:0.85rem;box-shadow:0 4px 12px rgba(0,0,0,0.15);opacity:0;transition:opacity 0.3s;pointer-events:auto;';
  container.appendChild(toast);
  requestAnimationFrame(() => toast.style.opacity = '1');
  setTimeout(() => {
    toast.style.opacity = '0';
    setTimeout(() => toast.remove(), 300);
  }, duration);
}

function premiumLevel(p) {
  if (p == null) return 'low';
  const abs = Math.abs(p);
  if (abs > 5) return 'purple';
  if (p > 0) return 'up';
  if (p < 0) return 'down';
  return 'low';
}

function premiumLabel(p) {
  if (p == null) return 'N/A';
  const abs = Math.abs(p);
  if (p < 0) {
    if (abs <= 1) return '正常';
    if (abs <= 3) return '折价';
    if (abs <= 5) return '高折价';
    return '深度折价';
  }
  if (abs <= 1) return '正常';
  if (abs <= 3) return '溢价';
  if (abs <= 5) return '高溢价';
  return '极高';
}

function escapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function renderCard(etf, minFee) {
  const premium = etf.premium ?? 0;
  const level = premiumLevel(premium);
  const changeClass = etf.change_pct >= 0 ? 'up' : 'down';
  const changeSign = etf.change_pct >= 0 ? '+' : '';
  const fee = etf.fee;
  const feeTotal = fee && fee.total != null ? fee.total : null;
  const feeText = feeTotal != null ? feeTotal.toFixed(2) + '%' : '--';
  const isWL = watchlist.has(etf.code);
  const isBestFee = feeTotal != null && minFee != null && feeTotal === minFee;
  return `
    <div class="list-item premium-${level}${isWL ? ' watchlist' : ''}${isBestFee ? ' best-fee' : ''}" data-code="${etf.code}" data-premium="${premium}">
      <span class="li-star" data-code="${etf.code}">${isWL ? '★' : '☆'}</span>
      <span class="li-code">${etf.code}</span>
      <span class="li-name">${escapeHtml(etf.name)}</span>
      <span class="li-manager">${escapeHtml(etf.manager)}</span>
      <span class="li-fee">${feeText}</span>
      <span class="li-price">${etf.price ?? '--'}</span>
      <span class="li-change ${changeClass}">${changeSign}${(etf.change_pct ?? 0).toFixed(2)}%</span>
      <span class="li-premium">${premium != null ? (premium >= 0 ? '+' : '') + premium.toFixed(2) + '%' : 'N/A'}</span>
      <span class="li-label">${premiumLabel(premium)}</span>
    </div>
  `;
}

function renderGrid(data, containerId, sortSelectId) {
  const container = document.getElementById(containerId);
  const sortBy = document.getElementById(sortSelectId).value;

  const sortFn = (a, b) => {
    switch (sortBy) {
      case 'premium-desc': return (b.premium ?? 0) - (a.premium ?? 0);
      case 'premium-asc': return (a.premium ?? 0) - (b.premium ?? 0);
      case 'fee-desc': return ((b.fee && b.fee.total) || 0) - ((a.fee && a.fee.total) || 0);
      case 'fee-asc': return ((a.fee && a.fee.total) || 0) - ((b.fee && b.fee.total) || 0);
      case 'code': return a.code.localeCompare(b.code);
      case 'name': return a.name.localeCompare(b.name);
      default: return 0;
    }
  };

  const minFee = Math.min(...data.map(e => (e.fee && e.fee.total) || Infinity));

  const pinned = data.filter(e => watchlist.has(e.code)).sort(sortFn);
  const rest = data.filter(e => !watchlist.has(e.code)).sort(sortFn);
  const sorted = [...pinned, ...rest];

  container.innerHTML = sorted.map(etf => renderCard(etf, minFee)).join('');

  // star toggle
  container.querySelectorAll('.li-star').forEach(el => {
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      toggleWatchlist(el.dataset.code);
    });
  });

  // click handler for chart modal
  container.querySelectorAll('.list-item').forEach(el => {
    el.addEventListener('click', (e) => {
      if (e.target.classList.contains('li-star')) return;
      openChart(el.dataset.code);
    });
  });
}

function renderSkeleton(containerId, count) {
  const container = document.getElementById(containerId);
  container.innerHTML = Array(count).fill(0).map(() => `
    <div class="list-item loading">
      <span class="li-star">☆</span>
      <span class="li-code">888888</span>
      <span class="li-name">加载中加载中</span>
      <span class="li-manager">加载中</span>
      <span class="li-fee">0.00%</span>
      <span class="li-price">88.888</span>
      <span class="li-change">+88.88%</span>
      <span class="li-premium">+88.88%</span>
      <span class="li-label">加载</span>
    </div>
  `).join('');
}

function updateMarketStatus(status) {
  const badge = document.getElementById('statusBadge');
  const text = document.getElementById('statusText');
  if (status === 'open') {
    badge.className = 'status-badge';
    text.textContent = '交易中';
  } else {
    badge.className = 'status-badge closed';
    text.textContent = '已收盘';
  }
}

function updateCounts(nasdaq, sp500) {
  document.getElementById('nasdaqCount').textContent = `${nasdaq.length} 只`;
  document.getElementById('sp500Count').textContent = `${sp500.length} 只`;
}

async function fetchData(forceRender = false) {
  try {
    const resp = await fetch(API);
    const data = await resp.json();

    allData = { nasdaq: data.nasdaq, sp500: data.sp500 };
    isMarketOpen = data.market_status === 'open';

    updateMarketStatus(data.market_status);
    document.getElementById('updateTime').textContent = data.update_time;

    if (forceRender || isMarketOpen) {
      renderGrid(data.nasdaq, 'nasdaqGrid', 'nasdaqSort');
      renderGrid(data.sp500, 'sp500Grid', 'sp500Sort');
      updateCounts(data.nasdaq, data.sp500);
    }
  } catch (err) {
    handleError('fetchData', err, '数据加载失败，请检查网络连接');
  }
}

async function toggleWatchlist(code) {
  try {
    const resp = await fetch(`/api/watchlist/toggle/${code}`, { method: 'POST' });
    const data = await resp.json();
    watchlist = new Set(data.codes);
    // re-render with updated watchlist
    renderGrid(allData.nasdaq, 'nasdaqGrid', 'nasdaqSort');
    renderGrid(allData.sp500, 'sp500Grid', 'sp500Sort');
  } catch (err) {
    handleError('toggleWatchlist', err, '操作失败');
  }
}

async function selectETF(code) {
  currentChartCode = code;
  const title = document.getElementById('chartTitle');
  const allEtfs = [...(allData.nasdaq || []), ...(allData.sp500 || [])];
  const etf = allEtfs.find(e => e.code === code);
  title.textContent = `${etf ? etf.name : code} (${code})`;

  // highlight selected row
  document.querySelectorAll('.list-item.selected').forEach(el => el.classList.remove('selected'));
  const row = document.querySelector(`.list-item[data-code="${code}"]`);
  if (row) row.classList.add('selected');

  try {
    const resp = await fetch(`${HISTORY_API}/${code}`);
    const data = await resp.json();
    renderChart(data.daily || []);
  } catch (err) {
    handleError('selectETF', err, '历史数据加载失败');
    renderChart([]);
  }
}

async function openChart(code) {
  selectETF(code);
}

function renderChart(records) {
  const canvas = document.getElementById('premiumChart');
  const ctx = canvas.getContext('2d');

  if (chartInstance) {
    chartInstance.destroy();
  }

  if (!records.length) {
    chartInstance = null;
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    return;
  }

  const isBB = theme === 'bloomberg';
  const lineColor = isBB ? '#ff8c00' : '#2563eb';
  const fillRgba = isBB ? 'rgba(255,140,0,' : 'rgba(37,99,235,';
  const textColor = isBB ? '#787878' : '#9ca3af';
  const gridColor = isBB ? 'rgba(42,42,42,0.6)' : 'rgba(226,230,234,0.6)';

  const labels = records.map(r => {
    if (typeof r[0] === 'string') {
      const parts = r[0].split('-');
      return parts.length >= 3 ? `${parts[1]}-${parts[2]}` : r[0];
    }
    const d = new Date(r[0] * 1000);
    return `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`;
  });
  const values = records.map(r => +r[1].toFixed(2));

  chartInstance = new Chart(ctx, {
    type: 'line',
    data: {
      labels,
      datasets: [{
        label: '溢价率 %',
        data: values,
        borderColor: lineColor,
        backgroundColor: (ctx) => {
          const g = ctx.chart.ctx.createLinearGradient(0, 0, 0, 320);
          g.addColorStop(0, fillRgba + '0.25)');
          g.addColorStop(1, fillRgba + '0.0)');
          return g;
        },
        borderWidth: 2,
        pointRadius: 0,
        pointHitRadius: 6,
        tension: 0.3,
        fill: true,
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: { duration: 300 },
      interaction: {
        intersect: false,
        mode: 'index',
      },
      plugins: {
        legend: { display: false },
        tooltip: {
          backgroundColor: isBB ? '#1a1a1a' : '#ffffff',
          titleColor: isBB ? '#c8c8c8' : '#1f2937',
          bodyColor: isBB ? '#ff8c00' : '#2563eb',
          borderColor: isBB ? '#333' : '#e2e6ea',
          borderWidth: 1,
          padding: 10,
          bodyFont: { size: 14, weight: 'bold' },
          displayColors: false,
          callbacks: {
            label: (ctx) => `${ctx.parsed.y.toFixed(2)}%`,
          }
        }
      },
      scales: {
        x: {
          display: true,
          grid: { display: false, drawBorder: false },
          ticks: {
            color: textColor,
            font: { size: 10 },
            maxTicksLimit: 10,
            autoSkip: true,
          }
        },
        y: {
          display: true,
          grid: { color: gridColor, drawBorder: false },
          ticks: {
            color: textColor,
            font: { size: 10 },
            callback: (v) => `${v.toFixed(1)}%`,
          }
        }
      }
    }
  });
}

function closeChart() {
  if (chartInstance) {
    chartInstance.destroy();
    chartInstance = null;
  }
  currentChartCode = null;
}

function startAutoRefresh() {
  if (refreshInterval) clearInterval(refreshInterval);
  refreshInterval = setInterval(fetchData, 600000);
}

function stopAutoRefresh() {
  if (refreshInterval) {
    clearInterval(refreshInterval);
    refreshInterval = null;
  }
}

async function checkMarketStatus() {
  try {
    const resp = await fetch(API);
    const data = await resp.json();
    isMarketOpen = data.market_status === 'open';
    updateMarketStatus(data.market_status);
    document.getElementById('updateTime').textContent = data.update_time;
    if (isMarketOpen) {
      if (!refreshInterval) {
        startAutoRefresh();
        fetchData();
      }
    } else {
      stopAutoRefresh();
    }
  } catch (err) {
    handleError('checkMarketStatus', err, null);
  }
}

window.addEventListener('beforeunload', () => {
  if (statusInterval) clearInterval(statusInterval);
  if (refreshInterval) clearInterval(refreshInterval);
  if (chartInstance) chartInstance.destroy();
});

// event listeners
document.addEventListener('DOMContentLoaded', async () => {
  applyTheme();
  document.getElementById('themeToggle').addEventListener('click', toggleTheme);

  renderSkeleton('nasdaqGrid', 7);
  renderSkeleton('sp500Grid', 3);

  try {
    const wlResp = await fetch(WATCHLIST_API);
    const wlData = await wlResp.json();
    watchlist = new Set(wlData.codes);
  } catch (err) {
    handleError('watchlistInit', err, null);
  }

  await fetchData(true);
  selectETF('513500');
  checkMarketStatus();
  statusInterval = setInterval(checkMarketStatus, 60000);

  document.getElementById('refreshBtn').addEventListener('click', () => {
    const btn = document.getElementById('refreshBtn');
    btn.classList.add('spin');
    fetchData(true).finally(() => {
      setTimeout(() => btn.classList.remove('spin'), 600);
    });
  });

  document.getElementById('toggleAuto').addEventListener('click', (e) => {
    e.preventDefault();
    autoRefresh = !autoRefresh;
    document.getElementById('toggleAuto').textContent = autoRefresh ? '暂停' : '开启';
    document.getElementById('toggleAuto').style.color = autoRefresh ? '' : 'var(--yellow)';
    if (autoRefresh && isMarketOpen && !refreshInterval) {
      startAutoRefresh();
      fetchData(true);
    } else if (!autoRefresh) {
      stopAutoRefresh();
    }
  });

  document.getElementById('nasdaqSort').addEventListener('change', () => {
    renderGrid(allData.nasdaq, 'nasdaqGrid', 'nasdaqSort');
  });
  document.getElementById('sp500Sort').addEventListener('change', () => {
    renderGrid(allData.sp500, 'sp500Grid', 'sp500Sort');
  });
});
