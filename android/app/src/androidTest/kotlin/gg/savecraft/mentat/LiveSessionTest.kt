package gg.savecraft.mentat

import androidx.test.platform.app.InstrumentationRegistry
import gg.savecraft.mentat.core.HttpTokenEndpoint
import io.livekit.android.LiveKit
import io.livekit.android.room.Room
import io.livekit.android.room.datastream.StreamTextOptions
import io.livekit.android.room.participant.Participant
import io.livekit.android.room.participant.RemoteParticipant
import io.livekit.android.room.track.RemoteAudioTrack
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertTrue
import org.junit.Test

class LiveSessionTest {
    @Test
    fun joinsAgentAndReceivesTranscriptionAfterChatMessage() = runBlocking {
        val testScope = this
        val room = LiveKit.create(InstrumentationRegistry.getInstrumentation().targetContext)
        try {
            val endpoint = requireTokenEndpoint()
            val grant = stage(TOKEN_FETCH_STAGE) {
                withContext(Dispatchers.IO) {
                    HttpTokenEndpoint(endpoint).fetch()
                }
            }
            val transcription = CompletableDeferred<String>()
            room.registerTextStreamHandler(TRANSCRIPTION_TOPIC) { reader, _ ->
                testScope.launch { transcription.complete(reader.flow.first()) }
            }

            stage(CONNECT_STAGE) { room.connect(grant.url, grant.token) }
            val agent = stage(AGENT_STAGE) { awaitAgent(room) }
            stage(AUDIO_TRACK_STAGE) { awaitAudioTrack(agent) }
            stage(CHAT_STAGE) {
                room.localParticipant
                    .sendText(CHAT_MESSAGE, StreamTextOptions(topic = CHAT_TOPIC))
                    .getOrThrow()
            }
            val reply = stage(TRANSCRIPTION_STAGE) { transcription.await() }
            assertTrue("$TRANSCRIPTION_STAGE produced an empty text stream", reply.isNotBlank())
        } finally {
            room.disconnect()
            room.release()
        }
    }

    private fun requireTokenEndpoint(): String = InstrumentationRegistry.getArguments()
        .getString(TOKEN_ENDPOINT_ARGUMENT)
        ?.takeIf(String::isNotBlank)
        ?: throw AssertionError(
            "Missing instrumentation argument $TOKEN_ENDPOINT_ARGUMENT; " +
                "run just android-live <mentat token endpoint>",
        )

    private suspend fun awaitAgent(room: Room): RemoteParticipant = awaitValue {
        room.remoteParticipants.values.firstOrNull { it.kind == Participant.Kind.AGENT }
    }

    private suspend fun awaitAudioTrack(agent: RemoteParticipant): RemoteAudioTrack = awaitValue {
        agent.audioTrackPublications
            .firstOrNull { (publication, track) -> publication.subscribed && track is RemoteAudioTrack }
            ?.second as? RemoteAudioTrack
    }

    private suspend fun <T> awaitValue(value: () -> T?): T {
        while (true) {
            value()?.let { return it }
            delay(POLL_INTERVAL_MS)
        }
    }

    private suspend fun <T> stage(name: String, block: suspend () -> T): T = try {
        withTimeout(STAGE_TIMEOUT_MS) { block() }
    } catch (error: Throwable) {
        throw AssertionError("Live session failed at $name: ${error.message}", error)
    }

    private companion object {
        const val TOKEN_ENDPOINT_ARGUMENT = "mentatTokenEndpoint"
        const val TOKEN_FETCH_STAGE = "token-fetch stage"
        const val CONNECT_STAGE = "LiveKit connect stage"
        const val AGENT_STAGE = "agent participant stage"
        const val AUDIO_TRACK_STAGE = "agent audio subscription stage"
        const val CHAT_STAGE = "lk.chat send stage"
        const val TRANSCRIPTION_STAGE = "lk.transcription receive stage"
        const val TRANSCRIPTION_TOPIC = "lk.transcription"
        const val CHAT_TOPIC = "lk.chat"
        const val CHAT_MESSAGE = "Please reply with a short greeting for the Android instrumentation test."
        const val STAGE_TIMEOUT_MS = 30_000L
        const val POLL_INTERVAL_MS = 100L
    }
}
