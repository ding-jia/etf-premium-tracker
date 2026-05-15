let autoRefresh = true;
let refreshInterval = null;
let allData = { nasdaq: [], sp500: [] };
let watchlist = new Set();
let chartInstance = null;
let currentChartCode = null;

const API = '/api/etfs';
const HISTORY_API = '/api/history';
const WATCHLIST_API = '/api/watchlist';

const ETFS = [
  { code: "513100", name: "纳指ETF国泰", exchange: "SH", category: "nasdaq", manager: "国泰基金" },
  { code: "159941", name: "纳指ETF广发", exchange: "SZ", category: "nasdaq", manager: "广发基金" },
  { code: "513300", name: "纳斯达克ETF华夏", exchange: "SH", category: "nasdaq", manager: "华夏基金" },
  { code: "159632", name: "纳斯达克ETF华安", exchange: "SZ", category: "nasdaq", manager: "华安基金" },
  { code: "513110", name: "纳指ETF华泰柏瑞", exchange: "SH", category: "nasdaq", manager: "华泰柏瑞基金" },
  { code: "159696", name: "纳指ETF易方达", exchange: "SZ", category: "nasdaq", manager: "易方达基金" },
  { code: "159501", name: "纳指ETF嘉实", exchange: "SZ", category: "nasdaq", manager: "嘉实基金" },
  { code: "159513", name: "纳斯达克100ETF大成", exchange: "SZ", category: "nasdaq", manager: "大成基金" },
  { code: "159659", name: "纳斯达克100ETF招商", exchange: "SZ", category: "nasdaq", manager: "招商基金" },
  { code: "159660", name: "纳指ETF汇添富", exchange: "SZ", category: "nasdaq", manager: "汇添富基金" },
  { code: "513390", name: "纳指100ETF博时", exchange: "SH", category: "nasdaq", manager: "博时基金" },
  { code: "513870", name: "纳指ETF富国", exchange: "SH", category: "nasdaq", manager: "富国基金" },
  { code: "159509", name: "纳指科技ETF景顺", exchange: "SZ", category: "nasdaq", manager: "景顺长城基金" },
  { code: "513290", name: "纳指生物科技ETF汇添富", exchange: "SH", category: "nasdaq", manager: "汇添富基金" },
  { code: "513500", name: "标普500ETF博时", exchange: "SH", category: "sp500", manager: "博时基金" },
  { code: "159655", name: "标普500ETF华夏", exchange: "SZ", category: "sp500", manager: "华夏基金" },
  { code: "159612", name: "标普500ETF国泰", exchange: "SZ", category: "sp500", manager: "国泰基金" },
  { code: "513650", name: "标普500ETF南方", exchange: "SH", category: "sp500", manager: "南方基金" },
  { code: "159502", name: "标普生物科技ETF嘉实", exchange: "SZ", category: "sp500", manager: "嘉实基金" },
  { code: "159518", name: "标普油气ETF嘉实", exchange: "SZ", category: "sp500", manager: "嘉实基金" },
  { code: "159529", name: "标普消费ETF景顺", exchange: "SZ", category: "sp500", manager: "景顺长城基金" },
  { code: "513350", name: "标普油气ETF富国", exchange: "SH", category: "sp500", manager: "富国基金" },
];

function premiumLevel(p) {
  const abs = Math.abs(p);
  if (abs > 5) return 'purple';
  if (p > 0) return 'up';
  if (p < 0) return 'down';
  return 'low';
}

