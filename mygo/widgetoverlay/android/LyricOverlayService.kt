package dev.mygo

// 安卓悬浮歌词挂件：从 wails3 LyricOverlayService.java 直接移植。
// TYPE_APPLICATION_OVERLAY + TextView，SSE 订阅 Go 后端 127.0.0.1:28977
// /api/v1/widget/state + /api/v1/widget/events（mygo stream_server 已挂），
// 显示层纯渲染（line 由后端 PickLine 算好）。

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.content.pm.ServiceInfo
import android.graphics.Color
import android.graphics.PixelFormat
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.util.Log
import android.view.Gravity
import android.view.MotionEvent
import android.view.View
import android.view.WindowManager
import android.widget.LinearLayout
import android.widget.TextView
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.nio.charset.StandardCharsets

class LyricOverlayService : Service() {
    companion object {
        private const val TAG = "MyGoLyricOverlay"
        private const val API = "http://127.0.0.1:28977"
        private const val CHANNEL_ID = "mygo_lyric_overlay"
        private const val NOTIFICATION_ID = 0x4C79
        private const val PREFS = "mygo_lyric_overlay"
        private const val POLL_MS = 500L
    }

    private lateinit var windowManager: WindowManager
    private var rootView: View? = null
    private var lyricView: TextView? = null
    private var params: WindowManager.LayoutParams? = null
    private var ctrl: LinearLayout? = null
    private var btnPause: TextView? = null
    private var rootBg: GradientDrawable? = null
    private var prefs: SharedPreferences? = null
    private var running = false
    private var pollThread: Thread? = null
    private val ui = Handler(Looper.getMainLooper())
    // 歌词字号档位（0-4 对应 14/16/18/22/26，×1.5 挂件基线）；−/+ 按钮调它，持久化。
    private var lyricSize = 2
    /** 面板收起延时（触摸后给足时间看清/连点）。之前 600ms 太快。 */
    private val hideDelayMs = 3500L

    /** 重置面板收起计时。 */
    private fun touchPanel() {
        ui.removeCallbacksAndMessages(null)
        rootView?.let { v ->
            ui.postDelayed({
                v.background = null
                ctrl?.visibility = View.GONE
            }, hideDelayMs)
        }
    }

    /** −/+ 调字号档位（不是歌词时间偏移！），更新 TextView 并持久化。 */
    private fun changeLyricSize(delta: Int) {
        val sizes = listOf(14f, 16f, 18f, 22f, 26f)
        lyricSize = (lyricSize + delta).coerceIn(0, sizes.size - 1)
        prefs?.edit()?.putInt("lyricsize", lyricSize)?.apply()
        lyricView?.textSize = sizes[lyricSize] * 1.5f
        touchPanel()
    }

    override fun onCreate() {
        super.onCreate()
        if (rootView != null) {
            startForegroundCompat()
            running = true
            return
        }
        prefs = getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        windowManager = getSystemService(WINDOW_SERVICE) as WindowManager
        createOverlayView()
        startForegroundCompat()
        running = true
        pollThread = Thread { sseLoop() }.also { it.start() }
        Log.i(TAG, "LyricOverlayService created")
    }

    private fun dp(v: Int) = Math.round(resources.displayMetrics.density * v)

