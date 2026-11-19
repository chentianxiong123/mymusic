// plugins/kuwo/plugin.js —— 酷我音乐音源（requiresCookie：无 cookie 不激活）
//
// 模式：cookie 可选（有则用自己的，无则匿名——但酷我取流 mobi.s 需 cookie/签名）。
//   - 搜索：search.kuwo.cn/r.s（匿名可用，返回 Python dict 单引号→转双引号解析）
//   - 取流：mobi.kuwo.cn/mobi.s（convert_url_with_sign，需 cookie）+ playUrl 降级（Secret 头）
var meta = {
  apiVersion: 1,
  key: "kuwo",
  name: "酷我音乐",
  version: "1.0.0",
  capabilities: ["source"],
  allowedHosts: ["search.kuwo.cn", "mobi.kuwo.cn", "www.kuwo.cn"],
  requiresCookie: true, // 酷我取流需登录 cookie（mobi.s convert_url_with_sign），无 cookie 不激活
  order: 50, // bilibili 10 / 网易云 20 / QQ 30 / 咪咕 40 之后
  settings: [
    { key: "quality", name: "音质", type: "select", options: ["128k", "320k"], default: "128k", desc: "取流码率（128kmp3/320kmp3；无损需 cookie）" },
    { key: "searchSize", name: "搜索条数", type: "select", options: ["20", "40", "60"], default: "40", desc: "每次搜索请求条数（插件自控，分页同尺寸）" },
    { key: "cookie", name: "登录 Cookie", type: "text", default: "", desc: "粘贴酷我登录态 Cookie（浏览器 www.kuwo.cn 登录后复制），空=不激活" },
  ],
};

// 请求头：Referer + 可选登录 cookie。
function kwHeaders(ctx) {
  var h = { "Referer": "http://www.kuwo.cn/" };
  if (ctx && ctx.config && ctx.config.get) {
    var extra = ctx.config.get("cookie", "");
    if (extra) h["Cookie"] = extra;
  }
  return h;
}

// 音源搜索：search.kuwo.cn/r.s（匿名可用；返回 Python dict 单引号 → 转双引号解析）。
function search(ctx) {
  var q = encodeURIComponent(ctx.query);
  var rn = ctx.size || 40;
  var pn = ((ctx.page || 1) - 1) * rn;
  var url = "http://search.kuwo.cn/r.s?all=" + q + "&ft=music&itemset=web_2013&client=kt&pcmp4=1&geo=c&vipver=1&pn=" + pn + "&rn=" + rn + "&rformat=json&encoding=utf8";
  var resp = ctx.host.http({ url: url, headers: { "Referer": "http://www.kuwo.cn/" } });
  if (resp.error) return { error: resp.error, total: 0, songs: [] };
  // 酷我 r.s 返回 Python dict（单引号），转双引号后 JSON.parse（同 music-lib parseKuwoLegacyJSON）。
  var body = resp.body.replace(/'/g, '"');
  var data = JSON.parse(body);
  var abslist = data.abslist || [];
  var songs = abslist.map(function (s) {
    var sec = parseInt(s.DURATION, 10) || 0;
    return {
      id: s.MUSICRID,
      title: cleanKw(s.SONGNAME),
      artist: cleanKw(s.ARTIST),
      cover: (s.albumpic || "").replace(/^http:/, "https:"),
      duration: fmtDur(sec),
      durationSec: sec,
      typename: "kuwo",
      album: cleanKw(s.ALBUM),
      vip: false, // 收费歌取流时判断（mobi.s 空）
    };
  });
  return { total: parseInt(data.TOTAL, 10) || songs.length, songs: songs };
}

// 取流：mobi.kuwo.cn/mobi.s（convert_url_with_sign，需 cookie）；降级 playUrl + Secret 头。
function getURL(ctx) {
  var rid = ctx.song.id;
  var cookie = (ctx.config && ctx.config.get) ? ctx.config.get("cookie", "") : "";
  var q = (ctx.config && ctx.config.get) ? ctx.config.get("quality", "128k") : "128k";
  var br = q === "320k" ? "320kmp3" : "128kmp3";
  // randomID 特定格式：C_APK_guanwang_{unixnano}_{rand}（对齐 music-lib fetchAudioURL）。
  var randomID = "C_APK_guanwang_" + Date.now() + "_" + Math.floor(Math.random() * 1000000);
  var url = "https://mobi.kuwo.cn/mobi.s?f=web&source=kwplayercar_ar_6.0.0.9_B_jiakong_vh.apk&from=PC&type=convert_url_with_sign&br=" + br + "&rid=" + encodeURIComponent(rid) + "&user=" + randomID;
  var headers = { "Referer": "http://www.kuwo.cn/" };
  if (cookie) headers["Cookie"] = cookie;
  var resp = ctx.host.http({ url: url, headers: headers });
  if (!resp.error) {
    try {
      var d = JSON.parse(resp.body);
      var u = (d.data && d.data.url) || "";
      // 4011 返回 data.url="None"（无效），排除；真直链通常含 http。
      if (u && u !== "None" && u.indexOf("http") === 0) return { url: u, quality: q === "320k" ? 2 : 0 };
    } catch (e) {}
  }
  // 降级：www.kuwo.cn/api playUrl + Secret 头（绕过防盗链）。
  var fUrl = "https://www.kuwo.cn/api/v1/www/music/playUrl?mid=" + encodeURIComponent(rid) + "&type=music&httpsStatus=1";
  var ck = cookie || "";
  if (ck.indexOf("kw_token") < 0) ck += "; kw_token=secret_token";
  var fResp = ctx.host.http({ url: fUrl, headers: { "Referer": "https://www.kuwo.cn/", "Secret": "kuwo_web_secret", "Cookie": ck } });
  if (!fResp.error) {
    try {
      var fd = JSON.parse(fResp.body);
      if (fd.data && fd.data.url && fd.data.url.indexOf("http") === 0) return { url: fd.data.url };
    } catch (e) {}
  }
  return { error: "酷我取流失败（需登录 cookie 或版权受限）" };
}

// 清理酷我文本（&nbsp; HTML 空格 → 空格）。
function cleanKw(s) {
  return String(s || "").replace(/&nbsp;/g, " ").replace(/&amp;/g, "&");
}

// 工具：秒 → mm:ss。
function fmtDur(sec) {
  var m = Math.floor(sec / 60), s = sec % 60;
  return m + ":" + (s < 10 ? "0" : "") + s;
}
