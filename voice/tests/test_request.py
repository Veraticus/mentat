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

from request import (
    CONSULT_TURN_CHARS,
    TURN_EFFORT,
    TURN_META,
    TURN_MODEL,
    consult_envelope,
    conversation_advanced,
    count_user_messages,
    last_user_text,
    turn_request,
)


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
                "model": "sonnet",
            },
        )

    def test_session_id_namespaces_the_room(self):
        # A daemon shared with other surfaces must never collide with a room.
        self.assertEqual(turn_request("office", "hi")["session_id"], "voice-office")

    def test_voice_turns_are_low_effort_and_identified(self):
        # Authority is per-turn in mentat policy: the surface and user ride
        # along with every request. The model rides along too: the daemon's
        # default is the deepest model, whose latency and usage limits don't
        # suit a caller waiting in silence (first live test hit both).
        self.assertEqual(TURN_EFFORT, "low")
        self.assertEqual(TURN_META, {"surface": "voice", "user": "josh"})
        self.assertEqual(TURN_MODEL, "sonnet")

    def test_effort_and_model_are_passed_through(self):
        # A consult escalates both knobs; the tool call carries them, so the
        # request builder takes them as arguments rather than reading globals.
        body = turn_request("kitchen", "why?", effort="high", model="fable")
        self.assertEqual(body["effort"], "high")
        self.assertEqual(body["model"], "fable")

    def test_unknown_effort_and_model_are_not_validated_here(self):
        # This layer stays dumb: the daemon owns the vocabulary, and rejecting
        # a value here would only turn a clear daemon error into a local one.
        body = turn_request("kitchen", "why?", effort="medium", model="opus")
        self.assertEqual(body["effort"], "medium")
        self.assertEqual(body["model"], "opus")


class ConsultEnvelopeTest(unittest.TestCase):
    PERSONA = "You are Luna, dry and quick."
    QUESTION = "What did the doctor say about the results?"

    def test_framing_sentence_comes_first_verbatim(self):
        # The reply is spoken as-is in the front's voice, so the envelope's
        # first instruction is the one that governs style.
        envelope = consult_envelope(self.PERSONA, "", [], self.QUESTION)
        self.assertTrue(
            envelope.startswith(
                "Your reply will be read aloud verbatim to the user as a "
                "continuation of this conversation — match this voice and style."
            ),
            envelope,
        )

    def test_sections_appear_in_order(self):
        envelope = consult_envelope(
            self.PERSONA,
            "Josh is planning a trip.",
            [("user", "where should I go?"), ("assistant", "somewhere warm")],
            self.QUESTION,
        )
        positions = [
            envelope.index("read aloud verbatim"),
            envelope.index(self.PERSONA),
            envelope.index("Josh is planning a trip."),
            envelope.index("user: where should I go?"),
            envelope.index(self.QUESTION),
        ]
        self.assertEqual(positions, sorted(positions), envelope)

    def test_turns_render_one_per_line_as_role_text(self):
        envelope = consult_envelope(
            self.PERSONA,
            "",
            [("user", "first"), ("assistant", "second"), ("user", "third")],
            self.QUESTION,
        )
        lines = envelope.splitlines()
        self.assertIn("user: first", lines)
        self.assertIn("assistant: second", lines)
        self.assertIn("user: third", lines)

    def test_summary_section_is_omitted_when_empty(self):
        with_summary = consult_envelope(
            self.PERSONA, "Josh is planning a trip.", [], self.QUESTION
        )
        without = consult_envelope(self.PERSONA, "", [], self.QUESTION)
        blank = consult_envelope(self.PERSONA, "   \n ", [], self.QUESTION)
        self.assertIn("Josh is planning a trip.", with_summary)
        # The label goes with the section: an empty heading is noise the
        # consulted model would have to interpret.
        self.assertIn("Conversation so far:", with_summary)
        self.assertNotIn("Conversation so far:", without)
        self.assertEqual(without, blank)

    def test_empty_last_turns_leaves_no_blank_hole(self):
        envelope = consult_envelope(self.PERSONA, "A summary.", [], self.QUESTION)
        self.assertIn(self.QUESTION, envelope)
        self.assertNotIn("\n\n\n", envelope)

    def test_long_turns_are_truncated_at_the_cap(self):
        envelope = consult_envelope(
            self.PERSONA, "", [("user", "x" * (CONSULT_TURN_CHARS + 50))], "q?"
        )
        self.assertIn("user: " + "x" * CONSULT_TURN_CHARS + "…", envelope)
        self.assertNotIn("x" * (CONSULT_TURN_CHARS + 1), envelope)

    def test_turns_at_the_cap_are_left_alone(self):
        envelope = consult_envelope(
            self.PERSONA, "", [("user", "x" * CONSULT_TURN_CHARS)], "q?"
        )
        self.assertIn("user: " + "x" * CONSULT_TURN_CHARS + "\n", envelope + "\n")
        self.assertNotIn("…", envelope)

    def test_question_carries_a_label(self):
        envelope = consult_envelope(self.PERSONA, "", [], self.QUESTION)
        self.assertIn("Question:\n" + self.QUESTION, envelope)

    def test_the_cap_is_pinned(self):
        # Bounded on purpose: mentatd's session already remembers the prior
        # consults, so a growing window would re-feed it its own history.
        self.assertEqual(CONSULT_TURN_CHARS, 500)


class ReorientationTest(unittest.TestCase):
    def test_counts_only_user_messages(self):
        self.assertEqual(
            count_user_messages(
                [
                    Message("user", "one"),
                    Message("assistant", "reply"),
                    FunctionCall(),
                    Message("user", "two"),
                ]
            ),
            2,
        )

    def test_counts_nothing_in_an_empty_context(self):
        self.assertEqual(count_user_messages([]), 0)

    def test_tool_and_assistant_items_do_not_advance_the_conversation(self):
        # The framework appends the function call and the front's own spoken
        # front-matter during the consult, so raw length would always grow.
        baseline_items = [Message("user", "what did the doctor say?")]
        at_dispatch = count_user_messages(baseline_items)
        during = [
            *baseline_items,
            FunctionCall(),
            Message("assistant", "let me check on that"),
        ]
        self.assertFalse(conversation_advanced(during, at_dispatch))

    def test_a_new_user_message_advances_the_conversation(self):
        baseline_items = [Message("user", "what did the doctor say?")]
        at_dispatch = count_user_messages(baseline_items)
        during = [
            *baseline_items,
            FunctionCall(),
            Message("assistant", "let me check on that"),
            Message("user", "actually, what time is it?"),
        ]
        self.assertTrue(conversation_advanced(during, at_dispatch))

    def test_an_unchanged_context_has_not_advanced(self):
        items = [Message("user", "asked"), Message("assistant", "answered")]
        self.assertFalse(conversation_advanced(items, count_user_messages(items)))


if __name__ == "__main__":
    unittest.main()
