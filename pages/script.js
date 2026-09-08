// ===== 静态数据 =====
const META = [
  ["513100","纳指ETF国泰","nasdaq","国泰基金","SH"],["159941","纳指ETF广发","nasdaq","广发基金","SZ"],
  ["513300","纳斯达克ETF华夏","nasdaq","华夏基金","SH"],["159632","纳斯达克ETF华安","nasdaq","华安基金","SZ"],
  ["513110","纳指ETF华泰柏瑞","nasdaq","华泰柏瑞基金","SH"],["159696","纳指ETF易方达","nasdaq","易方达基金","SZ"],
  ["159501","纳指ETF嘉实","nasdaq","嘉实基金","SZ"],["159513","纳斯达克100ETF大成","nasdaq","大成基金","SZ"],
  ["159659","纳斯达克100ETF招商","nasdaq","招商基金","SZ"],["159660","纳指ETF汇添富","nasdaq","汇添富基金","SZ"],
  ["513390","纳指100ETF博时","nasdaq","博时基金","SH"],["513870","纳指ETF富国","nasdaq","富国基金","SH"],
  ["159509","纳指科技ETF景顺","nasdaq","景顺长城基金","SZ"],["513500","标普500ETF博时","sp500","博时基金","SH"],
  ["159655","标普500ETF华夏","sp500","华夏基金","SZ"],["159612","标普500ETF国泰","sp500","国泰基金","SZ"],
  ["513650","标普500ETF南方","sp500","南方基金","SH"]
];
const FEES = {
  "513100":0.80,"159941":1.00,"513300":0.80,"159632":0.80,"513110":1.00,"159696":0.60,
  "159501":0.60,"159513":1.00,"159659":0.65,"159660":0.65,"513390":0.65,"513870":0.60,
  "159509":1.00,"513500":0.80,"159655":0.75,"159612":0.75,"513650":0.75
};
const SORT_FN = {
  "premium-desc":(a,b)=>(b.premium||0)-(a.premium||0),"premium-asc":(a,b)=>(a.premium||0)-(b.premium||0),
  "fee-desc":(a,b)=>(FEES[b.code]||0)-(FEES[a.code]||0),"fee-asc":(a,b)=>(FEES[a.code]||0)-(FEES[b.code]||0),
  "amount-desc":(a,b)=>(b.amount||0)-(a.amount||0),"amount-asc":(a,b)=>(a.amount||0)-(b.amount||0),
  "scale-desc":(a,b)=>(b.fund_scale||0)-(a.fund_scale||0),"scale-asc":(a,b)=>(a.fund_scale||0)-(b.fund_scale||0)
};

// ===== 状态 =====
let D = {nasdaq:[],sp500:[]}, WL = new Set(), chart = null, lastRec = [];
let theme = localStorage.getItem("theme")||"light";
let ma = {ma5:true,ma10:true,ma20:true};

// ===== 工具函数 =====
const $ = s => document.getElementById(s);
const esc = s => s==null?"":String(s).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;");
const fmtAmt = v => v==null||v===0?"--":v/1e8>=1?(v/1e8).toFixed(2)+"亿":(v/1e4).toFixed(0)+"万";
const fmtScale = v => v==null?"--":v.toFixed(1)+"亿";
const premiumLevel = p => p==null?"low":Math.abs(p)>5?"purple":p>0?"up":p<0?"down":"low";
const premiumLabel = p => {
  if(p==null)return"N/A";
  const a=Math.abs(p);
  if(p<0)return a<=1?"正常":a<=3?"折价":a<=5?"高折价":"深度折价";
  return a<=1?"正常":a<=3?"溢价":a<=5?"高溢价":"极高";
};

// ===== 数据获取：腾讯HTTPS <script>标签注入（HTTPS无混合内容问题） =====
function fetchQuotes() {
  return new Promise((resolve, reject) => {
    const codes = META.map(m => (m[4]==="SH"?"sh":"sz")+m[0]).join(",");
    const s = document.createElement("script");
    s.src = "https://qt.gtimg.cn/q=" + codes;
    s.onload = () => {
      const out = [];
      for (const m of META) {
        const raw = window["v_"+m[4].toLowerCase()+m[0]];
        if (!raw) continue;
        const p = raw.split("~");
        if (p.length < 82) continue;
        const price = +p[3]||0, iopv = +p[78]||0, nav = +p[81]||0, shares = +p[72]||0;
        if (!price) continue;
        out.push({
          code:m[0], name:m[1], category:m[2], manager:m[3], exchange:m[4],
          price, change_pct:+p[32]||0, volume:(+p[6]||0)*100,
          amount:(+p[37]||0)*1e4, premium:iopv?+p[77]:null,
          iopv:iopv||null, nav:nav||null,
          fund_scale:nav&&shares?+(nav*shares/1e8).toFixed(2):null,
          fee:FEES[m[0]]||null
        });
      }
      resolve(out);
    };
    s.onerror = () => reject(Error("行情加载失败"));
    document.head.appendChild(s);
    setTimeout(() => { try{document.head.removeChild(s)}catch(e){} }, 5000);
  });
}