    private fun createOverlayView() {
        val lyric = TextView(this).apply {
            text = "MyMusic 歌词"
            textSize = 27f // 默认档 18×1.5；onCreate 会按持久化档位覆盖
            setTextColor(Color.WHITE)
            setShadowLayer(3f, 0f, 1f, Color.argb(200, 0, 0, 0))
            typeface = Typeface.DEFAULT_BOLD
            gravity = Gravity.CENTER
            setPadding(dp(14), dp(8), dp(14), dp(8))
        }
        lyricView = lyric
        lyricSize = (prefs?.getInt("lyricsize", 2) ?: 2).coerceIn(0, 4)
        lyric.textSize = listOf(14f, 16f, 18f, 22f, 26f)[lyricSize] * 1.5f

        val bar = LinearLayout(this).apply { orientation = LinearLayout.HORIZONTAL; gravity = Gravity.CENTER; visibility = View.GONE }
        btnPause = ctrlBtn("⏸").also { b ->
            b.setOnClickListener { touchPanel(); postJson("/api/v1/widget/toggle", "{}") }
            bar.addView(b)
        }
        val btnMinus = ctrlBtn("−").also { b ->
            b.setOnClickListener { changeLyricSize(-1) } // 缩小歌词字号
            bar.addView(b)
        }
        val btnPlus = ctrlBtn("+").also { b ->
            b.setOnClickListener { changeLyricSize(1) } // 放大歌词字号
            bar.addView(b)
        }
        ctrlBtn("✕").also { b ->
            b.setOnClickListener { stopSelf() }
            bar.addView(b)
        }
        ctrl = bar

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            addView(lyric)
            addView(bar)
            setPadding(dp(6), dp(6), dp(6), dp(6))
            background = null
        }
        rootBg = GradientDrawable().apply {
            setColor(Color.argb(105, 12, 12, 20))
            cornerRadius = dp(14).toFloat()
            setStroke(dp(1), Color.argb(130, 255, 255, 255))
        }

        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O)
            WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY
        else WindowManager.LayoutParams.TYPE_PHONE
        params = WindowManager.LayoutParams(
            WindowManager.LayoutParams.WRAP_CONTENT,
            WindowManager.LayoutParams.WRAP_CONTENT,
            type,
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL,
            PixelFormat.TRANSLUCENT
        ).apply {
            gravity = Gravity.TOP or Gravity.START
            x = prefs?.getInt("x", 40) ?: 40
            y = prefs?.getInt("y", 200) ?: 200
        }
        rootView = root
        try { windowManager.removeView(root) } catch (_: Exception) {}
        params?.let { windowManager.addView(root, it) }
        attachDrag(root)
    }

    private fun ctrlBtn(text: String): TextView = TextView(this).apply {
        setText(text)
        textSize = 20f
        setTextColor(Color.WHITE)
        gravity = Gravity.CENTER
        setPadding(dp(14), dp(7), dp(14), dp(7))
        setOnTouchListener { v, ev ->
            when (ev.actionMasked) {
                MotionEvent.ACTION_DOWN -> v.setBackgroundColor(Color.argb(60, 255, 255, 255))
                MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> v.background = null
            }
            false
        }
    }

    private fun postJson(path: String, body: String) {
        Thread {
            try {
                val conn = URL(API + path).openConnection() as HttpURLConnection
                conn.requestMethod = "POST"
                conn.setRequestProperty("Content-Type", "application/json")
                conn.doOutput = true
                conn.outputStream.write(body.toByteArray(StandardCharsets.UTF_8))
                conn.responseCode
                conn.disconnect()
            } catch (e: Exception) {
                Log.d(TAG, "postJson error: ${e.javaClass.simpleName}")
            }
        }.start()
    }

    private fun attachDrag(target: View) {
        val down = FloatArray(2)
        target.setOnTouchListener { v, ev ->
            when (ev.actionMasked) {
                MotionEvent.ACTION_DOWN -> {
                    rootBg?.let { v.background = it }
                    ctrl?.visibility = View.VISIBLE
                    down[0] = ev.rawX - (params?.x ?: 0)
                    down[1] = ev.rawY - (params?.y ?: 0)
                    true
                }
                MotionEvent.ACTION_MOVE -> {
                    params?.let {
                        it.x = (ev.rawX - down[0]).toInt()
                        it.y = (ev.rawY - down[1]).toInt()
                        rootView?.let { rv -> windowManager.updateViewLayout(rv, it) }
                    }
                    true
                }
                MotionEvent.ACTION_UP -> {
                    params?.let { prefs?.edit()?.putInt("x", it.x)?.putInt("y", it.y)?.apply() }
                    touchPanel() // 3.5s 后收起（之前 600ms 太快）
                    true
                }
                else -> false
            }
        }
    }

    private fun startForegroundCompat() {
        val nm = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            nm.createNotificationChannel(NotificationChannel(CHANNEL_ID, "歌词悬浮窗", NotificationManager.IMPORTANCE_LOW))
        }
        var contentIntent: PendingIntent? = null
        packageManager.getLaunchIntentForPackage(packageName)?.let { launch ->
            val flags = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) PendingIntent.FLAG_IMMUTABLE else 0
            contentIntent = PendingIntent.getActivity(this, 0, launch, flags)
        }
        val n = Notification.Builder(this, if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) CHANNEL_ID else "lyric")
            .setSmallIcon(android.R.drawable.ic_media_play)
            .setContentTitle("MyMusic 歌词")
            .setContentText("悬浮歌词已开启")
            .setOngoing(true)
            .setContentIntent(contentIntent)
            .build()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(NOTIFICATION_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            startForeground(NOTIFICATION_ID, n)
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int = START_STICKY

    override fun onDestroy() {
        running = false
        pollThread?.interrupt()
        try { rootView?.let { windowManager.removeView(it) } } catch (_: Exception) {}
        super.onDestroy()
        Log.i(TAG, "LyricOverlayService destroyed")
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun sseLoop() {
        while (running) {
            try {
                val conn = URL(API + "/api/v1/widget/events").openConnection() as HttpURLConnection
                conn.setRequestProperty("Accept", "text/event-stream")
                conn.connectTimeout = 3000
                conn.readTimeout = 0
                conn.doInput = true
                val br = BufferedReader(InputStreamReader(conn.inputStream, StandardCharsets.UTF_8))
                try {
                    val state = getJson("/api/v1/widget/state")
                    state?.optJSONObject("data")?.let { applyState(it) }
                } catch (_: Exception) {}
                var event = ""
                var line: String? = null
                while (running) {
                    line = br.readLine() ?: break
                    when {
                        line!!.startsWith("event:") -> event = line!!.substring(6).trim()
                        line!!.startsWith("data:") -> {
                            val data = line!!.substring(5).trim()
                            if (event == "lyric-state" && data.isNotEmpty()) applyState(JSONObject(data))
                            event = ""
                        }
                        line!!.isEmpty() -> event = ""
                    }
                }
                br.close()
                conn.disconnect()
            } catch (e: Exception) {
                Log.d(TAG, "sse error: ${e.javaClass.simpleName}")
            }
            try { Thread.sleep(POLL_MS) } catch (_: InterruptedException) { break }
        }
    }

    private fun applyState(d: JSONObject) {
        var line = d.optString("line", "")
        val title = d.optString("title", "")
        val artist = d.optString("artist", "")
        if (line.isEmpty()) line = if (title.isEmpty()) "MyMusic 歌词" else "$title - $artist"
        val finalLine = line
        val playing = d.optBoolean("playing", true)
        ui.post {
            lyricView?.text = finalLine
            btnPause?.text = if (playing) "⏸" else "▶️"
        }
    }

    private fun getJson(path: String): JSONObject? {
        var conn: HttpURLConnection? = null
        try {
            conn = URL(API + path).openConnection() as HttpURLConnection
            conn.connectTimeout = 1000
            conn.readTimeout = 1000
            conn.requestMethod = "GET"
            if (conn.responseCode != 200) return null
            BufferedReader(InputStreamReader(conn.inputStream, StandardCharsets.UTF_8)).use { br ->
                val sb = StringBuilder()
                while (true) sb.append(br.readLine() ?: break)
                return JSONObject(sb.toString())
            }
        } catch (_: Exception) {
            return null
        } finally {
            conn?.disconnect()
        }
    }
}
