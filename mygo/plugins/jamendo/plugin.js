// Jamendo 音源（免费 CC 音乐，免登录；API 需 client_id——Jamendo Developer Portal 免费注册）。
//   - 搜索：/v3.0/tracks（namesearch）→ results[]（id/name/artist_name/album_name/duration/image）
//   - 取流：/v3.0/tracks（ids + audioformat=mp32）→ results[0].audio（mp3 URL）
// client_id 从设置读（设置页填；无 key 搜索报错提示）。
var meta = {
  apiVersion: 1,
  key: "jamendo",
  name: "Jamendo",
  capabilities: ["source"], // 免费 CC 音乐（无歌词）
  allowedHosts: ["api.jamendo.com"],
  order: 60, // bilibili 10 / 网易云 20 / QQ 30 / 咪咕 40 / 酷我 50 / 千千 55 之后
  settings: [
    { key: "client_id", name: "Client ID", type: "text", default: "", desc: "Jamendo Developer Portal（developer.jamendo.com）免费注册拿 client_id，必填（否则搜索报错）" },
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
  ],
};

// 搜索：namesearch → results[]。
function search(ctx) {
  var cid = ctx.config.get("client_id", "");
  if (!cid) return { error: "需要 Jamendo client_id（设置页填，developer.jamendo.com 免费注册）", total: 0, songs: [] };
  var url = "https://api.jamendo.com/v3.0/tracks/?client_id=" + encodeURIComponent(cid) +
    "&format=json&limit=" + ctx.size + "&offset=" + ((ctx.page - 1) * ctx.size) +
    "&namesearch=" + encodeURIComponent(ctx.query);
  var resp = ctx.host.http({ url: url, headers: { "User-Agent": "mymusic/1.0" } });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  var d = JSON.parse(resp.body);
  var results = d.results || [];
  var songs = results.map(function (t) {
    var sec = parseInt(t.duration || 0, 10) || 0;
    return {
      id: t.id,
      title: t.name || "",
      artist: t.artist_name || "",
      cover: t.image || "",
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "jamendo",
      album: t.album_name || "",
      vip: false, // 免费 CC 音乐
    };
  });
  var total = parseInt((d.headers || {}).results_count || songs.length, 10) || songs.length;
  return { total: total, songs: songs };
}

// 取流：ids + audioformat=mp32 → results[0].audio（mp3 直链）。
function getURL(ctx) {
  var cid = ctx.config.get("client_id", "");
  if (!cid) return { error: "需要 Jamendo client_id（设置页填）" };
  var url = "https://api.jamendo.com/v3.0/tracks/?client_id=" + encodeURIComponent(cid) +
    "&format=json&ids=" + encodeURIComponent(ctx.song.id) + "&audioformat=mp32";
  var resp = ctx.host.http({ url: url, headers: { "User-Agent": "mymusic/1.0" } });
  if (resp.error) return { error: resp.error };
  var d = JSON.parse(resp.body);
  var t = (d.results || [])[0] || {};
  var audio = t.audio || t.audiodownload || "";
  if (!audio) return { error: "Jamendo 无音频（audioformat 无此源）" };
  return { url: audio, quality: 0 };
}

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
