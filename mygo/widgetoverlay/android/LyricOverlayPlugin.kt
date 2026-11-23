package dev.mygo

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import org.json.JSONObject

/** Go → Kotlin 桥：start / stop / granted / openPermissionSettings。 */
class LyricOverlayPlugin : MyGoPlugin {
    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        try {
            when (method) {
                "start" -> {
                    if (!canDraw()) {
                        reply.success(JSONObject().put("granted", false))
                        return
                    }
                    val ctx = PluginHost.context
                    val i = Intent(ctx, LyricOverlayService::class.java)
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) ctx.startForegroundService(i)
                    else ctx.startService(i)
                    reply.success(JSONObject().put("granted", true))
                }
                "stop" -> {
                    PluginHost.context.stopService(Intent(PluginHost.context, LyricOverlayService::class.java))
                    reply.success(JSONObject())
                }
                "granted" -> reply.success(JSONObject().put("granted", canDraw()))
                "openSettings" -> {
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
                        val i = Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION, Uri.parse("package:" + PluginHost.context.packageName))
                        i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                        PluginHost.context.startActivity(i)
                    }
                    reply.success(JSONObject())
                }
                else -> reply.failure(PluginFailure("NoMethod", method))
            }
        } catch (e: Exception) {
            reply.failure(PluginFailure("OverlayError", e.message ?: e.javaClass.simpleName))
        }
    }

    private fun canDraw(): Boolean =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.M || Settings.canDrawOverlays(PluginHost.context)
}
