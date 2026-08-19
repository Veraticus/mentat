"""Tests for the pure mentat turn-request construction.

The chat-context stand-ins here mirror livekit's ChatContext.items: message
items carry type/role/text_content, and function-call items carry neither role
nor text. Reading them structurally is what keeps this testable offline —
livekit is not importable without the flake's voice-env.
"""

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from request import TURN_EFFORT, TURN_META, last_user_text, turn_request


class Message:
    """Stand-in for livekit.agents.llm.ChatMessage."""

    type = "message"

    def __init__(self, role, text):
        self.role = role
        self.text_content = text


class FunctionCall:
    """Stand-in for a non-message chat item: no role, no text."""

    type = "function_call"


class LastUserTextTest(unittest.TestCase):
    def test_returns_the_final_user_message(self):
        self.assertEqual(
            last_user_text(
                [
                    Message("user", "first"),
                    Message("assistant", "answered"),
                    Message("user", "second"),
                ]
            ),
            "second",
        )

    def test_ignores_assistant_messages(self):
        # The daemon owns conversation memory: only the new utterance travels.
        self.assertEqual(
            last_user_text([Message("user", "asked"), Message("assistant", "said")]),
            "asked",
        )

    def test_ignores_non_message_items(self):
        # A function-call item has no .role at all; reading it must not raise.
        self.assertEqual(
            last_user_text([Message("user", "asked"), FunctionCall()]), "asked"
        )

    def test_no_user_message_yields_none(self):
        self.assertIsNone(last_user_text([Message("assistant", "hello")]))

    def test_empty_context_yields_none(self):
        self.assertIsNone(last_user_text([]))

    def test_empty_final_user_message_yields_none(self):
        # Strictly the *final* user message: an empty one never falls back to
        # an older utterance, which would re-ask a question already answered.
        self.assertIsNone(
            last_user_text([Message("user", "earlier"), Message("user", "")])
        )

    def test_final_user_message_without_text_yields_none(self):
        # ChatMessage.text_content is None when the message holds no text part.
        self.assertIsNone(last_user_text([Message("user", None)]))


class TurnRequestTest(unittest.TestCase):
    def test_body_matches_the_conversation_api(self):
        self.assertEqual(
            turn_request("kitchen", "what's on today?"),
            {
                "session_id": "voice-kitchen",
                "text": "what's on today?",
                "meta": {"surface": "voice", "user": "josh"},
                "effort": "low",
            },
        )

    def test_session_id_namespaces_the_room(self):
        # A daemon shared with other surfaces must never collide with a room.
        self.assertEqual(turn_request("office", "hi")["session_id"], "voice-office")

    def test_voice_turns_are_low_effort_and_identified(self):
        # Authority is per-turn in mentat policy: the surface and user ride
        # along with every request.
        self.assertEqual(TURN_EFFORT, "low")
        self.assertEqual(TURN_META, {"surface": "voice", "user": "josh"})


if __name__ == "__main__":
    unittest.main()
