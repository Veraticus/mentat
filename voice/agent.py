"""LiveKit voice agent: a room's microphone in, mentat's answer spoken out.

The thinking parts of this surface live next door and are unit-tested offline:
voice/stream.py turns mentat's NDJSON into speakable chunks (acknowledging
once when a turn works before it speaks, never mid-sentence), and
voice/request.py builds the turn request and reads the user's last utterance
out of a chat context. This file is the part that cannot be tested without a
livekit runtime — wiring the STT/TTS pipeline and driving one HTTP turn per
utterance — so it is deliberately thin, and it is verified against a real room
rather than in CI.

Run as `python agent.py start`; livekit-agents ships no console script. The
SDK reads LIVEKIT_URL and the LIVEKIT_API_*/LIVEKIT_INFERENCE_API_* credentials
straight from the environment.
"""

from __future__ import annotations

import logging
import os
from collections.abc import AsyncGenerator

import aiohttp
from livekit import agents
from livekit.agents import (
    Agent,
    AgentSession,
    JobContext,
    ModelSettings,
    WorkerOptions,
    inference,
    llm,
)
from livekit.plugins import silero

from request import last_user_text, turn_request
from stream import TurnError, TurnStream

logger = logging.getLogger("mentat.voice")

DEFAULT_MENTAT_URL = "http://127.0.0.1:8484"

# A turn legitimately runs for minutes while the daemon uses tools; only the
# connect and the gap between chunks are bounded.
TIMEOUT = aiohttp.ClientTimeout(total=None, connect=10, sock_read=600)

# Said when the daemon cannot be reached or fails the turn. Fixed and short:
# whatever went wrong, the caller is waiting in silence for an answer.
FAILURE_REPLY = "Sorry — I can't reach Mentat right now."

# Never sent anywhere: mentatd owns the system prompt, and llm_node below
# never consults the agent's instructions. Agent requires the field.
INSTRUCTIONS = "Voice access to mentat. The daemon holds the real prompt."


class _RoutedToMentat(llm.LLM):
    """Stands in for a provider LLM so the pipeline generates replies at all.

    An AgentSession with no LLM silently skips the response (agent_activity.py:
    `elif self.llm is None: return  # skip response if no llm is set`), so the
    session needs one even though MentatAgent.llm_node answers every turn
    itself and never calls this.
    """

    def chat(self, **kwargs: object) -> llm.LLMStream:
        raise RuntimeError("llm_node override must be used")


class MentatAgent(Agent):
    """Answers each user turn by streaming one mentat turn back as speech."""

    def __init__(self, *, room_name: str, mentat_url: str) -> None:
        super().__init__(instructions=INSTRUCTIONS)
        self._room_name = room_name
        self._mentat_url = mentat_url

    async def llm_node(
        self,
        chat_ctx: llm.ChatContext,
        tools: list[llm.Tool],
        model_settings: ModelSettings,
    ) -> AsyncGenerator[str, None]:
        """Stream one daemon turn as the reply to the latest utterance."""
        text = last_user_text(chat_ctx.items)
        if text is None:
            # No utterance reached us, so there is nothing to ask and nothing
            # to say. Not a failure — the apology below would be a lie.
            return

        stream = TurnStream()
        try:
            async with aiohttp.ClientSession(timeout=TIMEOUT) as http:
                async with http.post(
                    f"{self._mentat_url}/v1/conversation",
                    json=turn_request(self._room_name, text),
                ) as response:
                    if response.status != 200:
                        raise TurnError(f"daemon answered HTTP {response.status}")
                    async for chunk in response.content.iter_any():
                        for spoken in stream.feed(chunk):
                            yield spoken
                    if not stream.done:
                        # The body ended without a done event — mentatd
                        # restarted mid-turn, so the socket closed cleanly and
                        # aiohttp raises nothing. Without this the reply just
                        # stops, possibly mid-sentence, and the caller is left
                        # guessing whether that was the whole answer.
                        raise TurnError("stream ended without done")
        except (TurnError, aiohttp.ClientError, TimeoutError) as err:
            # Barge-in is deliberately not caught: cancellation arrives as
            # CancelledError or GeneratorExit, both BaseException, so it
            # unwinds past here — closing the request, which is what aborts
            # the daemon's turn — and an interrupted turn stays silent
            # instead of apologizing for a failure that never happened.
            logger.warning("mentat turn failed: %s", err)
            yield FAILURE_REPLY


def prewarm(proc: agents.JobProcess) -> None:
    """Load the VAD once per worker process, before a job is ever assigned.

    Loading it inside the entrypoint would put model load on the critical path
    of the first utterance in every room, after the caller is already
    listening; the worker warms up idle instead.
    """
    proc.userdata["vad"] = silero.VAD.load()


async def entrypoint(ctx: JobContext) -> None:
    """Serve one room for as long as it lives."""
    await ctx.connect()

    session = AgentSession(
        vad=ctx.proc.userdata["vad"],
        # The docs' two-argument form is the one agents 1.6.10 takes:
        # "deepgram/flux-general" is a model literal it knows and `language`
        # is a separate keyword, not a ":en" suffix on the model string.
        stt=inference.STT("deepgram/flux-general", language="en"),
        tts=inference.TTS("cartesia/sonic-3"),
        llm=_RoutedToMentat(),
        # Flux emits end-of-turn itself, so waiting out a silence window after
        # it would only add latency to every reply. (Spelled as turn_handling
        # rather than the turn_detection/min_endpointing_delay arguments: those
        # are deprecated in agents 1.6.10 and migrate to exactly this.)
        turn_handling={
            "turn_detection": "stt",
            "endpointing": {"min_delay": 0.0},
        },
    )

    await session.start(
        agent=MentatAgent(
            room_name=ctx.room.name,
            mentat_url=os.environ.get("MENTAT_URL", DEFAULT_MENTAT_URL),
        ),
        room=ctx.room,
    )


if __name__ == "__main__":
    agents.cli.run_app(
        WorkerOptions(entrypoint_fnc=entrypoint, prewarm_fnc=prewarm)
    )
