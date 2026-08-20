package gg.savecraft.mentat.session

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import gg.savecraft.mentat.BuildConfig
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@Config(sdk = [35])
@RunWith(RobolectricTestRunner::class)
class AppSettingsTest {
    private lateinit var context: Context

    @Before
    fun clearSettings() {
        context = ApplicationProvider.getApplicationContext()
        context.getSharedPreferences("mentat-settings", Context.MODE_PRIVATE).edit().clear().commit()
    }

    @Test
    fun usesBuildConfigDefaultWhenNoOverrideIsSaved() {
        assertEquals(BuildConfig.MENTAT_TOKEN_ENDPOINT, AppSettings(context).tokenEndpointUrl)
    }

    @Test
    fun savesAndReturnsTokenEndpointOverride() {
        val settings = AppSettings(context)

        settings.saveTokenEndpointUrl(" https://voice.example.com/token ")

        assertEquals("https://voice.example.com/token", settings.tokenEndpointUrl)
    }

    @Test
    fun blankOverrideRestoresBuildConfigDefault() {
        val settings = AppSettings(context)
        settings.saveTokenEndpointUrl("https://voice.example.com/token")

        settings.saveTokenEndpointUrl("   ")

        assertEquals(BuildConfig.MENTAT_TOKEN_ENDPOINT, settings.tokenEndpointUrl)
    }
}
