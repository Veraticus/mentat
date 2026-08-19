package gg.savecraft.mentat.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class SessionStateMachineTest {
    private val grant = TokenGrant(
        token = "token",
        room = "room",
        url = "wss://voice.example.com",
        expiresAt = "2026-08-19T16:00:00Z",
    )

    @Test
    fun happyPathMovesFromIdleToLive() {
        val machine = SessionStateMachine()

        assertEquals(SessionState.FetchingToken, machine.transition(SessionEvent.AssistInvoked))
        assertEquals(SessionState.Connecting, machine.transition(SessionEvent.TokenReceived(grant)))
        assertEquals(SessionState.Live, machine.transition(SessionEvent.RoomConnected))
    }

    @Test
    fun connectionLossReconnectsToLive() {
        val machine = SessionStateMachine(SessionState.Live)

        assertEquals(SessionState.Reconnecting, machine.transition(SessionEvent.ConnectionLost))
        assertEquals(SessionState.Live, machine.transition(SessionEvent.Reconnected))
    }

    @Test
    fun reconnectFailureMovesToFailed() {
        val machine = SessionStateMachine(SessionState.Live)
        machine.transition(SessionEvent.ConnectionLost)

        assertEquals(
            SessionState.Failed("reconnect failed"),
            machine.transition(SessionEvent.ReconnectFailed("reconnect failed")),
        )
    }

    @Test
    fun tokenFailureMovesToFailed() {
        val machine = SessionStateMachine(SessionState.FetchingToken)

        assertEquals(
            SessionState.Failed("token failed"),
            machine.transition(SessionEvent.TokenFailed("token failed")),
        )
    }

    @Test
    fun connectionFailureFailsConnectingAndLiveStates() {
        listOf(SessionState.Connecting, SessionState.Live).forEach { initialState ->
            assertEquals(
                SessionState.Failed("connection failed"),
                SessionStateMachine(initialState).transition(
                    SessionEvent.ConnectFailed("connection failed"),
                ),
            )
        }
    }

    @Test
    fun permissionDeniedFailsEveryPreLiveState() {
        val preLiveStates = listOf(
            SessionState.Idle,
            SessionState.FetchingToken,
            SessionState.Connecting,
        )

        preLiveStates.forEach { initialState ->
            val result = SessionStateMachine(initialState).transition(SessionEvent.PermissionDenied)
            assertTrue("$initialState did not fail", result is SessionState.Failed)
        }
    }

    @Test
    fun serviceStartFailureFailsEveryPreLiveState() {
        val preLiveStates = listOf(
            SessionState.Idle,
            SessionState.FetchingToken,
            SessionState.Connecting,
        )

        preLiveStates.forEach { initialState ->
            assertEquals(
                SessionState.Failed("service failed"),
                SessionStateMachine(initialState).transition(
                    SessionEvent.ServiceStartFailed("service failed"),
                ),
            )
        }
    }

    @Test
    fun endRequestEndsLiveAndReconnectingStates() {
        listOf(SessionState.Live, SessionState.Reconnecting).forEach { initialState ->
            assertEquals(
                SessionState.Ended,
                SessionStateMachine(initialState).transition(SessionEvent.EndRequested),
            )
        }
    }

    @Test
    fun everyIllegalEventLeavesStateUnchanged() {
        val states = listOf(
            SessionState.Idle,
            SessionState.FetchingToken,
            SessionState.Connecting,
            SessionState.Live,
            SessionState.Reconnecting,
            SessionState.Ended,
            SessionState.Failed("existing failure"),
        )
        val events = listOf(
            SessionEvent.AssistInvoked,
            SessionEvent.TokenReceived(grant),
            SessionEvent.TokenFailed("token failed"),
            SessionEvent.ConnectFailed("connection failed"),
            SessionEvent.RoomConnected,
            SessionEvent.ConnectionLost,
            SessionEvent.Reconnected,
            SessionEvent.ReconnectFailed("reconnect failed"),
            SessionEvent.ServiceStartFailed("service failed"),
            SessionEvent.PermissionDenied,
            SessionEvent.EndRequested,
        )
        val legalTransitions = setOf(
            SessionState.Idle to SessionEvent.AssistInvoked,
            SessionState.Idle to SessionEvent.ServiceStartFailed("service failed"),
            SessionState.Idle to SessionEvent.PermissionDenied,
            SessionState.FetchingToken to SessionEvent.TokenReceived(grant),
            SessionState.FetchingToken to SessionEvent.TokenFailed("token failed"),
            SessionState.FetchingToken to SessionEvent.ServiceStartFailed("service failed"),
            SessionState.FetchingToken to SessionEvent.PermissionDenied,
            SessionState.Connecting to SessionEvent.RoomConnected,
            SessionState.Connecting to SessionEvent.ConnectFailed("connection failed"),
            SessionState.Connecting to SessionEvent.ServiceStartFailed("service failed"),
            SessionState.Connecting to SessionEvent.PermissionDenied,
            SessionState.Live to SessionEvent.ConnectFailed("connection failed"),
            SessionState.Live to SessionEvent.ConnectionLost,
            SessionState.Live to SessionEvent.EndRequested,
            SessionState.Reconnecting to SessionEvent.Reconnected,
            SessionState.Reconnecting to SessionEvent.ReconnectFailed("reconnect failed"),
            SessionState.Reconnecting to SessionEvent.EndRequested,
        )

        states.forEach { initialState ->
            events.forEach { event ->
                if (initialState to event !in legalTransitions) {
                    assertEquals(
                        "$event should be ignored in $initialState",
                        initialState,
                        SessionStateMachine(initialState).transition(event),
                    )
                }
            }
        }
    }
}
