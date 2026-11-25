// 退出记忆开关（对齐 native 启动恢复配置）：各孤岛读取 localStorage 前先判断是否允许恢复。
// 关闭时读空值（不删用户数据——重新打开开关即可恢复）。
(function () {
  window.hcRestore = {
    allowed: function (key) {
      try { return localStorage.getItem('hc:restore:' + key) !== '0'; } catch (e) { return true; }
    },
    read: function (key, fallback) {
      if (!this.allowed(key)) return fallback;
      try { return localStorage.getItem(key) ?? fallback; } catch (e) { return fallback; }
    },
    readJSON: function (key, fallback) {
      if (!this.allowed(key)) return fallback;
      try { return JSON.parse(localStorage.getItem(key) || '[]'); } catch (e) { return fallback; }
    }
  };
})();
