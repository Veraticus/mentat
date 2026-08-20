package gg.savecraft.mentat.session

import android.content.Context
import gg.savecraft.mentat.BuildConfig

class AppSettings(context: Context) {
    private val preferences = context.getSharedPreferences(PREFERENCES_NAME, Context.MODE_PRIVATE)

    val tokenEndpointUrl: String
        get() = preferences.getString(TOKEN_ENDPOINT_KEY, BuildConfig.MENTAT_TOKEN_ENDPOINT)
            ?: BuildConfig.MENTAT_TOKEN_ENDPOINT

    fun saveTokenEndpointUrl(value: String) {
        val endpointUrl = value.trim()
        val editor = preferences.edit()
        if (endpointUrl.isEmpty()) {
            editor.remove(TOKEN_ENDPOINT_KEY)
        } else {
            editor.putString(TOKEN_ENDPOINT_KEY, endpointUrl)
        }
        editor.apply()
    }

    private companion object {
        const val PREFERENCES_NAME = "mentat-settings"
        const val TOKEN_ENDPOINT_KEY = "token-endpoint-url"
    }
}
