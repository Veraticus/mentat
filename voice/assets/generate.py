"""Generate the voice surface's two sound assets.

The .wav files next to this script are checked in, but they are authored here
rather than in an editor: a reviewer cannot diff a binary, and a sound asset
that arrives as opaque bytes can never be audited. Generation is therefore
deterministic — pure arithmetic, no randomness, no timestamps — so re-running
this script reproduces the committed bytes exactly, and the code below is the
real source of truth for what the assistant sounds like.

Both clips feed LiveKit's BackgroundAudioPlayer:

  earcon.wav   the "I heard you" blip, fired the moment the turn enters its
               thinking state after the user stops talking — roughly a second
               ahead of first speech, so the silence never reads as a failure.
  waiting.wav  a soft bed looped under 10-60s deep-model consults, playing
               beneath any speech rather than instead of it.

Usage: generate.py [output-dir]   (defaults to this script's own directory)
"""

from __future__ import annotations

import math
import sys
import wave
from array import array
from pathlib import Path

SAMPLE_RATE = 48000
CHANNELS = 1
SAMPLE_WIDTH = 2  # 16-bit PCM
FULL_SCALE = 32767

# --- earcon ----------------------------------------------------------------
# A rising two-tone chime (A5 then E6, a perfect fifth up). Rising and brief
# is the acknowledgment register — Siri's and Alexa's blips both rise; a
# falling or sustained tone reads as an error or an alarm.
EARCON_TONES = ((880.0, 0.0), (1320.0, 0.10))  # (frequency hz, onset seconds)
EARCON_TONE_S = 0.15  # each tone's length; total clip = 0.10 + 0.15 = 0.25s
EARCON_ATTACK_S = 0.006  # long enough that the onset is a swell, not a click
EARCON_DECAY_S = 0.045  # exponential decay constant, giving a struck-bell tail
# Clearly audible over room noise while staying well under speech, which the
# background player mixes at full level.
EARCON_PEAK_DBFS = -12.0

# --- waiting ---------------------------------------------------------------
# Everything here is counted in whole cycles *per loop* rather than in hertz,
# because that is exactly the property that makes the loop seamless: with an
# integer cycle count, the sample one past the end lands back on the sample at
# the start, matching in both value and slope, so splicing the clip to itself
# produces no tick. Frequency falls out as cycles / WAITING_S.
WAITING_S = 3.0
WAITING_PARTIAL_CYCLES = ((660, 1.0), (990, 0.5))  # 220Hz + 330Hz, a fifth
WAITING_MOD_CYCLES = 1  # one slow breath across the 3s loop = 0.333Hz
WAITING_MOD_DEPTH = 0.25  # light: the pad dips to 75% and back, never pulses
# It plays *under* speech for up to a minute, so it sits far down — loud
# enough to say "still working", quiet enough never to compete with the answer.
WAITING_PEAK_DBFS = -24.0

EARCON_NAME = "earcon.wav"
WAITING_NAME = "waiting.wav"


def _amplitude(dbfs: float) -> float:
    """Linear amplitude (0..1) for a level in dBFS."""
    return 10.0 ** (dbfs / 20.0)


def _normalize(signal: list[float], peak_dbfs: float) -> list[float]:
    """Scale a signal so its loudest sample sits exactly at peak_dbfs."""
    peak = max(abs(value) for value in signal)
    scale = _amplitude(peak_dbfs) / peak
    return [value * scale for value in signal]


def _chime_envelope(index: int, length: int) -> float:
    """Soft attack into exponential decay, exactly 0 at both ends.

    Both ends matter: a hard onset or a truncated tail is a step against the
    silence around it, which is the click this envelope exists to prevent. The
    raised-cosine attack removes the first, and subtracting the decay curve's
    value at the final sample pins the tail to true zero rather than to the
    small-but-nonzero value an exponential would otherwise still hold.
    """
    tau = EARCON_DECAY_S * SAMPLE_RATE
    floor = math.exp(-(length - 1) / tau)
    value = (math.exp(-index / tau) - floor) / (1.0 - floor)
    attack = round(EARCON_ATTACK_S * SAMPLE_RATE)
    if index < attack:
        value *= 0.5 - 0.5 * math.cos(math.pi * index / attack)
    return value


def earcon_signal() -> list[float]:
    """The earcon as floats, peak-normalized to EARCON_PEAK_DBFS."""
    tone_len = round(EARCON_TONE_S * SAMPLE_RATE)
    total = max(round(onset * SAMPLE_RATE) for _, onset in EARCON_TONES) + tone_len
    signal = [0.0] * total
    for frequency, onset in EARCON_TONES:
        start = round(onset * SAMPLE_RATE)
        step = math.tau * frequency / SAMPLE_RATE
        for i in range(tone_len):
            signal[start + i] += math.sin(step * i) * _chime_envelope(i, tone_len)
    return _normalize(signal, EARCON_PEAK_DBFS)


def waiting_signal() -> list[float]:
    """The wait loop as floats, peak-normalized to WAITING_PEAK_DBFS."""
    total = round(WAITING_S * SAMPLE_RATE)
    signal = []
    for i in range(total):
        # Phase as a fraction of the whole loop, so an integer cycle count is
        # an exact integer number of turns by the final sample.
        turn = i / total
        pad = sum(
            weight * math.sin(math.tau * cycles * turn)
            for cycles, weight in WAITING_PARTIAL_CYCLES
        )
        # Cosine modulation sits at its maximum at both ends of the loop, so
        # the breath crosses the splice as smoothly as the carrier does.
        depth = 1.0 - WAITING_MOD_DEPTH * (
            1.0 - math.cos(math.tau * WAITING_MOD_CYCLES * turn)
        ) / 2.0
        signal.append(pad * depth)
    return _normalize(signal, WAITING_PEAK_DBFS)


def to_pcm16(signal: list[float]) -> array[int]:
    """Quantize floats in -1..1 to signed 16-bit samples."""
    return array("h", [round(value * FULL_SCALE) for value in signal])


def write_wav(path: Path, samples: array[int]) -> None:
    """Write mono 16-bit PCM at SAMPLE_RATE.

    array("h") is the right pairing for wave: both speak native byte order and
    the module handles the little-endian file format itself, so the bytes match
    on any host.
    """
    with wave.open(str(path), "wb") as out:
        out.setnchannels(CHANNELS)
        out.setsampwidth(SAMPLE_WIDTH)
        out.setframerate(SAMPLE_RATE)
        out.writeframes(samples.tobytes())


def generate(out_dir: Path) -> None:
    """Write both assets into out_dir."""
    write_wav(out_dir / EARCON_NAME, to_pcm16(earcon_signal()))
    write_wav(out_dir / WAITING_NAME, to_pcm16(waiting_signal()))


def main(argv: list[str]) -> None:
    out_dir = Path(argv[1]) if len(argv) > 1 else Path(__file__).resolve().parent
    generate(out_dir)


if __name__ == "__main__":
    main(sys.argv)
