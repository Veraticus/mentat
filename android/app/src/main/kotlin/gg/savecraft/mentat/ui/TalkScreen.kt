package gg.savecraft.mentat.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.unit.dp
import gg.savecraft.mentat.core.SessionState
import gg.savecraft.mentat.core.TranscriptSegment
import gg.savecraft.mentat.session.AppSettings

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TalkScreen(
    state: SessionState,
    transcript: List<TranscriptSegment>,
    muted: Boolean,
    settings: AppSettings,
    onMuteChanged: (Boolean) -> Unit,
    onEnd: () -> Unit,
) {
    var editingSettings by remember { mutableStateOf(false) }
    var endpointUrl by remember { mutableStateOf(settings.tokenEndpointUrl) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(state.title()) },
                actions = {
                    TextButton(onClick = { editingSettings = !editingSettings }) {
                        Text("Settings")
                    }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(
                text = state.detail(),
                style = MaterialTheme.typography.titleMedium,
            )
            if (editingSettings) {
                OutlinedTextField(
                    value = endpointUrl,
                    onValueChange = { endpointUrl = it },
                    modifier = Modifier.fillMaxWidth(),
                    label = { Text("Token endpoint") },
                    singleLine = true,
                )
                Button(onClick = {
                    settings.saveTokenEndpointUrl(endpointUrl)
                    editingSettings = false
                }) {
                    Text("Save")
                }
            }
            LazyColumn(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth(),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                items(transcript, key = { it.id }) { segment ->
                    Caption(segment)
                }
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Button(
                    onClick = { onMuteChanged(!muted) },
                    modifier = Modifier.weight(1f),
                ) {
                    Text(if (muted) "Unmute" else "Mute")
                }
                Button(
                    onClick = onEnd,
                    modifier = Modifier.weight(1f),
                ) {
                    Text("End")
                }
            }
        }
    }
}

@Composable
private fun Caption(segment: TranscriptSegment) {
    Column(
        modifier = Modifier.alpha(if (segment.final) 1f else 0.65f),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Text(
            text = segment.participantIdentity,
            style = MaterialTheme.typography.labelMedium,
        )
        Text(
            text = segment.text,
            style = MaterialTheme.typography.bodyLarge,
            fontStyle = if (segment.final) FontStyle.Normal else FontStyle.Italic,
        )
    }
}

private fun SessionState.title(): String = when (this) {
    SessionState.Idle,
    SessionState.FetchingToken,
    SessionState.Connecting,
    -> "Connecting"
    SessionState.Live -> "Live"
    SessionState.Reconnecting -> "Reconnecting"
    is SessionState.Failed -> "Failed"
    SessionState.Ended -> "Ended"
}

private fun SessionState.detail(): String = when (this) {
    is SessionState.Failed -> reason
    SessionState.Live -> "Listening"
    SessionState.Reconnecting -> "Restoring connection"
    SessionState.Ended -> "Session ended"
    else -> "Starting voice session"
}
