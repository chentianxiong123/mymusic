// ccMixter 音源（免登录公开 API，创作共用 CC 音乐——独立/remix 免费）。
//   - 搜索：api.ccmixter.org/api/query（f=json）→ uploads[]（upload_name/user_name/download_url/duration）
//   - 取流：download_url 直链（免登录）
// 无歌词（独立音乐大多无词）；内容偏 CC 创作（非主流流行）。
var meta = {
  apiVersion: 1,
  key: "ccmixter",
  name: "ccMixter",
  capabilities: ["source"], // 免登录零版权（无歌词）
  allowedHosts: ["api.ccmixter.org", "ccmixter.org"],
  order: 80, // bilibili 10 / 网易云 20 / QQ 30 / 咪咕 40 / 酷我 50 / 千千 55 / Jamendo 60 / archive 65 / 酷狗 70 / Audius 75 之后
  settings: [
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
  ],
};

// 搜索：api/query（f=json，q=关键词）。
function search(ctx) {
  var url = "http://api.ccmixter.org/api/query?f=json&limit=" + ctx.size + "&offset=" + ((ctx.page - 1) * ctx.size) + "&q=" + encodeURIComponent(ctx.query);
  var resp = ctx.host.http({ url: url, headers: { "User-Agent": "mymusic/1.0" } });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  var d = JSON.parse(resp.body);
  var list = d.uploads || d || [];
  var songs = list.map(function (u) {
    var sec = parseInt(u.duration || 0, 10) || 0;
    return {
      id: u.download_url || (u.id ? String(u.id) : ""),
      title: u.upload_name || "",
      artist: u.user_name || "",
      cover: "",
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "ccmixter",
      album: "",
      vip: false, // CC 免费
    };
  }).filter(function (s) { return s.id; });
  return { total: songs.length, songs: songs };
}

// 取流：download_url 直链（免登录）。
function getURL(ctx) {
  var url = ctx.song.id;
  if (!url) return { error: "缺下载 URL" };
  if (!/^https?:/.test(url)) url = "http://" + url; // 补协议
  return { url: url, quality: 0 };
}

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
