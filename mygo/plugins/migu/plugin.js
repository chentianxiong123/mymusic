// plugins/migu/plugin.js —— 咪咕音乐音源（免登录，PQ 标准音质）
//
// 模式：cookie 可选（有则用自己的，无则匿名）。
//   - 搜索：search_all.do（匿名可用）
//   - 取流：listen-url（匿名 PQ 免登录直链；ZQ32/无损需 cookie）
var meta = {
  apiVersion: 1,
  key: "migu",
  name: "咪咕音乐",
  version: "1.0.0",
  capabilities: ["source", "lyric"],
  allowedHosts: ["pd.musicapp.migu.cn", "c.musicapp.migu.cn", "y.migu.cn", "d.musicapp.migu.cn"],
  order: 40, // bilibili 10 / 网易云 20 / QQ 30 之后
  settings: [
    { key: "quality", name: "音质", type: "select", options: ["PQ", "ZQ32"], default: "PQ", desc: "音质（标准 / 高品质）" },
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
    { key: "cookie", name: "登录 Cookie", type: "text", default: "", desc: "粘贴咪咕登录态 Cookie（浏览器 music.migu.cn 登录后复制），空=匿名（PQ 免登录）" },
  ],
};

var MIGU_UA = "Android_migu/7.41.13 okhttp/3.12.13";
var MIGU_CHANNEL = "0146832";
var MIGU_VERSION = "7.41.13";

// 请求头：咪咕 App 头（channel/version 必需）+ 可选登录 cookie。
function miguHeaders(ctx) {
  var h = { "User-Agent": MIGU_UA, "channel": MIGU_CHANNEL, "version": MIGU_VERSION, "Referer": "https://music.migu.cn/" };
  if (ctx && ctx.config && ctx.config.get) {
    var extra = ctx.config.get("cookie", "");
    if (extra) h["Cookie"] = extra;
  }
  return h;
}

// 音源搜索：search_all.do（匿名可用）。
function search(ctx) {
  var q = encodeURIComponent(ctx.query);
  var n = ctx.size || 40;
  var ss = JSON.stringify({ song: 1, album: 0, singer: 0, tagSong: 0, mvSong: 0, songlist: 0, bestShow: 1 });
  var url = "http://pd.musicapp.migu.cn/MIGUM2.0/v1.0/content/search_all.do?ua=Android_migu&version=5.0.1&text=" + q +
            "&pageNo=" + (ctx.page || 1) + "&pageSize=" + n + "&searchSwitch=" + encodeURIComponent(ss);
  var resp = ctx.host.http({ url: url, headers: miguHeaders(ctx) });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  var data = JSON.parse(resp.body);
  var rs = (data.songResultData && data.songResultData.result) || [];
  var songs = rs.map(function (s) {
    var singers = (s.singers || []).map(function (x) { return x.name; }).join("/");
    var album = (s.albums && s.albums[0] && s.albums[0].name) || "";
    var cover = (s.imgItems && s.imgItems[0] && s.imgItems[0].img) || s.img1 || "";
    var sec = s.duration || 0;
    return {
      id: s.contentId,
      title: s.name,
      artist: singers,
      cover: cover,
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "migu",
      album: album,
      vip: false, // 免费歌免登录（PQ）；无损/付费需 cookie，取流时判断
    };
  });
  var tc = data.songResultData && data.songResultData.totalCount;
  // 咪咕 totalCount 是字符串（如"10"），转 number 供 Go 反序列化。
  var total = (typeof tc === "number") ? tc : (parseInt(tc, 10) || songs.length);
  return { total: total, songs: songs };
}

// 取流：listen-url（免登录 PQ 直链；ZQ32 需 cookie）。
function getURL(ctx) {
  var id = ctx.song.id;
  var tone = ctx.config && ctx.config.get ? ctx.config.get("quality", "PQ") : "PQ";
  if (tone !== "ZQ32") tone = "PQ";
  var url = "https://c.musicapp.migu.cn/MIGUM2.0/v2.1/content/listen-url?netType=01&resourceType=2&contentId=" + id + "&toneFlag=" + tone;
  var resp = ctx.host.http({ url: url, headers: miguHeaders(ctx) });
  if (resp.error) return { error: resp.error };
  var d = JSON.parse(resp.body);
  var data = d.data || {};
  if (!data.url) return { error: "无直链（无损需登录 cookie，切 PQ 或粘贴 cookie）" };
  return { url: data.url, quality: tone === "ZQ32" ? 2 : 0 };
}

// 歌词：resourceinfo.do 拿 lrcUrl → 拉 lrc（免费歌；收费歌 lyricUrl 可能空）。
function lyric(ctx) {
  var id = ctx.sid; // 歌 contentId（loadLyricAsync 传 a.current.BVID）
  if (!id) return { error: "缺歌 id" };
  var url = "http://c.musicapp.migu.cn/MIGUM2.0/v1.0/content/resourceinfo.do?resourceId=" + encodeURIComponent(id) + "&resourceType=2";
  var resp = ctx.host.http({ url: url, headers: { "Referer": "https://y.migu.cn/", "User-Agent": "Android_migu/7.41.13 okhttp/3.12.13" } });
  if (resp.error) return { error: resp.error };
  var d = JSON.parse(resp.body);
  var res = (d.resource || d.Resource || [])[0] || {};
  var lrcUrl = res.lrcUrl || res.lyricUrl || "";
  if (!lrcUrl) return { error: "无歌词" };
  lrcUrl = String(lrcUrl).replace(/^http:/, "https:");
  var lr = ctx.host.http({ url: lrcUrl, headers: { "Referer": "https://y.migu.cn/" } });
  if (lr.error) return { error: lr.error };
  return { lrc: lr.body };
}

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
