package gg.savecraft.mentat.core

sealed interface SessionState {
    data object Idle : SessionState
    data object FetchingToken : SessionState
    data object Connecting : SessionState
    data object Live : SessionState
    data object Reconnecting : SessionState
    data object Ended : SessionState
    data class Failed(val reason: String) : SessionState
}

sealed interface SessionEvent {
    data object AssistInvoked : SessionEvent
    data class TokenReceived(val grant: TokenGrant) : SessionEvent
    data class TokenFailed(val reason: String) : SessionEvent
    data class ConnectFailed(val reason: String) : SessionEvent
    data object RoomConnected : SessionEvent
    data object ConnectionLost : SessionEvent
    data object Reconnected : SessionEvent
    data class ReconnectFailed(val reason: String) : SessionEvent
    data class ServiceStartFailed(val reason: String) : SessionEvent
    data object PermissionDenied : SessionEvent
    data object EndRequested : SessionEvent
}

class SessionStateMachine(initialState: SessionState = SessionState.Idle) {
    var state: SessionState = initialState
        private set

    fun transition(event: SessionEvent): SessionState {
        state = when (state) {
            SessionState.Idle -> when (event) {
                SessionEvent.AssistInvoked -> SessionState.FetchingToken
                is SessionEvent.ServiceStartFailed -> SessionState.Failed(event.reason)
                SessionEvent.PermissionDenied -> SessionState.Failed("Permission denied")
                else -> state
            }
            SessionState.FetchingToken -> when (event) {
                is SessionEvent.TokenReceived -> SessionState.Connecting
                is SessionEvent.TokenFailed -> SessionState.Failed(event.reason)
                is SessionEvent.ServiceStartFailed -> SessionState.Failed(event.reason)
                SessionEvent.PermissionDenied -> SessionState.Failed("Permission denied")
                else -> state
            }
            SessionState.Connecting -> when (event) {
                SessionEvent.RoomConnected -> SessionState.Live
                is SessionEvent.ConnectFailed -> SessionState.Failed(event.reason)
                is SessionEvent.ServiceStartFailed -> SessionState.Failed(event.reason)
                SessionEvent.PermissionDenied -> SessionState.Failed("Permission denied")
                else -> state
            }
            SessionState.Live -> when (event) {
                is SessionEvent.ConnectFailed -> SessionState.Failed(event.reason)
                SessionEvent.ConnectionLost -> SessionState.Reconnecting
                SessionEvent.EndRequested -> SessionState.Ended
                else -> state
            }
            SessionState.Reconnecting -> when (event) {
                SessionEvent.Reconnected -> SessionState.Live
                is SessionEvent.ReconnectFailed -> SessionState.Failed(event.reason)
                SessionEvent.EndRequested -> SessionState.Ended
                else -> state
            }
            SessionState.Ended,
            is SessionState.Failed,
            -> state
        }
        return state
    }
}
