// plugins/demo-source/plugin.js —— JS 沙箱插件示例（网易云音源，免登录）
//
// 形态：全局函数（非 ES module，sobek 不是 Node）。
//   - var meta   : 声明式元数据（apiVersion/key/name/capabilities/allowedHosts/order）
//   - search()   : 音源搜索 → { total, songs:[{id,title,artist,cover,duration,durationSec,typename}] }
//   - getURL()   : 取流 → { url, quality, duration }（或 { error: "..." }）
//   - lyric()    : 歌词 → { lrc }（宿主统一解析 LRC）
//
// 所有网络走 ctx.host.http（宿主注入，白名单 meta.allowedHosts，超时+并发上限）。
var meta = {
  apiVersion: 1,
  key: "demo",
  name: "示例音源（网易云）",
  version: "1.0.0",
  capabilities: ["source", "lyric"],
  allowedHosts: ["music.163.com"],
  order: 25, // 内置 10/20/30/40/50/60 的空位
};

// 免登录明文接口需带的头（Referer + Cookie 匿名设备id，防 405 风控）。
function wyHeaders() {
  return {
    "Referer": "https://music.163.com/",
    "Cookie": "os=pc; appver=8.7.01; channel=netease",
  };
}

// 音源搜索：网易云明文 /api/search/get/web（免登录）。
function search(ctx) {
  var offset = (ctx.page - 1) * ctx.size;
  var resp = ctx.host.http({
    url: "https://music.163.com/api/search/get/web?s=" + encodeURIComponent(ctx.query) +
         "&type=1&offset=" + offset + "&limit=" + ctx.size + "&total=true",
    headers: wyHeaders(),
  });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  var data = JSON.parse(resp.body);
  if (!data.result || !data.result.songs) return { total: 0, songs: [] };
  var songs = data.result.songs.map(function (s) {
    var artists = (s.artists || []).map(function (a) { return a.name; }).join("/");
    var sec = Math.round((s.duration || 0) / 1000);
    return {
      id: String(s.id),
      title: s.name,
      artist: artists,
      cover: (s.album && s.album.picUrl) || "",
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "demo",
    };
  });
  return { total: data.result.songCount || songs.length, songs: songs };
}

// 取流：网易云明文 /api/song/enhance/player/url（免登录，免费歌 128k）。
// VIP/无权限 → data[0].url 为 null → 返回 error。
function getURL(ctx) {
  var id = ctx.song.id;
  var br = ctx.quality >= 2 ? 320000 : 128000;
  var resp = ctx.host.http({
    url: "https://music.163.com/api/song/enhance/player/url?ids=[" + id + "]&br=" + br,
    headers: wyHeaders(),
  });
  if (resp.error) return { error: resp.error };
  var data = JSON.parse(resp.body);
  var item = data.data && data.data[0];
  if (!item || !item.url) return { error: "无权限或需登录（VIP 歌免登录不可播）" };
  return { url: item.url, quality: ctx.quality || 1, duration: (item.time || 0) / 1000 };
}

// 歌词：网易云明文 /api/song/lyric（免登录）。宿主统一解析 LRC。
function lyric(ctx) {
  var resp = ctx.host.http({
    url: "https://music.163.com/api/song/lyric?id=" + ctx.song.id + "&lv=1&kv=1&tv=-1",
    headers: wyHeaders(),
  });
  if (resp.error) return { error: resp.error, lrc: "" };
  var data = JSON.parse(resp.body);
  return { lrc: (data.lrc && data.lrc.lyric) || "" };
}

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
