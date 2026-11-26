// WebDAV 音源插件（用户自建服务器 / 坚果云 / alist 转换的任意云盘）。
//   - 搜索：PROPFIND 递归列目录 → 解析 XML → 按扩展名筛音频 → 文件名匹配关键词 → 本地切片分页
//   - 取流：文件 URL 直链（WebDAV basic 认证走 URL 内嵌凭据，或公开访问）
//   - total = 匹配总数（真实；PROPFIND 一次拉全量后切片）
// 无歌词（本地文件大多无词；歌词绑定可在插件详情页配别的源）。
var meta = {
  apiVersion: 1,
  key: "webdav",
  name: "WebDAV",
  version: "1.0.0",
  capabilities: ["source"],
  allowedHosts: [], // 留空：靠 hostSettings 放行用户自填服务器
  hostSettings: ["server_url"], // 用户填的 URL host 自动加入网络白名单（沙箱仍只放行那一个 host）
  order: 85, // 独立音源之后
  settings: [
    { key: "server_url", name: "服务器地址", type: "text", default: "", desc: "WebDAV 服务根 URL，如 http://192.168.1.10:5005/ 或 https://dav.example.com/" },
    { key: "username", name: "用户名", type: "text", default: "", desc: "WebDAV 账号（留空=公开访问）" },
    { key: "password", name: "密码/令牌", type: "text", default: "", desc: "WebDAV 密码或应用令牌" },
    { key: "music_path", name: "音乐目录", type: "text", default: "", desc: "相对服务器根的音乐目录（如 /music/；留空=根目录递归）" },
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每页显示条数（PROPFIND 拉全量后本地切片）" },
  ],
};

var AUDIO_EXT = [".mp3", ".m4a", ".flac", ".wav", ".aac", ".ogg", ".opus", ".wma", ".ape", ".mp4"];

// 目录列表缓存（内存，不落盘）：PROPFIND 拉全量很慢，翻页/换关键词都复用。
// key = 服务器地址 + 音乐目录；TTL 5 分钟（host.now() 毫秒）。
// 搜索词变化不失效——过滤是本地做的，缓存的是原始文件列表。
var dirCache = { key: "", files: [], at: 0 };
var CACHE_TTL = 5 * 60 * 1000;

// search：PROPFIND Depth:1 逐目录递归（WebDAV 标准），拉全量音频 → 关键词匹配 → 本地切片。
function search(ctx) {
  var base = ctx.config.get("server_url", "").replace(/\/+$/, "");
  if (!base) return { error: "需要 WebDAV 服务器地址（插件设置里填）", total: 0, songs: [] };
  var root = ctx.config.get("music_path", "").replace(/^\/+/, "");
  var kw = (ctx.query || "").toLowerCase();

  // 目录列表：命中缓存则复用（翻页/换关键词不重拉），否则 PROPFIND 递归全量
  var cacheKey = base + "|" + root;
  var files;
  if (dirCache.key === cacheKey && dirCache.files.length > 0 && (host.now() - dirCache.at) < CACHE_TTL) {
    files = dirCache.files;
  } else {
    files = [];
    walk(base, root, ctx, 0, files, { n: 0 }); // 限深 6 层 + 2000 条防误配拉爆
    dirCache.key = cacheKey;
    dirCache.files = files;
    dirCache.at = host.now();
  }
  // 关键词匹配（文件名/路径子串）
  var matched = files.filter(function (f) {
    return !kw || f.path.toLowerCase().indexOf(kw) >= 0;
  });
  matched.sort(function (a, b) { return a.path < b.path ? -1 : (a.path > b.path ? 1 : 0); });

  var size = ctx.size || 40;
  var page = Math.max(1, ctx.page || 1);
  var start = (page - 1) * size;
  var slice = matched.slice(start, start + size).map(function (f) {
    return {
      id: f.href, // 完整 URL（getURL 直接用；含编码后的路径）
      title: f.name,
      artist: "",
      cover: "",
      duration: "",
      durationSec: 0,
      typename: "webdav",
      album: "",
      vip: false,
    };
  });
  // total = 真实匹配总数（PROPFIND 全量），分页按 total/size 严格对齐
  return { total: matched.length, songs: slice };
}

