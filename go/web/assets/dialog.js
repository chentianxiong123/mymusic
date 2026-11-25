// 统一弹窗组件（对齐 native Overlay 弹窗）：替代浏览器原生 prompt/confirm。
// 响应式：窄屏左右留 16px，高度不超 90%；点背景/取消关闭；回车确认、Esc 关闭。
(function () {
  window.hcDialog = {
    _build: function (opts) {
      var self = this;
      var old = document.getElementById('hc-dialog-root');
      if (old) old.remove();
      var root = document.createElement('div');
      root.id = 'hc-dialog-root';
      root.style.cssText = 'position:fixed;inset:0;z-index:9999;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.55)';
      var panel = document.createElement('div');
      panel.style.cssText = 'width:min(' + (opts.width || 360) + 'px,calc(100vw - 32px));max-height:90vh;overflow:auto;background:#fff;dark:bg-gray-800;border-radius:18px;padding:20px;box-shadow:0 24px 48px rgba(0,0,0,.3);box-sizing:border-box';
      if (document.documentElement.classList.contains('dark')) panel.style.background = '#1f2937';
      var title = document.createElement('div');
      title.style.cssText = 'font-size:16px;font-weight:700;margin-bottom:12px';
      title.textContent = opts.title || '';
      panel.appendChild(title);
      var body = document.createElement('div');
      body.style.cssText = 'font-size:13px;color:#6b7280;margin-bottom:16px';
      if (opts.message) { var p = document.createElement('p'); p.textContent = opts.message; body.appendChild(p); }
      var input = null;
      if (opts.input) {
        input = document.createElement('input');
        input.type = 'text';
        input.value = opts.value || '';
        input.placeholder = opts.placeholder || '';
        input.style.cssText = 'width:100%;box-sizing:border-box;padding:10px 12px;border-radius:10px;border:1px solid #d1d5db;background:transparent;font-size:14px';
        if (document.documentElement.classList.contains('dark')) input.style.borderColor = '#4b5563';
        body.appendChild(input);
      }
      panel.appendChild(body);
      var row = document.createElement('div');
      row.style.cssText = 'display:flex;gap:10px;justify-content:flex-end';
      var cancel = document.createElement('button');
      cancel.textContent = opts.cancelText || '取消';
      cancel.style.cssText = 'padding:8px 16px;border-radius:10px;border:1px solid #d1d5db;background:transparent;cursor:pointer;font-size:14px';
      var ok = document.createElement('button');
      ok.textContent = opts.okText || '确定';
      ok.style.cssText = 'padding:8px 16px;border-radius:10px;border:none;background:linear-gradient(90deg,#ec4899,#8b5cf6);color:#fff;cursor:pointer;font-size:14px;font-weight:500';
      row.appendChild(cancel); row.appendChild(ok); panel.appendChild(row);
      root.appendChild(panel); document.body.appendChild(root);
      if (input) { input.focus(); input.select(); }
      var done = function (val) {
        root.remove();
        document.removeEventListener('keydown', key);
        opts.callback(val);
      };
      var key = function (e) {
        if (e.key === 'Escape') done(null);
        else if (e.key === 'Enter' && input) done(input.value);
      };
      document.addEventListener('keydown', key);
      root.addEventListener('click', function (e) { if (e.target === root) done(null); });
      cancel.addEventListener('click', function () { done(null); });
      ok.addEventListener('click', function () { done(input ? input.value : true); });
    },
    confirm: function (message, callback, title) {
      if (typeof callback !== 'function') return new Promise(function (res) { this._build({ title: title || '确认', message: message, callback: res }); }.bind(this));
      this._build({ title: title || '确认', message: message, callback: callback });
    },
    prompt: function (title, value, callback, placeholder) {
      if (typeof callback !== 'function') return new Promise(function (res) { this._build({ title: title, input: true, value: value, placeholder: placeholder, callback: res }); }.bind(this));
      this._build({ title: title, input: true, value: value, placeholder: placeholder, callback: callback });
    }
  };
})();
