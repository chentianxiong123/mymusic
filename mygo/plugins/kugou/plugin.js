// 酷狗音乐音源（免登录；搜索 song_search_v2 + 取流 trackercdn key=MD5(hash+kgcloudv2)）。
//   - 搜索：songsearch.kugou.com/song_search_v2（免登录，无签名）→ data.lists[]
//   - 取流：trackercdn.kugou.com/i/v2（appid=1001 + key=MD5(hash+"kgcloudv2")，免登录免费歌）
// 歌词后续（krcs.kugou.com 2 步未接）；VIP/版权歌 trackercdn 无 url。
var meta = {
  apiVersion: 1,
  key: "kugou",
  name: "酷狗音乐",
  capabilities: ["source"], // 免登录（歌词后续）
  allowedHosts: ["songsearch.kugou.com", "trackercdn.kugou.com"],
  order: 70, // bilibili 10 / 网易云 20 / QQ 30 / 咪咕 40 / 酷我 50 / 千千 55 / Jamendo 60 / archive 65 之后
  settings: [
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
  ],
};

// 搜索：song_search_v2（免登录）。
function search(ctx) {
  var url = "http://songsearch.kugou.com/song_search_v2?keyword=" + encodeURIComponent(ctx.query) +
    "&platform=WebFilter&format=json&page=" + ctx.page + "&pagesize=" + ctx.size +
    "&userid=-1&clientver=&tag=em&filter=2&iscorrection=1&privilege_filter=0&_=" + Date.now();
  var resp = ctx.host.http({ url: url, headers: kgHeaders(ctx) });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  var d = JSON.parse(resp.body);
  var lists = ((d.data || {}).lists || []);
  var songs = lists.map(function (it) {
    // 优先高品质 hash（SQ>HQ>Res>Ogg>File）；FileName="歌手 - 歌名"。
    var hash = it.SQFileHash || it.HQFileHash || it.ResFileHash || it.FileHash || "";
    var fn = it.FileName || "";
    var parts = fn.split(" - ");
    var artist = parts.length > 1 ? parts[0] : (it.SingerName || "");
    var title = parts.length > 1 ? parts.slice(1).join(" - ") : fn;
    var sec = parseInt(it.Duration || 0, 10) || 0;
    return {
      id: hash,
      title: title,
      artist: artist,
      cover: "",
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "kugou",
      album: it.AlbumName || "",
      vip: false, // 收费歌取流时判断（trackercdn 空）
    };
  });
  var total = parseInt((d.data || {}).total || songs.length, 10) || songs.length;
  return { total: total, songs: songs };
}

// 取流：trackercdn（key=MD5(hash+kgcloudv2)，免费歌匿名；VIP 无 url）。
function getURL(ctx) {
  var hash = ctx.song.id;
  if (!hash) return { error: "缺 hash" };
  var key = ctx.host.md5(hash + "kgcloudv2");
  var url = "https://trackercdn.kugou.com/i/v2/?cdnBackup=1&behavior=download&pid=1&cmd=21&appid=1001&hash=" +
    encodeURIComponent(hash) + "&key=" + key;
  var resp = ctx.host.http({ url: url, headers: kgHeaders(ctx) });
  if (resp.error) return { error: resp.error };
  var d = JSON.parse(resp.body);
  var audio = (d.data || {}).play_url || "";
  if (!audio) return { error: "酷狗无直链（VIP/版权受限）" };
  return { url: audio, quality: 0 };
}

// 请求头（酷狗匿名 UA）。
function kgHeaders(ctx) {
  return { "User-Agent": "Mozilla/5.0 (Linux; Android 10; SM-G981B) AppleWebKit/537.36 Chrome/80.0.3987.162 Mobile Safari/537.36" };
}

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
