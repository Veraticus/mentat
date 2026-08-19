"""Pure construction of what a voice room sends mentat: turns and consults.

A plain turn is the room's own utterance. A consult is the front voice asking
the deep brain a question mid-conversation and speaking the answer verbatim,
which needs an envelope around it and a check on whether the caller kept
talking during the wait.

No livekit import: the chat-context readers are structural — they read .type,
.role and .text_content off whatever items they are handed — so a real livekit
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

#: Governs the consulted reply's form: mentatd answers in its own register by
#: default, but a consult's answer is spoken by the front voice, unedited.
CONSULT_FRAMING = (
    "Your reply will be read aloud verbatim to the user as a continuation of "
    "this conversation — match this voice and style."
)

#: Per-turn character cap for the conversation window in a consult envelope.
#: The envelope must stay bounded: mentatd keeps one persistent session per
#: room and already remembers the earlier consults, so an unbounded window
#: re-feeds it its own history at a token cost that grows with the call.
CONSULT_TURN_CHARS = 500


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


def count_user_messages(items: Iterable[Any]) -> int:
    """How many user messages the chat context holds right now."""
    return sum(
        1
        for item in items
        if getattr(item, "type", None) == "message" and item.role == "user"
    )


def conversation_advanced(items: Iterable[Any], users_at_dispatch: int) -> bool:
    """Did the user say something new since the consult was dispatched?

    Only user messages count. Comparing raw context length would fire on every
    consult: the framework appends the function-call and function-output items
    itself, and the front's spoken front-matter lands as an assistant message
    while the consult is still in flight. A caller who kept talking through the
    wait, though, has moved the conversation on — and the answer, arriving
    afterwards, has to be reoriented before it is spoken.
    """
    return count_user_messages(items) > users_at_dispatch


def turn_request(
    room_name: str,
    text: str,
    effort: str = TURN_EFFORT,
    model: str = TURN_MODEL,
) -> dict[str, Any]:
    """The POST body for one /v1/conversation turn from this room.

    effort and model default to the fast pairing every plain voice turn wants;
    a consult escalates them per call. Values are passed through unvalidated —
    the daemon owns that vocabulary, and duplicating it here would only turn a
    clear daemon rejection into a local one that drifts out of date.
    """
    return {
        "session_id": SESSION_PREFIX + room_name,
        "text": text,
        "meta": TURN_META,
        "effort": effort,
        "model": model,
    }


def consult_envelope(
    persona_card: str,
    summary: str,
    last_turns: Iterable[tuple[str, str]],
    question: str,
) -> str:
    """The consult text: what the front voice sends the deep brain.

    mentatd's reply is spoken verbatim, so the envelope leads with that
    constraint and the persona it must sound like, then gives just enough
    conversation for the question to make sense.
    """
    sections = [CONSULT_FRAMING, persona_card]
    # An empty heading is noise the consulted model would have to interpret,
    # so the label leaves with its section.
    if summary.strip():
        sections.append("Conversation so far:\n" + summary)
    window = "\n".join(f"{role}: {_capped(text)}" for role, text in last_turns)
    if window:
        sections.append(window)
    sections.append("Question:\n" + question)
    return "\n\n".join(sections)


def _capped(text: str) -> str:
    """One turn's text, cut at the cap with a visible truncation marker."""
    if len(text) <= CONSULT_TURN_CHARS:
        return text
    return text[:CONSULT_TURN_CHARS] + "…"
