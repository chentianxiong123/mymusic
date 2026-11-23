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
  key: "netease",
  name: "网易云",
  version: "1.0.0",
  // 网易云歌词已有内置 netease 歌词源（main.go RegisterLyric），此处不重复声明 lyric。
  capabilities: ["source"],
  allowedHosts: ["music.163.com"],
  order: 20, // bilibili 10 之后；歌词元数据 netease 同为 20，Type 不同不冲突
  // 插件设置声明（系统统一管理：声明归插件，存储归宿主 SettingStore）。
  settings: [
    { key: "quality", name: "音质", type: "select", options: ["128k", "320k"], default: "320k", desc: "播放音质（128k / 320k）" },
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
    { key: "cookie", name: "登录 Cookie", type: "text", default: "", desc: "粘贴网易云登录态 Cookie（浏览器 music.163.com 登录后复制），空=免登录 128k；有=无损/VIP" },
  ],
};

// 请求头：Referer + Cookie（匿名设备 id 防 405 风控；若有登录态 cookie 则追加，无损/VIP）。
function wyHeaders(ctx) {
  var c = "os=pc; appver=8.7.01; channel=netease";
  if (ctx && ctx.config && ctx.config.get) {
    var extra = ctx.config.get("cookie", "");
    if (extra) c += "; " + extra;
  }
  return { "Referer": "https://music.163.com/", "Cookie": c };
}

// 音源搜索：网易云明文 /api/search/get/web（免登录）。
function search(ctx) {
  var offset = (ctx.page - 1) * ctx.size;
  var resp = ctx.host.http({
    url: "https://music.163.com/api/search/get/web?s=" + encodeURIComponent(ctx.query) +
         "&type=1&offset=" + offset + "&limit=" + ctx.size + "&total=true",
    headers: wyHeaders(ctx),
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
      // 搜索接口 /api/search/get/web 不返回 album.picUrl（只有 picId + 歌手图 img1v1Url）。
      // 用歌手图兜底（实测可达 200）；播放时可选调 song/detail 补专辑封面（album.picUrl）。
      cover: ((s.artists && s.artists[0] && s.artists[0].img1v1Url) || ""),
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "netease",
      // 专辑名（album.name）+ 收费/会员标志（fee>0 = 需会员/付费，如屋顶 fee=8）。
      album: (s.album && s.album.name) || "",
      vip: (s.fee || 0) > 0,
    };
  });
  return { total: data.result.songCount || songs.length, songs: songs };
}

// 取流：网易云明文 /api/song/enhance/player/url（免登录，免费歌 128k）。
// VIP/无权限 → data[0].url 为 null → 返回 error。
function getURL(ctx) {
  var id = ctx.song.id;
  // 插件设置（系统统一管理）：host.config.get(key, def) 读设置，set 落库。
  var q = ctx.config && ctx.config.get ? ctx.config.get("quality", "320k") : "320k";
  var br = q === "128k" ? 128000 : 320000;
  var resp = ctx.host.http({
    url: "https://music.163.com/api/song/enhance/player/url?ids=[" + id + "]&br=" + br,
    headers: wyHeaders(ctx),
  });
  if (resp.error) return { error: resp.error };
  var data = JSON.parse(resp.body);
  var item = data.data && data.data[0];
  if (!item || !item.url) return { error: "无权限或需登录（VIP 歌免登录不可播）" };
  return { url: item.url, quality: q === "128k" ? 0 : 2, duration: (item.time || 0) / 1000 };
}

// 注：歌词用内置 netease 歌词源（keywords=标题+歌手 搜网易云，与 demo 同源），
// 播放时 loadLyricAsync 走主线 lyric 域，demo 无需也不能提供 lyric 能力。

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
