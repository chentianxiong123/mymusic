// Internet Archive 音源（archive.org 免登录公开 API + 直链，零版权）。
//   - 搜索：advancedsearch.php（mediatype:audio AND title/creator）→ response.docs[]
//   - 取流：metadata/{identifier} → files[]（音频）→ download/{identifier}/{file}
// 无歌词（音频无词）；内容偏老歌/现场/有声书（etree/audiopoetry/78rpm）。
var meta = {
  apiVersion: 1,
  key: "archive",
  name: "Internet Archive",
  capabilities: ["source"], // 免登录零版权（无歌词）
  allowedHosts: ["archive.org"],
  order: 65, // bilibili 10 / 网易云 20 / QQ 30 / 咪咕 40 / 酷我 50 / 千千 55 / Jamendo 60 之后
  settings: [
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
  ],
};

// 搜索：advancedsearch（title/creator 关键词，mediatype:audio）。
function search(ctx) {
  var q = 'mediatype:audio AND (title:"' + ctx.query + '" OR creator:"' + ctx.query + '")';
  var url = "https://archive.org/advancedsearch.php?q=" + encodeURIComponent(q) +
    "&fl[]=identifier&fl[]=title&fl[]=creator&rows=" + ctx.size + "&page=" + ctx.page + "&output=json";
  var resp = ctx.host.http({ url: url, headers: { "User-Agent": "mymusic/1.0" } });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  var d = JSON.parse(resp.body);
  var docs = ((d.response || {}).docs || []);
  var songs = docs.map(function (x) {
    var creator = x.creator || [];
    if (!Array.isArray(creator)) creator = [creator];
    return {
      id: x.identifier,
      title: x.title || "",
      artist: creator.filter(Boolean).join(","),
      cover: "",
      duration: "",
      durationSec: 0,
      typename: "archive",
      album: "",
      vip: false, // 零版权
    };
  });
  var total = parseInt((d.response || {}).numFound || songs.length, 10) || songs.length;
  return { total: total, songs: songs };
}

// 取流：metadata → files[]（音频）→ download 直链。
function getURL(ctx) {
  var id = ctx.song.id;
  var mresp = ctx.host.http({ url: "https://archive.org/metadata/" + encodeURIComponent(id), headers: { "User-Agent": "mymusic/1.0" } });
  if (mresp.error) return { error: mresp.error };
  var md = JSON.parse(mresp.body);
  var files = (md.files || []);
  var f = files.filter(function (x) {
    return /\.(mp3|flac|ogg|wav|m4a)$/i.test(x.name || "") || /^(mp3|flac|ogg|wav|m4a)$/i.test(String(x.format || "").toLowerCase());
  })[0];
  if (!f) return { error: "Internet Archive 无音频文件" };
  return { url: "https://archive.org/download/" + encodeURIComponent(id) + "/" + encodeURIComponent(f.name), quality: 0 };
}