// ===== 历史K线：腾讯JSON API =====
async function fetchDaily(code) {
  const m = META.find(x=>x[0]===code);
  const pre = m?(m[4]==="SH"?"sh":"sz"):"sh";
  try {
    const r = await fetch(`https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param=${pre}${code},day,,,250,qfq`);
    const j = await r.json();
    return (j.data?.[pre+code]?.qfqday||[]).map(k=>[k[0],+k[2]]);
  } catch(e) { return []; }
}

// ===== 持久化 =====
const loadWL = () => { try{return new Set(JSON.parse(localStorage.getItem("etf_wl")||"[]"))}catch(e){return new Set()} };
const saveWL = () => localStorage.setItem("etf_wl",JSON.stringify([...WL]));
const saveSnap = (code,premium) => {
  if(premium==null) return;
  const k="etf_h_"+code, h=JSON.parse(localStorage.getItem(k)||"[]");
  const today = new Date().toLocaleDateString("sv-SE");
  const last = h[h.length-1];
  if(last&&last[0]===today) last[1]=premium; else h.push([today,premium]);
  if(h.length>180) h.splice(0,h.length-180);
  localStorage.setItem(k,JSON.stringify(h));
};

// ===== 渲染 =====
function renderGrid(data, gridId, sortId) {
  const grid = $(gridId);
  const sort = $(sortId).value;
  const fn = SORT_FN[sort]||((a,b)=>0);
  const pinned = data.filter(e=>WL.has(e.code)).sort(fn);
  const rest = data.filter(e=>!WL.has(e.code)).sort(fn);
  grid.innerHTML = [...pinned,...rest].map(e => {
    const lv=premiumLevel(e.premium), isWL=WL.has(e.code);
    return `<div class="list-item premium-${lv}${isWL?" watchlist":""}" data-code="${e.code}">
<span class="li-star" data-code="${e.code}">${isWL?"★":"☆"}</span>
<span>${e.code}</span>
<span>${esc(e.manager)}</span>
<span>${e.price??"--"}</span>
<span class="${e.change_pct>=0?"up":"down"}">${e.change_pct>=0?"+":""}${e.change_pct.toFixed(2)}%</span>
<span class="li-premium">${e.premium!=null?(e.premium>=0?"+":"")+e.premium.toFixed(2)+"%":"N/A"}</span>
<span>${premiumLabel(e.premium)}</span>
<span>${fmtAmt(e.amount)}</span><span>${fmtScale(e.fund_scale)}</span></div>`;
  }).join("");
  // 事件委托：star点击 → 切换置顶，其他区域点击 → 选中看图表
  grid.onclick = e => {
    const star = e.target.closest(".li-star");
    if (star) { toggleWL(star.dataset.code); return; }
    const item = e.target.closest(".list-item");
    if (item) select(item.dataset.code);
  };
}

function renderAll() {
  renderGrid(D.nasdaq,"nasdaqGrid","nasdaqSort");
  renderGrid(D.sp500,"sp500Grid","sp500Sort");
  $("nasdaqCount").textContent=D.nasdaq.length+"只";
  $("sp500Count").textContent=D.sp500.length+"只";
}

// ===== 刷新 =====
async function refresh() {
  const btn=$("refreshBtn");
  btn.classList.add("spin");
  try {
    const data = await fetchQuotes();
    if(!data.length) throw Error("上游返回空数据");
    const now=new Date(), status=isTrading(now)?"open":"closed";
    D={nasdaq:data.filter(e=>e.category==="nasdaq"),sp500:data.filter(e=>e.category==="sp500")};
    // 存储每只ETF的溢价率快照
    data.forEach(e=>saveSnap(e.code,e.premium));
    $("statusBadge").className="status-badge"+(status==="closed"?" closed":"");
    $("statusText").textContent=status==="open"?"交易中":"已收盘";
    $("updateTime").textContent=now.toLocaleString("zh-CN",{hour12:false});
    renderAll();
    // 如果当前有选中的图表，重新加载
    const sel=document.querySelector(".list-item.selected");
    if(sel) select(sel.dataset.code);
  } catch(e) { console.error(e); }
  setTimeout(()=>btn.classList.remove("spin"),600);
}

// ===== 交易时间判断（A股 UTC+8） =====
function isTrading(d) {
  const t=new Date(d.toLocaleString("en-US",{timeZone:"Asia/Shanghai"}));
  const day=t.getDay(), h=t.getHours(), m=t.getMinutes(), mins=h*60+m;
  if(day===0||day===6) return false;
  return (mins>=570&&mins<690)||(mins>=780&&mins<900);
}

// ===== Watchlist =====
function toggleWL(code) {
  WL.has(code)?WL.delete(code):WL.add(code);
  saveWL(); renderAll();
}

