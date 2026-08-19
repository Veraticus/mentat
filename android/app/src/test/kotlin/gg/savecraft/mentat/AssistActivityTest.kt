package gg.savecraft.mentat

import android.Manifest
import android.app.Application
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config

@Config(sdk = [35])
@RunWith(RobolectricTestRunner::class)
class AssistActivityTest {
    private lateinit var application: Application

    @Before
    fun resetPermission() {
        application = ApplicationProvider.getApplicationContext()
        Shadows.shadowOf(application).denyPermissions(Manifest.permission.RECORD_AUDIO)
    }

    @Test
    fun deniedRecordAudioPermissionDoesNotStartOrBindVoiceService() {
        Robolectric.buildActivity(AssistActivity::class.java).create()

        assertNull(Shadows.shadowOf(application).peekNextStartedService())
        assertTrue(Shadows.shadowOf(application).boundServiceConnections.isEmpty())
    }

    @Test
    fun grantedRecordAudioPermissionStartsVoiceService() {
        Shadows.shadowOf(application).grantPermissions(Manifest.permission.RECORD_AUDIO)

        Robolectric.buildActivity(AssistActivity::class.java).create()

        assertEquals(
            "gg.savecraft.mentat.session.VoiceSessionService",
            Shadows.shadowOf(application).peekNextStartedService().component?.className,
        )
    }
}
