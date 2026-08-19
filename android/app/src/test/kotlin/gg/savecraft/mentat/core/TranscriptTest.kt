package gg.savecraft.mentat.core

import org.junit.Assert.assertEquals
import org.junit.Test

class TranscriptTest {
    @Test
    fun partialUpdatesAreReplacedUntilFinal() {
        val transcript = Transcript()

        transcript.update(segment(id = "one", text = "hel", final = false))
        transcript.update(segment(id = "one", text = "hello", final = false))
        transcript.update(segment(id = "one", text = "hello world", final = true))

        assertEquals(
            listOf(segment(id = "one", text = "hello world", final = true)),
            transcript.segments,
        )
    }

    @Test
    fun interleavedParticipantsStayOrderedByFirstAppearance() {
        val transcript = Transcript()

        transcript.update(segment(id = "alice-1", participant = "alice", text = "Hi", final = false))
        transcript.update(segment(id = "bob-1", participant = "bob", text = "Hello", final = true))
        transcript.update(segment(id = "alice-1", participant = "alice", text = "Hi Bob", final = true))

        assertEquals(
            listOf(
                segment(id = "alice-1", participant = "alice", text = "Hi Bob", final = true),
                segment(id = "bob-1", participant = "bob", text = "Hello", final = true),
            ),
            transcript.segments,
        )
    }

    @Test
    fun reapplyingSameFinalSegmentIsIdempotent() {
        val transcript = Transcript()
        val final = segment(id = "one", text = "done", final = true)

        transcript.update(final)
        transcript.update(final)

        assertEquals(listOf(final), transcript.segments)
    }

    @Test
    fun finalSegmentIgnoresLaterUpdatesWithSameId() {
        val transcript = Transcript()
        val final = segment(id = "one", text = "done", final = true)
        transcript.update(final)

        transcript.update(segment(id = "one", text = "changed", final = false))
        transcript.update(segment(id = "one", text = "changed again", final = true))

        assertEquals(listOf(final), transcript.segments)
    }

    @Test
    fun newSegmentAfterFinalAppends() {
        val transcript = Transcript()
        val first = segment(id = "one", text = "first", final = true)
        val second = segment(id = "two", text = "second", final = false)

        transcript.update(first)
        transcript.update(second)

        assertEquals(listOf(first, second), transcript.segments)
    }

    private fun segment(
        id: String,
        participant: String = "assistant",
        text: String,
        final: Boolean,
    ) = TranscriptSegment(
        id = id,
        participantIdentity = participant,
        text = text,
        final = final,
    )
}
