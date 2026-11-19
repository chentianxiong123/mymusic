// Audius 音源（免登录公开 API，无 key，去中心化独立音乐——电子/Hip-Hop/EDM 免费）。
//   - 搜索：/v1/tracks/search（query/app_name）→ data[]（id/title/user/artwork/duration）
//   - 取流：/v1/tracks/{id}/stream（app_name）→ 音频流直链（免登录）
// 无歌词（独立音乐大多无词）；内容偏独立/电子（非主流流行）。
var meta = {
  apiVersion: 1,
  key: "audius",
  name: "Audius",
  capabilities: ["source"], // 免登录零版权（无歌词）
  allowedHosts: ["api.audius.co"],
  order: 75, // bilibili 10 / 网易云 20 / QQ 30 / 咪咕 40 / 酷我 50 / 千千 55 / Jamendo 60 / archive 65 / 酷狗 70 之后
  settings: [
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
  ],
};

var APP_NAME = "mymusic"; // 公开 API 自定义应用名（免 key）

// 搜索：/v1/tracks/search。
function search(ctx) {
  var url = "https://api.audius.co/v1/tracks/search?query=" + encodeURIComponent(ctx.query) +
    "&app_name=" + APP_NAME + "&limit=" + ctx.size + "&offset=" + ((ctx.page - 1) * ctx.size);
  var resp = ctx.host.http({ url: url, headers: { "User-Agent": "mymusic/1.0" } });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  var d = JSON.parse(resp.body);
  var tracks = d.data || [];
  var songs = tracks.map(function (t) {
    var artwork = t.artwork || {};
    var art = artwork["480x480"] || artwork["150x150"] || artwork["1000x1000"] || "";
    var sec = Math.round((t.duration || 0) / 1000) || 0;
    return {
      id: t.id,
      title: t.title || "",
      artist: ((t.user || {}).name || ""),
      cover: art,
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "audius",
      album: t.album_name || "",
      vip: false, // 免费
    };
  });
  return { total: songs.length, songs: songs };
}

// 取流：/v1/tracks/{id}/stream（免登录直链；直接返回 URL，播放链路下载）。
function getURL(ctx) {
  var id = ctx.song.id;
  if (!id) return { error: "缺 track id" };
  var url = "https://api.audius.co/v1/tracks/" + encodeURIComponent(id) + "/stream?app_name=" + APP_NAME;
  return { url: url, quality: 0 };
}

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
