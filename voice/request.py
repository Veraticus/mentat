"""Pure construction of one mentat turn request for a voice room.

No livekit import: the chat-context reader is structural — it reads .type,
.role and .text_content off whatever items it is handed — so a real livekit
ChatMessage and a plain test double both work, and voice/tests runs offline
next to voice/stream.py. Everything agent.py does beyond this is glue.
"""

from __future__ import annotations

from collections.abc import Iterable
from typing import Any

#: mentat session ids are namespaced so a daemon shared with other surfaces
#: can never collide with a LiveKit room name.
SESSION_PREFIX = "voice-"

# Voice turns are latency-bound: low effort, identified surface, and a fast
# model — the daemon's default is the deepest one, whose latency (and usage
# limits) don't suit a caller waiting in silence. The user is part of the
# turn's authority context (mentat policy is per-turn).
TURN_META = {"surface": "voice", "user": "josh"}
TURN_EFFORT = "low"
TURN_MODEL = "sonnet"


def last_user_text(items: Iterable[Any]) -> str | None:
    """The final user message's text, or None when the turn carries none.

    The daemon owns conversation memory — one persistent session per room — so
    a turn sends the new utterance alone and never serializes the history the
    pipeline hands over. Strictly the *final* user message: an empty one does
    not fall back to an older utterance, which would re-ask an answered
    question.
    """
    for item in reversed(list(items)):
        if getattr(item, "type", None) == "message" and item.role == "user":
            return item.text_content or None
    return None


def turn_request(room_name: str, text: str) -> dict[str, Any]:
    """The POST body for one /v1/conversation turn from this room."""
    return {
        "session_id": SESSION_PREFIX + room_name,
        "text": text,
        "meta": TURN_META,
        "effort": TURN_EFFORT,
        "model": TURN_MODEL,
    }