function premiumLabel(p) {
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

function formatVol(v) {
  if (!v) return '--';
  if (v >= 1e8) return (v / 1e8).toFixed(2) + '亿';
  if (v >= 1e4) return (v / 1e4).toFixed(1) + '万';
  return v.toString();
}

function formatAmount(v) {
  if (!v) return '--';
  if (v >= 1e8) return (v / 1e8).toFixed(2) + '亿';
  if (v >= 1e4) return (v / 1e4).toFixed(1) + '万';
  return v.toString();
}

function renderCard(etf) {
  const premium = etf.premium ?? 0;
  const level = premiumLevel(premium);
  const changeClass = etf.change_pct >= 0 ? 'up' : 'down';
  const changeSign = etf.change_pct >= 0 ? '+' : '';
  const isWatchlist = watchlist.has(etf.code) ? ' watchlist' : '';

  return `
    <div class="list-item premium-${level}${isWatchlist}" data-code="${etf.code}" data-premium="${premium}">
      <span class="li-code">${etf.code}</span>
      <span class="li-name">${isWatchlist ? '★ ' : ''}${etf.name}</span>
      <span class="li-manager">${etf.manager}</span>
      <span class="li-price">${etf.price ?? '--'}</span>
      <span class="li-change ${changeClass}">${changeSign}${(etf.change_pct ?? 0).toFixed(2)}%</span>
      <span class="li-premium">${premium >= 0 ? '+' : ''}${premium.toFixed(2)}%</span>
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
      case 'code': return a.code.localeCompare(b.code);
      case 'name': return a.name.localeCompare(b.name);
      default: return 0;
    }
  };

  const pinned = data.filter(e => watchlist.has(e.code)).sort(sortFn);
  const rest = data.filter(e => !watchlist.has(e.code)).sort(sortFn);
  const sorted = [...pinned, ...rest];

  container.innerHTML = sorted.map(etf => renderCard(etf)).join('');

  // click handler for chart modal
  container.querySelectorAll('.list-item').forEach(el => {
    el.addEventListener('click', () => openChart(el.dataset.code));
  });
}

function renderSkeleton(containerId, count) {
  const container = document.getElementById(containerId);
  container.innerHTML = Array(count).fill(0).map(() => `
    <div class="list-item loading">
      <span class="li-code">888888</span>
      <span class="li-name">加载中加载中</span>
      <span class="li-manager">加载中</span>
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

async function fetchData() {
  try {
    const resp = await fetch(API);
    const data = await resp.json();

    allData = { nasdaq: data.nasdaq, sp500: data.sp500 };

    updateMarketStatus(data.market_status);
    document.getElementById('updateTime').textContent = data.update_time;

    renderGrid(data.nasdaq, 'nasdaqGrid', 'nasdaqSort');
    renderGrid(data.sp500, 'sp500Grid', 'sp500Sort');
    updateCounts(data.nasdaq, data.sp500);
  } catch (err) {
    console.error('fetch error:', err);
  }
}

async function openChart(code) {
  currentChartCode = code;
  const overlay = document.getElementById('chartModal');
  const title = document.getElementById('modalTitle');
  const canvas = document.getElementById('premiumChart');

  const etf = ETFS.find(e => e.code === code);
  title.textContent = `${etf ? etf.name : code} (${code}) · 历史溢价率`;

  overlay.classList.add('active');

  try {
    const resp = await fetch(`${HISTORY_API}/${code}`);
    const data = await resp.json();
    renderChart(data.history || []);
  } catch {
    renderChart([]);
  }
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

  const labels = records.map(r => {
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
        borderColor: '#4a8eff',
        backgroundColor: (ctx) => {
          const g = ctx.chart.ctx.createLinearGradient(0, 0, 0, 320);
          g.addColorStop(0, 'rgba(74, 142, 255, 0.2)');
          g.addColorStop(1, 'rgba(74, 142, 255, 0.0)');
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
          backgroundColor: '#ffffff',
          titleColor: '#1f2937',
          bodyColor: '#2563eb',
          borderColor: '#e2e6ea',
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
            color: '#9ca3af',
            font: { size: 10 },
            maxTicksLimit: 10,
            autoSkip: true,
          }
        },
        y: {
          display: true,
          grid: { color: 'rgba(226, 230, 234, 0.6)', drawBorder: false },
          ticks: {
            color: '#9ca3af',
            font: { size: 10 },
            callback: (v) => `${v.toFixed(1)}%`,
          }
        }
      }
    }
  });
}

function closeChart() {
  document.getElementById('chartModal').classList.remove('active');
  if (chartInstance) {
    chartInstance.destroy();
    chartInstance = null;
  }
  currentChartCode = null;
}

function startAutoRefresh() {
  if (refreshInterval) clearInterval(refreshInterval);
  refreshInterval = setInterval(() => {
    if (autoRefresh) fetchData();
  }, 30000);
}

// event listeners
document.addEventListener('DOMContentLoaded', async () => {
  renderSkeleton('nasdaqGrid', 7);
  renderSkeleton('sp500Grid', 3);

  try {
    const wlResp = await fetch(WATCHLIST_API);
    const wlData = await wlResp.json();
    watchlist = new Set(wlData.codes);
  } catch (err) {
    console.error('watchlist fetch error:', err);
  }

  fetchData();
  startAutoRefresh();

  document.getElementById('refreshBtn').addEventListener('click', () => {
    const btn = document.getElementById('refreshBtn');
    btn.classList.add('spin');
    fetchData().finally(() => {
      setTimeout(() => btn.classList.remove('spin'), 600);
    });
  });

  document.getElementById('toggleAuto').addEventListener('click', (e) => {
    e.preventDefault();
    autoRefresh = !autoRefresh;
    document.getElementById('toggleAuto').textContent = autoRefresh ? '暂停' : '开启';
    document.getElementById('toggleAuto').style.color = autoRefresh ? '' : 'var(--yellow)';
  });

  document.getElementById('modalClose').addEventListener('click', closeChart);
  document.getElementById('chartModal').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) closeChart();
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeChart();
  });

  document.getElementById('nasdaqSort').addEventListener('change', () => {
    renderGrid(allData.nasdaq, 'nasdaqGrid', 'nasdaqSort');
  });
  document.getElementById('sp500Sort').addEventListener('change', () => {
    renderGrid(allData.sp500, 'sp500Grid', 'sp500Sort');
  });
});
