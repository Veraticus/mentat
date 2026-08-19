package gg.savecraft.mentat

import android.Manifest
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.content.pm.PackageManager
import android.os.Bundle
import android.os.IBinder
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import gg.savecraft.mentat.core.SessionEvent
import gg.savecraft.mentat.core.SessionState
import gg.savecraft.mentat.core.SessionStateMachine
import gg.savecraft.mentat.core.TranscriptSegment
import gg.savecraft.mentat.session.AppSettings
import gg.savecraft.mentat.session.VoiceSessionService
import gg.savecraft.mentat.ui.TalkScreen
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch

class AssistActivity : ComponentActivity() {
    private val activityScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val uiState = MutableStateFlow<SessionState>(SessionState.Idle)
    private val uiTranscript = MutableStateFlow<List<TranscriptSegment>>(emptyList())
    private val uiMicEnabled = MutableStateFlow(false)
    private lateinit var settings: AppSettings
    private var service: VoiceSessionService? = null
    private var bound = false
    private var stateJob: Job? = null
    private var transcriptJob: Job? = null
    private var micJob: Job? = null

    private val permissionRequest = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { granted ->
        if (granted) {
            startAndBindVoiceService()
        } else {
            uiState.value = SessionStateMachine().transition(SessionEvent.PermissionDenied)
        }
    }

    private val connection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName, binder: IBinder) {
            service = (binder as VoiceSessionService.LocalBinder).service()
            stateJob = activityScope.launch {
                service!!.state.collect { uiState.value = it }
            }
            transcriptJob = activityScope.launch {
                service!!.transcript.collect { uiTranscript.value = it }
            }
            micJob = activityScope.launch {
                service!!.micEnabled.collect { uiMicEnabled.value = it }
            }
        }

        override fun onServiceDisconnected(name: ComponentName) {
            stateJob?.cancel()
            transcriptJob?.cancel()
            micJob?.cancel()
            service = null
            bound = false
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        settings = AppSettings(this)
        logAssistIntent()
        setContent {
            val state by uiState.collectAsState()
            val transcript by uiTranscript.collectAsState()
            val micEnabled by uiMicEnabled.collectAsState()
            MaterialTheme {
                TalkScreen(
                    state = state,
                    transcript = transcript,
                    muted = !micEnabled,
                    settings = settings,
                    onMuteChanged = { muted -> service?.mute(muted) },
                    onEnd = {
                        service?.end()
                        finish()
                    },
                )
            }
        }
        beginVoiceSession()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        logAssistIntent()
        beginVoiceSession()
    }

    override fun onDestroy() {
        if (bound) {
            unbindService(connection)
        }
        activityScope.cancel()
        super.onDestroy()
    }

    private fun beginVoiceSession() {
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED) {
            startAndBindVoiceService()
        } else {
            permissionRequest.launch(Manifest.permission.RECORD_AUDIO)
        }
    }

    private fun startAndBindVoiceService() {
        if (bound) {
            return
        }
        val intent = Intent(this, VoiceSessionService::class.java)
        startForegroundService(intent)
        bound = bindService(intent, connection, Context.BIND_AUTO_CREATE)
    }

    private fun logAssistIntent() {
        Log.i("MentatAssist", "MENTAT_ASSIST_RECEIVED action=" + intent.action)
    }
}