// walk：递归 PROPFIND Depth:1，收文件 + 子目录。
function walk(base, dir, ctx, depth, out, counter) {
  if (depth > 6 || counter.n > 2000 || out.length > 2000) return;
  var url = dir ? base + "/" + dir.replace(/\/+$/, "") + "/" : base + "/";
  var user = ctx.config.get("username", "");
  var pass = ctx.config.get("password", "");
  var headers = { "Depth": "1", "User-Agent": "mymusic/1.0" };
  if (user) headers["Authorization"] = "Basic " + host.b64encode(user + ":" + pass);
  var resp = ctx.host.http({ url: url, method: "PROPFIND", headers: headers, timeoutMs: 15000 });
  if (resp.error || (resp.status !== 207 && resp.status !== 200)) return;
  var entries = parsePropfind(resp.body, base, dir);
  for (var i = 0; i < entries.length; i++) {
    counter.n++;
    var e = entries[i];
    if (e.isDir) {
      walk(base, e.path, ctx, depth + 1, out, counter);
    } else if (isAudio(e.name)) {
      out.push({ href: e.href, path: e.path, name: e.name });
    }
  }
}

// parsePropfind：解析 WebDAV multistatus XML（DAV: 命名空间前缀不固定，用标签本地名匹配）。
function parsePropfind(xml, base, parentDir) {
  var out = [];
  // 每个 <response> 块：href + resourcetype/collection
  var responses = xml.match(/<[a-zA-Z0-9]*:?response[\s>][\s\S]*?<\/[a-zA-Z0-9]*:?response>/gi) || [];
  for (var i = 0; i < responses.length; i++) {
    var block = responses[i];
    var hm = block.match(/<[a-zA-Z0-9]*:?href>([\s\S]*?)<\/[a-zA-Z0-9]*:?href>/i);
    if (!hm) continue;
    var href = decodeXML(hm[1].trim());
    // href 三种形态：完整 URL / 绝对路径（以 / 开头，含 base path）/ 相对路径。
    // 绝对路径以服务器根为基准拼（http://host + /dav/x），不能再拼 base（会变 /dav/dav 死循环）。
    var abs = href;
    if (!/^https?:/i.test(abs)) {
      if (href.charAt(0) === "/") {
        var hm2 = base.match(/^(https?:\/\/[^/]+)/i);
        abs = (hm2 ? hm2[1] : "") + href;
      } else {
        abs = base + href;
      }
    }
    // 路径去掉 base 的路径部分 → 相对 base 的路径（walk 递归用）
    var basePath = base.replace(/^https?:\/\/[^/]+/, "").replace(/\/+$/, "");
    var absPath = abs.replace(/^https?:\/\/[^/]+/, "").replace(/\/+$/, "");
    var p = absPath.indexOf(basePath) === 0 ? absPath.substring(basePath.length).replace(/^\/+/, "") : absPath.replace(/^\/+/, "");
    // 跳过自己（避免死循环）——WebDAV 响应里 href=/dav/ 就是 base 自己
    var self = parentDir.replace(/^\/+/, "").replace(/\/+$/, "");
    if (p === self) continue;
    var name = p.substring(p.lastIndexOf("/") + 1);
    if (!name) continue;
    var isDir = /<[a-zA-Z0-9]*:?collection\s*\/>/i.test(block) ||
                /<[a-zA-Z0-9]*:?resourcetype[^>]*>[\s\S]*?<[a-zA-Z0-9]*:?collection[\s\S]*?<\/[a-zA-Z0-9]*:?resourcetype>/i.test(block);
    out.push({ href: abs, path: p, name: decodeXML(name), isDir: isDir });
  }
  return out;
}

// isAudio：扩展名白名单（大小写不敏感）。
function isAudio(name) {
  var low = name.toLowerCase();
  for (var i = 0; i < AUDIO_EXT.length; i++) {
    if (low.indexOf(AUDIO_EXT[i], low.length - AUDIO_EXT[i].length) >= 0) return true;
  }
  return false;
}

// decodeXML：实体反转义 + URL 解码（href 常是 %XX 编码）。
function decodeXML(s) {
  s = s.replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">")
       .replace(/&quot;/g, "\"").replace(/&apos;/g, "'");
  try { s = decodeURIComponent(s); } catch (e) {}
  return s;
}

// getURL：直接返回 WebDAV 文件 URL。
// 凭据处理：如果 URL 里没内嵌 user:pass，且插件配了账号，这里拼进 URL（basic 认证）。
function getURL(ctx) {
  var url = ctx.song.id;
  if (!url) return { error: "缺文件 URL" };
  var user = ctx.config.get("username", "");
  var pass = ctx.config.get("password", "");
  if (user && url.indexOf("://") >= 0) {
    var at = url.indexOf("://") + 3;
    var slash = url.indexOf("/", at);
    if (slash < 0) slash = url.length;
    var authority = url.substring(at, slash);
    if (authority.indexOf("@") < 0) { // 没有内嵌凭据才拼
      url = url.substring(0, at) + encodeURIComponent(user) + ":" + encodeURIComponent(pass) + "@" + url.substring(at);
    }
  }
  return { url: url, quality: 0 };
}

// fmtDur 占位（WebDAV 无时长元数据，UI 显示空）——保留以对齐其它插件风格。
function fmtDur(sec) { return ""; }
