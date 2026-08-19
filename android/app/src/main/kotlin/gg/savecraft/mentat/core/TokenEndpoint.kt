package gg.savecraft.mentat.core

import java.net.HttpURLConnection
import java.net.URI
import java.net.URL
import java.time.Instant
import org.json.JSONObject

data class TokenGrant(
    val token: String,
    val room: String,
    val url: String,
    val expiresAt: String,
)

interface TokenEndpoint {
    fun fetch(): TokenGrant
}

class TokenFetchException(
    message: String,
    cause: Throwable? = null,
) : Exception(message, cause)

class HttpTokenEndpoint(baseUrl: String) : TokenEndpoint {
    private val endpointUrl = "${baseUrl.trimEnd('/')}/v1/voice/token"

    override fun fetch(): TokenGrant {
        var connection: HttpURLConnection? = null
        try {
            connection = URL(endpointUrl).openConnection() as HttpURLConnection
            connection.requestMethod = "POST"
            connection.doOutput = true
            connection.setFixedLengthStreamingMode(0)
            connection.outputStream.use { }

            val status = connection.responseCode
            if (status != HttpURLConnection.HTTP_OK) {
                connection.errorStream?.close()
                throw TokenFetchException("Token endpoint returned HTTP $status")
            }

            val body = connection.inputStream.bufferedReader().use { it.readText() }
            return parseGrant(body)
        } catch (exception: TokenFetchException) {
            throw exception
        } catch (exception: Exception) {
            throw TokenFetchException("Token request failed", exception)
        } finally {
            connection?.disconnect()
        }
    }

    private fun parseGrant(body: String): TokenGrant {
        try {
            val json = JSONObject(body)
            val url = json.getString("url")
            val expiresAt = json.getString("expires_at")
            val scheme = URI.create(url).scheme
            if (scheme != "ws" && scheme != "wss") {
                throw IllegalArgumentException("Token URL must use ws or wss")
            }
            Instant.parse(expiresAt)

            return TokenGrant(
                token = json.getString("token"),
                room = json.getString("room"),
                url = url,
                expiresAt = expiresAt,
            )
        } catch (exception: Exception) {
            throw TokenFetchException("Invalid token response", exception)
        }
    }
}
