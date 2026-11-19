// plugins/qqmusic/plugin.js —— QQ 音乐音源（cookie 可选）
//
// 模式（与网易云/B站一致）：cookie 可选，有 cookie 用自己的（登录态/无损），无则匿名。
//   - 搜索：匿名可用（client_search_cp）
//   - 取流：UrlGetVkey（music-lib 姿势），匿名返回 code=1000 无 purl → 需 cookie
//   - 有 cookie → purl 直链（ws.stream.qqmusic.qq.com）
var meta = {
  apiVersion: 1,
  key: "qqmusic",
  name: "QQ音乐",
  version: "1.0.0",
  capabilities: ["source"],
  allowedHosts: ["c.y.qq.com", "u.y.qq.com"],
  order: 30, // bilibili 10 / 网易云 20 之后
  settings: [
    { key: "quality", name: "音质", type: "select", options: ["M500", "M800"], default: "M800", desc: "取流前缀（M500=128k 标准，M800=320k 高品；无损需 cookie）" },
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
    { key: "cookie", name: "登录 Cookie", type: "text", default: "", desc: "粘贴 QQ 音乐登录态 Cookie（浏览器 y.qq.com 登录后复制），空=匿名（仅搜索，取流需登录）" },
  ],
};

// 请求头：Referer + Content-Type；有登录态 cookie 则追加（无损/登录态）。
function qqHeaders(ctx) {
  var h = { "Referer": "https://y.qq.com/", "Content-Type": "application/json" };
  if (ctx && ctx.config && ctx.config.get) {
    var extra = ctx.config.get("cookie", "");
    if (extra) h["Cookie"] = extra;
  }
  return h;
}

// 音源搜索：c.y.qq.com/soso/fcgi-bin/client_search_cp（匿名可用）。
function search(ctx) {
  var q = encodeURIComponent(ctx.query);
  var n = parseInt(ctx.config && ctx.config.get ? ctx.config.get("searchSize", "40") : "40", 10) || 40;
  var p = ctx.page || 1;
  var resp = ctx.host.http({
    url: "https://c.y.qq.com/soso/fcgi-bin/client_search_cp?p=" + p + "&n=" + n + "&w=" + q + "&format=json",
    headers: { "Referer": "https://y.qq.com/" },
  });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  var data = JSON.parse(resp.body);
  var song = data.data && data.data.song;
  if (!song) return { total: 0, songs: [] };
  var songs = song.list.map(function (s) {
    var artists = (s.singer || []).map(function (x) { return x.name; }).join("/");
    var sec = s.interval || 0;
    return {
      id: s.songmid,
      title: s.songname,
      artist: artists,
      // 封面：album.mid 拼 y.gtimg.cn（QQ 封面 CDN，无防盗链）。
      cover: s.albummid ? "https://y.gtimg.cn/music/photo_new/T002R300x300M000" + s.albummid + ".jpg" : "",
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "qqmusic",
      album: s.albumname || "",
      vip: !!(s.pay && s.pay.payplay),
    };
  });
  return { total: song.totalnum || songs.length, songs: songs };
}

// 取流：u.y.qq.com/cgi-bin/musicu.fcg（UrlGetVkey，music-lib 姿势）。
// 匿名 → code=1000 无 purl → error（需 cookie）；有 cookie → purl 直链。
function getURL(ctx) {
  var mid = ctx.song.id;
  var prefix = ctx.config && ctx.config.get ? ctx.config.get("quality", "M800") : "M800";
  if (prefix !== "M500") prefix = "M800";
  var filename = prefix + mid + mid + ".mp3"; // M500{mid}{mid}.mp3 / M800{mid}{mid}.mp3
  var guid = String(Math.floor(Math.random() * 9000000000) + 1000000000);
  var body = JSON.stringify({
    comm: { cv: 4747474, ct: 24, format: "json", inCharset: "utf-8", outCharset: "utf-8", notice: 0, platform: "yqq.json", needNewCode: 1, uin: 0 },
    req_1: { module: "music.vkey.GetVkey", method: "UrlGetVkey", param: { guid: guid, songmid: [mid], songtype: [0], uin: "0", loginflag: 1, platform: "20", filename: [filename] } },
  });
  var resp = ctx.host.http({ url: "https://u.y.qq.com/cgi-bin/musicu.fcg", method: "POST", body: body, headers: qqHeaders(ctx) });
  if (resp.error) return { error: resp.error };
  var data = JSON.parse(resp.body);
  var req1 = data.req_1 || {};
  var info = (req1.data && req1.data.midurlinfo && req1.data.midurlinfo[0]) || {};
  if (!info.purl) return { error: "需登录或版权受限（QQ 取流要 cookie，粘贴后可用）" };
  return { url: "https://ws.stream.qqmusic.qq.com/" + info.purl, quality: prefix === "M500" ? 0 : 2 };
}

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
