package gg.savecraft.mentat.session

import android.content.Intent
import gg.savecraft.mentat.core.SessionState
import gg.savecraft.mentat.core.TokenEndpoint
import gg.savecraft.mentat.core.TokenFetchException
import gg.savecraft.mentat.core.TokenGrant
import gg.savecraft.mentat.core.TranscriptSegment
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@Config(sdk = [35])
@RunWith(RobolectricTestRunner::class)
class VoiceSessionServiceTest {
    private val scope = CoroutineScope(Dispatchers.Unconfined)

    @Test
    fun happyPathFetchesTokenConnectsAndPublishesMicrophone() = runBlocking {
        val liveKit = FakeLiveKitSession()
        val service = controller(liveKit)

        service.start()
        liveKit.events.emit(LiveKitEvent.Connected)

        assertEquals(SessionState.Live, service.state.value)
        assertEquals("wss://voice.example.com" to "token", liveKit.connection)
        assertTrue(liveKit.microphoneEnabled.value)
        assertTrue(service.micEnabled.value)
    }

    @Test
    fun tokenFailureFailsTheSession() = runBlocking {
        val service = controller(FakeLiveKitSession(), FailingTokenEndpoint)

        service.start()

        assertEquals(SessionState.Failed("Token request failed"), service.state.value)
    }

    @Test
    fun connectionFailureFailsTheSession() = runBlocking {
        val liveKit = FakeLiveKitSession(connectFailure = IllegalStateException("connection refused"))
        val service = controller(liveKit)

        service.start()

        assertEquals(SessionState.Failed("connection refused"), service.state.value)
    }

    @Test
    fun microphonePublishFailureAfterConnectionFailsTheSession() = runBlocking {
        val liveKit = FakeLiveKitSession(
            connectEvent = LiveKitEvent.Connected,
            setMicFailure = IllegalStateException("microphone rejected"),
        )
        val service = controller(liveKit)

        service.start()

        assertEquals(SessionState.Failed("microphone rejected"), service.state.value)
    }

    @Test
    fun startForegroundFailureFailsTheSessionAndStopsTheService() {
        FailingForegroundService.stopped = false
        val service = Robolectric.buildService(FailingForegroundService::class.java).create().get()

        service.onStartCommand(Intent(), 0, 1)

        assertEquals(SessionState.Failed("notification rejected"), service.state.value)
        assertTrue(FailingForegroundService.stopped)
    }

    @Test
    fun endDisconnectsClosesAndStopsTheService() = runBlocking {
        val liveKit = FakeLiveKitSession()
        var stopped = false
        val service = controller(liveKit, stopService = { stopped = true })

        service.start()
        liveKit.events.emit(LiveKitEvent.Connected)
        service.end()

        assertTrue(liveKit.closed)
        assertTrue(stopped)
        assertEquals(SessionState.Ended, service.state.value)
    }

    @Test
    fun destroyingTheServiceClosesTheLiveKitSession() {
        DestroyableVoiceSessionService.liveKit = FakeLiveKitSession()
        val service = Robolectric.buildService(DestroyableVoiceSessionService::class.java).create().get()

        service.onDestroy()

        assertTrue(DestroyableVoiceSessionService.liveKit.closed)
    }

    @Test
    fun terminalDisconnectFromLiveFailsTheSession() = runBlocking {
        val liveKit = FakeLiveKitSession()
        val service = controller(liveKit)

        service.start()
        liveKit.events.emit(LiveKitEvent.Connected)
        liveKit.events.emit(LiveKitEvent.Disconnected("server closed"))

        assertEquals(SessionState.Failed("server closed"), service.state.value)
    }

    @Test
    fun muteAndTranscriptionUpdateTheExposedFlows() = runBlocking {
        val liveKit = FakeLiveKitSession()
        val service = controller(liveKit)

        service.start()
        service.mute(true)
        liveKit.transcripts.emit(
            TranscriptSegment("one", "agent", "Hel", final = false),
        )
        liveKit.transcripts.emit(
            TranscriptSegment("one", "agent", "Hello", final = true),
        )

        assertFalse(liveKit.microphoneEnabled.value)
        assertFalse(service.micEnabled.value)
        assertEquals(
            listOf(TranscriptSegment("one", "agent", "Hello", final = true)),
            service.transcript.value,
        )
    }

    @Test
    fun muteFailureKeepsTheAuthoritativeMicrophoneState() = runBlocking {
        val liveKit = FakeLiveKitSession()
        val service = controller(liveKit)
        service.start()
        liveKit.setMicFailure = IllegalStateException("microphone rejected")

        service.mute(true)

        assertTrue(service.micEnabled.value)
        assertTrue(liveKit.microphoneEnabled.value)
    }

    private fun controller(
        liveKitSession: FakeLiveKitSession,
        tokenEndpoint: TokenEndpoint = FakeTokenEndpoint,
        stopService: () -> Unit = {},
    ) = VoiceSessionController(
        tokenEndpoint = tokenEndpoint,
        liveKitSession = liveKitSession,
        stopService = stopService,
        scope = scope,
    )

    private object FakeTokenEndpoint : TokenEndpoint {
        override fun fetch() = TokenGrant(
            token = "token",
            room = "room",
            url = "wss://voice.example.com",
            expiresAt = "2026-08-19T16:00:00Z",
        )
    }

    private object FailingTokenEndpoint : TokenEndpoint {
        override fun fetch(): TokenGrant = throw TokenFetchException("Token request failed")
    }

    class FakeLiveKitSession(
        private val connectEvent: LiveKitEvent? = null,
        private val connectFailure: Exception? = null,
        var setMicFailure: Exception? = null,
    ) : LiveKitSession {
        override val events = MutableSharedFlow<LiveKitEvent>()
        override val transcripts = MutableSharedFlow<TranscriptSegment>()
        val microphoneEnabled = MutableStateFlow(false)
        var connection: Pair<String, String>? = null
        var closed = false

        override suspend fun connect(url: String, token: String) {
            connectFailure?.let { throw it }
            connection = url to token
            connectEvent?.let { events.emit(it) }
        }

        override suspend fun setMicEnabled(enabled: Boolean) {
            setMicFailure?.let { throw it }
            microphoneEnabled.value = enabled
        }

        override suspend fun disconnect() = Unit

        override fun close() {
            closed = true
        }
    }

    class FailingForegroundService : VoiceSessionService() {
        override fun liveKitSession(): LiveKitSession = FakeLiveKitSession()

        override fun startForegroundNotification() {
            throw IllegalStateException("notification rejected")
        }

        override fun stopVoiceService() {
            stopped = true
        }

        companion object {
            var stopped = false
        }
    }

    class DestroyableVoiceSessionService : VoiceSessionService() {
        override fun liveKitSession(): LiveKitSession = liveKit

        companion object {
            lateinit var liveKit: FakeLiveKitSession
        }
    }
}
