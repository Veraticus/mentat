"""Pure translation of mentat NDJSON wire bytes into speakable chunks.

No livekit or aiohttp imports — this module is the testable core of the voice
agent (voice/tests/test_stream.py runs it offline with stdlib unittest). The
wire contract is pinned by the daemon's golden tests (test/wire.test.ts).

The voice surface differs from the chat surfaces in one way: silence is not
free. A turn that runs tools before saying anything leaves the caller
listening to nothing, so the first tool_start of a turn is turned into a
short spoken acknowledgment — but only while the turn is still silent, since
once speech is streaming an ack would interject mid-sentence.
"""

from __future__ import annotations

import json

#: Spoken once per turn when work starts before any speech has.
ACKNOWLEDGMENT = "One moment."


class TurnError(Exception):
    """The turn failed: a terminal error line or an is_error done."""


class LineSplitter:
    """Splits a chunked byte stream into complete NDJSON lines.

    Buffering happens at the byte level so a UTF-8 sequence split across
    chunks survives; '\\n' is a single byte in UTF-8, so splitting before
    decoding is safe. An incomplete tail (connection cut mid-line) is never
    emitted.
    """

    _buffer: bytes

    def __init__(self) -> None:
        self._buffer = b""

    def feed(self, chunk: bytes) -> list[str]:
        self._buffer += chunk
        *complete, self._buffer = self._buffer.split(b"\n")
        return [decoded for raw in complete if (decoded := raw.decode().strip())]


class TurnStream:
    """Accumulates one turn's wire bytes into the text a voice pipeline speaks.

    State is turn-scoped: construct one per turn, never reuse across turns.
    """

    done: bool
    """True once the turn's done event arrived without an error."""

    _splitter: LineSplitter
    _spoke: bool
    _acked: bool

    def __init__(self) -> None:
        self._splitter = LineSplitter()
        self._spoke = False
        self._acked = False
        self.done = False

    def feed(self, data: bytes) -> list[str]:
        """Returns the chunks to speak, in order, for these bytes.

        Raises TurnError on the terminal error line and on an is_error done;
        anything already collected from this chunk is dropped with it, since
        the turn is over.
        """
        chunks: list[str] = []
        for line in self._splitter.feed(data):
            spoken = self._consume(line)
            if spoken is not None:
                chunks.append(spoken)
        return chunks

    def _consume(self, line: str) -> str | None:
        try:
            event = json.loads(line)
        except ValueError as err:
            raise TurnError(f"malformed wire line: {line[:120]}") from err
        if not isinstance(event, dict):
            raise TurnError(f"malformed wire line: {line[:120]}")

        kind = event.get("kind")
        if kind == "text_delta":
            # omitempty: the daemon drops the text key when the delta is empty,
            # and an empty delta is not speech — the ack is still owed.
            text = event.get("text", "")
            if not text:
                return None
            self._spoke = True
            return str(text)
        if kind == "tool_start":
            if self._spoke or self._acked:
                return None
            self._acked = True
            return ACKNOWLEDGMENT
        if kind == "error":
            raise TurnError(str(event.get("message", "unknown daemon error")))
        if kind == "done":
            done = event.get("done", {})
            if isinstance(done, dict) and done.get("is_error"):
                raise TurnError(str(done.get("text", "turn failed")))
            self.done = True
            return None
        # thinking_delta, thinking, tool_result, and kinds newer than this
        # adapter: nothing to say, and forward compatibility means a newer
        # daemon must not break the turn.
        return None
