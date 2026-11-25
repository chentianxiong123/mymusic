// 搜索历史（零件/服务器态补充）：localStorage P 态记录 + 搜索框下 chips
// 记录时机：任何 /hx/search?kw= 请求发出前（htmx:beforeRequest）；渲染跟随页面切换
(function () {
  const KEY = 'hc:searches';
  const MAX = 8;
  function load() {
    try { return window.hcRestore ? window.hcRestore.readJSON(KEY, []) : JSON.parse(localStorage.getItem(KEY) || '[]'); } catch (e) { return []; }
  }
  function save(list) {
    try { localStorage.setItem(KEY, JSON.stringify(list)); } catch (e) {}
  }
  function record(kw) {
    kw = (kw || '').trim();
    if (!kw) return;
    let list = load().filter((k) => k !== kw);
    list.unshift(kw);
    save(list.slice(0, MAX));
  }
  function clear(one) {
    let list = load();
    if (one) list = list.filter((k) => k !== one);
    else list = [];
    save(list);
    render();
  }
  function render() {
    const box = document.getElementById('search-history');
    if (!box) return;
    const list = load();
    if (!list.length) { box.innerHTML = ''; return; }
    // 单行横滑 + hover 悬浮箭头（对齐 native 搜索历史最终形态）
    box.innerHTML =
      '<div class="hc-his-wrap" style="position:relative;max-width:100%;margin:16px auto 0;display:flex;justify-content:center">' +
      '<div class="hc-his-strip" style="display:flex;gap:8px;overflow-x:auto;scrollbar-width:none;white-space:nowrap;padding:2px 28px;max-width:100%">' +
      list.map((k) =>
        '<button class="search-his-chip px-3 py-1.5 rounded-full text-sm bg-gray-800/70 border border-gray-700 text-gray-300 hover:border-pink-500/60 hover:text-pink-300 transition-colors" style="flex:none" data-kw="' +
        k.replace(/"/g, '&quot;') + '">' + k.replace(/</g, '&lt;') + '</button>'
      ).join('') +
      '<button class="search-his-clear px-2 py-1.5 text-xs text-gray-600 hover:text-red-400 transition-colors" style="flex:none">清空</button>' +
      '</div>' +
      '<button class="hc-his-nav hc-his-prev" style="display:none;position:absolute;left:0;top:50%;transform:translateY(-50%);z-index:2;width:28px;height:28px;border-radius:50%;background:rgba(0,0,0,.7);color:#fff;border:none;cursor:pointer">‹</button>' +
      '<button class="hc-his-nav hc-his-next" style="display:none;position:absolute;right:0;top:50%;transform:translateY(-50%);z-index:2;width:28px;height:28px;border-radius:50%;background:rgba(0,0,0,.7);color:#fff;border:none;cursor:pointer">›</button>' +
      '</div>';
    const strip = box.querySelector('.hc-his-strip');
    const prev = box.querySelector('.hc-his-prev');
    const next = box.querySelector('.hc-his-next');
    const updNav = () => {
      if (!strip) return;
      prev.style.display = strip.scrollLeft > 4 ? '' : 'none';
      next.style.display = strip.scrollLeft + strip.clientWidth < strip.scrollWidth - 4 ? '' : 'none';
    };
    if (strip && prev && next) {
      strip.addEventListener('scroll', updNav);
      prev.addEventListener('click', () => strip.scrollBy({ left: -240, behavior: 'smooth' }));
      next.addEventListener('click', () => strip.scrollBy({ left: 240, behavior: 'smooth' }));
      const wrap = box.querySelector('.hc-his-wrap');
      wrap.addEventListener('mouseenter', updNav);
      wrap.addEventListener('mouseleave', () => { prev.style.display = 'none'; next.style.display = 'none'; });
      requestAnimationFrame(updNav);
    }
    box.querySelectorAll('.search-his-chip').forEach((b) => {
      b.addEventListener('click', () => {
        const inp = document.querySelector('input[name=kw]');
        if (inp) { inp.value = b.dataset.kw; inp.dispatchEvent(new Event('input', { bubbles: true })); }
      });
    });
    box.querySelector('.search-his-clear').addEventListener('click', () => clear());
  }
  document.addEventListener('htmx:beforeRequest', (e) => {
    const d = e.detail && (e.detail.requestConfig || e.detail);
    const path = (d && (d.path || (d.requestConfig && d.requestConfig.path))) || '';
    if (path.includes('/hx/search')) {
      const params = (d && (d.parameters || (d.requestConfig && d.requestConfig.parameters))) || {};
      record(params.kw || '');
    }
  });
  document.addEventListener('htmx:afterSwap', render);
  render();
})();