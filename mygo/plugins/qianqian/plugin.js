// 千千音乐音源（qianqian，原百度音乐，music.91q.com 免登录）。
// 接口（musicdl 姿势）：公开 APPID=16073360 + sign=MD5(sorted params + secret)，免登录。
//   - 搜索：/v1/search（word/type/pageNo/pageSize/appid + timestamp + sign）→ data.typeTrack[]
//   - 取流：/v1/song/tracklink（TSID/appid/rate + timestamp + sign）→ data.path
//   - 歌词：搜索返回 t.lyric（URL）→ 拉 lrc
var meta = {
  apiVersion: 1,
  key: "qianqian",
  name: "千千音乐",
  capabilities: ["source", "lyric"],
  allowedHosts: ["music.91q.com"],
  order: 55, // bilibili 10 / 网易云 20 / QQ 30 / 咪咕 40 / 酷我 50 之后
  settings: [
    { key: "quality", name: "音质", type: "select", options: ["128", "320"], default: "320", desc: "取流码率（128=128k，320=高品；无损 3000 需接口支持）" },
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
  ],
};

var APPID = "16073360";
var SECRET = "0b50b02fd0d73a9c4c8c3a781c30845f"; // 公开（musicdl 常量）

// 请求头（Referer 必需；千千免登录无 cookie）。
function qqHeaders(ctx) {
  return { "Referer": "https://music.91q.com/player", "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36" };
}

// sign：params sorted + timestamp + sign=MD5(string+secret)（千千 API 必需；host.md5 注入）。
function signedParams(params, ctx) {
  params.timestamp = String(Math.floor(Date.now() / 1000));
  var keys = Object.keys(params).sort();
  var string = keys.map(function (k) { return k + "=" + params[k]; }).join("&");
  params.sign = ctx.host.md5(string + SECRET);
  return params;
}

// 构造 URL。
function qqURL(path, params) {
  return "https://music.91q.com" + path + "?" + Object.keys(params).map(function (k) { return k + "=" + encodeURIComponent(params[k]); }).join("&");
}

// 搜索：/v1/search → data.typeTrack[]（TSID/title/artist/albumTitle/pic/duration/lyric）。
function search(ctx) {
  var params = signedParams({ word: ctx.query, type: "1", pageNo: String(ctx.page), pageSize: String(ctx.size), appid: APPID }, ctx);
  var resp = ctx.host.http({ url: qqURL("/v1/search", params), headers: qqHeaders(ctx) });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  var d = JSON.parse(resp.body);
  var list = ((d.data || {}).typeTrack || []);
  var songs = list.map(function (t) {
    var sec = parseInt(t.duration || 0, 10) || 0;
    return {
      id: t.TSID,
      title: t.title || "",
      artist: (t.artist || []).map(function (a) { return a.name; }).filter(Boolean).join(","),
      cover: t.pic || "",
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "qianqian",
      album: t.albumTitle || "",
      vip: false,
    };
  });
  var total = parseInt((d.data || {}).total || songs.length, 10) || songs.length;
  return { total: total, songs: songs };
}

// 取流：/v1/song/tracklink → data.path（或 trail_audio_info.path）。
function getURL(ctx) {
  var rate = ctx.config.get("quality", "320") === "128" ? "128" : "320";
  var params = signedParams({ TSID: ctx.song.id, appid: APPID, rate: rate }, ctx);
  var resp = ctx.host.http({ url: qqURL("/v1/song/tracklink", params), headers: qqHeaders(ctx) });
  if (resp.error) return { error: resp.error };
  var d = JSON.parse(resp.body);
  var url = (d.data || {}).path || ((d.data || {}).trail_audio_info || {}).path || "";
  if (!url) return { error: "千千无直链（可能下架或需登录）" };
  return { url: url, quality: rate === "128" ? 0 : 2 };
}

// 歌词：搜标题拿 t.lyric（URL）→ 拉 lrc（千千免费歌；付费歌可能空）。
function lyric(ctx) {
  var kw = (ctx.keywords || [""])[0];
  if (!kw) return { error: "缺标题" };
  var params = signedParams({ word: kw, type: "1", pageNo: "1", pageSize: "10", appid: APPID }, ctx);
  var resp = ctx.host.http({ url: qqURL("/v1/search", params), headers: qqHeaders(ctx) });
  if (resp.error) return { error: resp.error };
  var d = JSON.parse(resp.body);
  var t = ((d.data || {}).typeTrack || [])[0] || {};
  var lyr = t.lyric || "";
  if (!lyr) return { error: "千千无歌词" };
  var lr = ctx.host.http({ url: lyr, headers: qqHeaders(ctx) });
  if (lr.error) return { error: lr.error };
  return { lrc: lr.body };
}

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