// ===== 图表 =====
async function select(code) {
  const all=[...D.nasdaq,...D.sp500], etf=all.find(e=>e.code===code);
  $("chartTitle").textContent=etf?`${etf.name} (${code})`:code;
  document.querySelectorAll(".list-item.selected").forEach(el=>el.classList.remove("selected"));
  const row=document.querySelector(`.list-item[data-code="${code}"]`);
  if(row) row.classList.add("selected");
  // 优先使用localStorage中的溢价率历史，否则尝试K线API
  let rec=JSON.parse(localStorage.getItem("etf_h_"+code)||"[]");
  if(rec.length<3) rec=await fetchDaily(code);
  lastRec=rec;
  // 延迟一帧确保DOM已更新，canvas尺寸已确定
  requestAnimationFrame(()=>drawChart(rec));
}

function drawChart(rec) {
  const cvs=$("premiumChart");
  if(!cvs) return;
  if(chart){chart.destroy();chart=null}
  if(!rec||!rec.length){return}
  const ctx=cvs.getContext("2d");
  const isBB=theme==="bloomberg";
  const lc=isBB?"#ff8c00":"#2563eb", fc=isBB?"rgba(255,140,0,":"rgba(37,99,235,";
  const tc=isBB?"#787878":"#9ca3af", gc=isBB?"rgba(42,42,42,.6)":"rgba(226,230,234,.6)";
  const labels=rec.map(r=>typeof r[0]==="string"?r[0].slice(5):new Date(r[0]*1000).toTimeString().slice(0,5));
  const vals=rec.map(r=>r[1]==null?null:+r[1].toFixed(2));
  const maC={ma5:isBB?"#fff":"#6b7280",ma10:isBB?"#38bdf8":"#8b5cf6",ma20:isBB?"#22c55e":"#dc2626"};
  function calcMA(d,n){const r=[];for(let i=0;i<d.length;i++){if(i<n-1){r.push(null);continue}let s=0,c=0;for(let j=i-n+1;j<=i;j++)if(d[j]!=null){s+=d[j];c++}r.push(c?+(s/c).toFixed(2):null)}return r}
  const gradientBg=(context)=>{const g=context.chart.ctx.createLinearGradient(0,0,0,320);g.addColorStop(0,fc+"0.25)");g.addColorStop(1,fc+"0)");return g};
  const ds=[{label:"溢价率%",data:vals,borderColor:lc,backgroundColor:gradientBg,borderWidth:2,pointRadius:0,tension:.3,fill:true}];
  ["ma5","ma10","ma20"].forEach(k=>{if(ma[k])ds.push({label:k.toUpperCase(),data:calcMA(vals,+k.slice(2)),borderColor:maC[k],backgroundColor:"transparent",borderWidth:1,pointRadius:0,tension:.3,fill:false,borderDash:k==="ma5"?[]:k==="ma10"?[4,2]:[6,3]})});
  chart=new Chart(ctx,{type:"line",data:{labels,datasets:ds},options:{responsive:true,maintainAspectRatio:false,animation:{duration:300},interaction:{intersect:false,mode:"index"},plugins:{legend:{display:false},tooltip:{backgroundColor:isBB?"#1a1a1a":"#fff",titleColor:isBB?"#c8c8c8":"#1f2937",bodyColor:lc,borderColor:isBB?"#333":"#e2e6ea",borderWidth:1,padding:10,callbacks:{label:c=>`${c.dataset.label}: ${c.parsed.y!=null?c.parsed.y.toFixed(2)+"%":"--"}`}}},scales:{x:{display:true,grid:{display:false},ticks:{color:tc,font:{size:10},maxTicksLimit:10,autoSkip:true}},y:{display:true,grid:{color:gc},ticks:{color:tc,font:{size:10},callback:v=>v.toFixed(1)+"%"}}}}});
}

// ===== 主题 =====
function applyTheme() {
  document.documentElement.setAttribute("data-theme",theme);
  $("themeToggle").textContent=theme==="bloomberg"?"LIT":"BBG";
  if(lastRec.length) drawChart(lastRec);
}

// ===== 初始化 =====
document.addEventListener("DOMContentLoaded", async () => {
  WL=loadWL(); applyTheme();
  $("themeToggle").onclick=()=>{theme=theme==="bloomberg"?"light":"bloomberg";localStorage.setItem("theme",theme);applyTheme()};
  $("refreshBtn").onclick=refresh;
  $("nasdaqSort").onchange=renderAll;
  $("sp500Sort").onchange=renderAll;
  document.querySelectorAll(".ma-toggle").forEach(el=>el.onclick=()=>{ma[el.dataset.ma]=!ma[el.dataset.ma];el.classList.toggle("active");if(lastRec.length)drawChart(lastRec)});
  // 骨架屏
  ["nasdaqGrid","sp500Grid"].forEach(id=>{$(id).innerHTML=Array(7).fill(0).map(()=>'<div class="list-item loading"><span>☆</span><span>888888</span><span>加载</span><span>88.888</span><span>+88.88%</span><span>+88.88%</span><span>加载</span><span>加载</span><span>加载</span></div>').join("")});
  await refresh();
  select("513500");
});
