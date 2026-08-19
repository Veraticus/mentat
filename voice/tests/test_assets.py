"""Tests for the generated voice sound assets.

The assets are checked in as .wav files but authored as code, so these tests
guard both halves: the committed files have the shape the LiveKit
BackgroundAudioPlayer needs (48kHz mono 16-bit, right durations, right
loudness), and re-running the generator reproduces them byte for byte. The
second half is what makes a binary asset reviewable — a reviewer who cannot
diff a .wav can still read generate.py and confirm the bytes follow from it.
"""

import math
import subprocess
import sys
import tempfile
import unittest
import wave
from array import array
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "assets"))

import generate

ASSETS = Path(__file__).resolve().parent.parent / "assets"
EARCON = ASSETS / "earcon.wav"
WAITING = ASSETS / "waiting.wav"

FULL_SCALE = 32767


def read_wav(path):
    """(samples, framerate, channels, sampwidth) for a 16-bit PCM file.

    array("h") is the right pairing for wave: both sides speak native byte
    order and the module handles the little-endian file format itself.
    """
    with wave.open(str(path), "rb") as w:
        params = w.getparams()
        samples = array("h", w.readframes(params.nframes))
    return samples, params.framerate, params.nchannels, params.sampwidth


def peak_dbfs(samples):
    return 20.0 * math.log10(max(abs(s) for s in samples) / FULL_SCALE)


class FormatTest(unittest.TestCase):
    """Both clips must be what the playback path expects: 48kHz mono 16-bit."""

    def test_earcon_exists(self):
        self.assertTrue(EARCON.is_file(), f"{EARCON} not generated")

    def test_waiting_exists(self):
        self.assertTrue(WAITING.is_file(), f"{WAITING} not generated")

    def test_earcon_format(self):
        _, framerate, channels, width = read_wav(EARCON)
        self.assertEqual((framerate, channels, width), (48000, 1, 2))

    def test_waiting_format(self):
        _, framerate, channels, width = read_wav(WAITING)
        self.assertEqual((framerate, channels, width), (48000, 1, 2))


class EarconTest(unittest.TestCase):
    """The acknowledgment blip: short, polite, click-free."""

    def test_duration_is_a_blip(self):
        # It lands the instant the user stops talking, roughly a second ahead
        # of first speech; long enough to register, short enough not to delay.
        samples, framerate, _, _ = read_wav(EARCON)
        self.assertGreaterEqual(len(samples) / framerate, 0.1)
        self.assertLessEqual(len(samples) / framerate, 0.5)

    def test_peak_is_minus_12_dbfs(self):
        # Audible over room noise but well under speech, which the player
        # mixes at full level.
        samples, _, _, _ = read_wav(EARCON)
        self.assertAlmostEqual(peak_dbfs(samples), -12.0, delta=0.1)

    def test_starts_and_ends_in_silence(self):
        # A non-zero first or last sample is a step discontinuity against the
        # silence around it — the click the envelope exists to prevent.
        samples, _, _, _ = read_wav(EARCON)
        self.assertEqual(samples[0], 0)
        self.assertEqual(samples[-1], 0)

    def test_rises_in_pitch(self):
        # The two-tone rise is what makes it read as "listening" rather than
        # as an alarm: the second tone starts above the first.
        self.assertLess(generate.EARCON_TONES[0][0], generate.EARCON_TONES[1][0])
        self.assertLess(generate.EARCON_TONES[0][1], generate.EARCON_TONES[1][1])


class WaitingTest(unittest.TestCase):
    """The loop under a long consult: long, quiet, and seamless."""

    def test_duration_supports_a_slow_loop(self):
        samples, framerate, _, _ = read_wav(WAITING)
        self.assertGreaterEqual(len(samples) / framerate, 1.5)
        self.assertLessEqual(len(samples) / framerate, 5.0)

    def test_peak_is_minus_24_dbfs(self):
        # It plays *under* speech for up to a minute; anything louder competes
        # with the answer it is covering for.
        samples, _, _, _ = read_wav(WAITING)
        self.assertAlmostEqual(peak_dbfs(samples), -24.0, delta=0.1)

    def test_loop_seam_samples_are_near_zero(self):
        # Whole-cycle durations put both ends at the carrier's zero crossing,
        # so the level at the splice is a rounding artifact, not a step.
        samples, _, _, _ = read_wav(WAITING)
        threshold = FULL_SCALE // 100  # 1% of full scale
        self.assertLess(abs(samples[0]), threshold)
        self.assertLess(abs(samples[-1]), threshold)

    def test_loop_seam_matches_the_slope_it_wraps_into(self):
        # The real seamlessness property, and stronger than near-zero ends:
        # wrapping from the last sample to the first must be an ordinary step
        # for that phase — same size AND same direction as the step that
        # follows it. Endpoints that are both near zero but on opposite slopes
        # (a duration off by half a cycle) leave a corner that still ticks;
        # only the signed comparison catches that.
        samples, _, _, _ = read_wav(WAITING)
        seam_step = samples[0] - samples[-1]
        next_step = samples[1] - samples[0]
        self.assertAlmostEqual(seam_step, next_step, delta=2)  # int16 rounding


class DeterminismTest(unittest.TestCase):
    """Regeneration must be byte-identical or the assets stop being auditable."""

    def test_regeneration_reproduces_the_committed_bytes(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = subprocess.run(
                [sys.executable, str(ASSETS / "generate.py"), tmp],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            for committed in (EARCON, WAITING):
                fresh = Path(tmp) / committed.name
                self.assertEqual(
                    fresh.read_bytes(),
                    committed.read_bytes(),
                    f"{committed.name} does not match generate.py output",
                )


if __name__ == "__main__":
    unittest.main()
