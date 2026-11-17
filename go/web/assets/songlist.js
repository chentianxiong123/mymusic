// 歌单（零件）：新建 / 重命名 / 添加到歌单弹层。
// 数据走 /api/v1/songlists（RESTful，{code,message,data} 信封）。
// 与 playlist（= 播放队列）区分：歌单是用户自建、可命名、可多份的收藏集合。
(function () {
  function toast(msg) { hcBus.emit('toast', { msg: msg }); }

  function api(url, opts) {
    return fetch(url, opts).then((r) => r.json()).then((j) => {
      if (j && j.code === 0) return j.data;
      throw new Error((j && j.message) || '请求失败');
    });
  }

  // 新建歌单：prompt 名字 → POST → 刷新网格片段（htmx 触发）
  async function create() {
    const name = window.prompt('歌单名称');
    if (!name || !name.trim()) return;
    try {
      await api('/api/v1/songlists', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim() }),
      });
      toast('已创建「' + name.trim() + '」');
      refreshGrid();
    } catch (e) {
      toast(e.message || '创建失败');
    }
  }

  // 重命名：prompt 现名 → PATCH → 刷新
  async function rename(id, curName) {
    const name = window.prompt('新歌单名称', curName || '');
    if (!name || !name.trim()) return;
    try {
      await api('/api/v1/songlists/' + id + '/rename', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim() }),
      });
      toast('已改名');
      refreshDetail(id);
    } catch (e) {
      toast(e.message || '改名失败');
    }
  }

  // 刷新歌单网格片段（htmx.ajax 走 hx-target，保持 htmx 事件链）
  function refreshGrid() {
    if (window.htmx) {
      htmx.ajax('GET', '/hx/songlists', { target: '#main', select: '#main', swap: 'innerHTML' });
    } else {
      location.reload();
    }
  }

  // 刷新歌单详情片段（保留当前页）
  function refreshDetail(id) {
    if (!id) { refreshGrid(); return; }
    if (window.htmx) {
      htmx.ajax('GET', '/hx/songlists/' + id, { target: '#main', select: '#main', swap: 'innerHTML' });
    } else {
      location.reload();
    }
  }

  // ---- 添加到歌单弹层 ----
  // 从右键菜单「添加到歌单」触发；弹出歌单列表，点一个即加入。
  let sheet = null;

  function closeSheet() {
    if (sheet) { sheet.remove(); sheet = null; }
  }

  function buildSheet(item) {
    const m = document.createElement('div');
    m.className = 'fixed inset-0 z-[75] bg-black/50 flex items-end sm:items-center justify-center';
    m.innerHTML =
      '<div class="w-full sm:max-w-sm bg-white dark:bg-gray-800 rounded-t-3xl sm:rounded-3xl shadow-2xl overflow-hidden">' +
      '<div class="flex items-center justify-between px-5 py-4 border-b border-gray-100 dark:border-gray-700">' +
      '<span class="font-semibold text-gray-900 dark:text-white">添加到歌单</span>' +
      '<button class="w-8 h-8 rounded-full flex items-center justify-center text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700" data-close>' +
      '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6l12 12M18 6L6 18"/></svg></button>' +
      '</div>' +
      '<div class="px-5 py-3 text-sm text-gray-500 dark:text-gray-400 truncate">' + (item.title || '') + '</div>' +
      '<div class="max-h-72 overflow-y-auto pb-2" data-list><div class="py-8 text-center text-sm text-gray-400">加载中…</div></div>' +
      '<div class="p-3 border-t border-gray-100 dark:border-gray-700">' +
      '<button data-new class="w-full px-4 py-2.5 rounded-full bg-gradient-to-r from-pink-500 to-violet-500 text-white text-sm font-medium">新建歌单</button>' +
      '</div></div>';
    document.body.appendChild(m);

    m.addEventListener('click', (e) => {
      if (e.target === m || e.target.closest('[data-close]')) closeSheet();
    });
    m.querySelector('[data-new]').addEventListener('click', () => {
      const name = window.prompt('新歌单名称');
      if (name && name.trim()) {
        api('/api/v1/songlists', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name: name.trim() }),
        })
          .then((g) => { closeSheet(); addItem(g.id, item); toast('已创建并添加'); refreshGrid(); })
          .catch((e2) => toast(e2.message || '创建失败'));
      }
    });
    return m;
  }

  function renderList(m, item) {
    const box = m.querySelector('[data-list]');
    api('/api/v1/songlists').then((lists) => {
      if (!lists || !lists.length) {
        box.innerHTML = '<div class="py-8 text-center text-sm text-gray-400">还没有歌单，点下方「新建歌单」</div>';
        return;
      }
      box.innerHTML = '';
      lists.forEach((g) => {
        const b = document.createElement('button');
        b.className = 'w-full flex items-center px-5 py-3 text-left hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors';
        b.innerHTML =
          '<div class="w-10 h-10 rounded-lg bg-gray-100 dark:bg-gray-700 flex-shrink-0 flex items-center justify-center mr-3">' +
          '<svg class="w-5 h-5 text-gray-400" viewBox="0 0 24 24" fill="currentColor"><path d="M9 18V6H7v12h2zm8-12v12h2V6h-2z"/></svg></div>' +
          '<div class="flex-1 min-w-0"><p class="text-sm font-medium text-gray-900 dark:text-white truncate"></p>' +
          '<p class="text-xs text-gray-400 mt-0.5"></p></div>';
        b.querySelector('p').textContent = g.name;
        b.querySelectorAll('p')[1].textContent = (g.count || 0) + ' 首';
        b.addEventListener('click', () => { closeSheet(); addItem(g.id, item); });
        box.appendChild(b);
      });
    }).catch(() => {
      box.innerHTML = '<div class="py-8 text-center text-sm text-red-400">加载失败</div>';
    });
  }

  function addItem(id, item) {
    const q = new URLSearchParams({
      bvid: item.bvid || '',
      title: item.title || '',
      artist: item.artist || '',
      cover: item.cover || '',
      duration: String(item.duration || 0),
    });
    fetch('/hx/songlists/add/' + id + '?' + q.toString(), { method: 'POST' })
      .then((r) => {
        toast(r.ok ? '已添加到歌单' : '添加失败');
        if (window.htmx && location.pathname.indexOf('/hx/songlists/') === 0) {
          htmx.ajax('GET', location.pathname, { target: '#main', select: '#main', swap: 'innerHTML' });
        }
      })
      .catch(() => toast('添加失败'));
  }

  // 供右键菜单调用
  function pick(item) {
    if (!item || !item.bvid) return;
    closeSheet();
    sheet = buildSheet(item);
    renderList(sheet, item);
  }

  window.hcSonglist = { create: create, rename: rename, pick: pick };

  // 添加到歌单完成 → 刷新（若当前在歌单详情页）
  hcBus.on('songlist-changed', () => { /* 保留：未来跨页刷新钩子 */ });
})();