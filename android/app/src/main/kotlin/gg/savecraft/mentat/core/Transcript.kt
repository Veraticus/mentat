package gg.savecraft.mentat.core

data class TranscriptSegment(
    val id: String,
    val participantIdentity: String,
    val text: String,
    val final: Boolean,
)

class Transcript {
    private val segmentsById = linkedMapOf<String, TranscriptSegment>()

    val segments: List<TranscriptSegment>
        get() = segmentsById.values.toList()

    fun update(segment: TranscriptSegment) {
        if (segmentsById[segment.id]?.final == true) {
            return
        }
        segmentsById[segment.id] = segment
    }
}
