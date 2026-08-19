package gg.savecraft.mentat.core

import com.sun.net.httpserver.HttpServer
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class TokenEndpointTest {
    @Test
    fun fetchPostsEmptyBodyAndParsesGrant() {
        val method = AtomicReference<String>()
        val bodySize = AtomicInteger(-1)
        val response = """
            {
              "token": "jwt-token",
              "room": "android-room",
              "url": "wss://voice.example.com",
              "expires_at": "2026-08-19T16:00:00.000Z"
            }
        """.trimIndent()

        withServer(
            response = response,
            onRequest = { requestMethod, requestBody ->
                method.set(requestMethod)
                bodySize.set(requestBody.size)
            },
        ) { baseUrl ->
            assertEquals(
                TokenGrant(
                    token = "jwt-token",
                    room = "android-room",
                    url = "wss://voice.example.com",
                    expiresAt = "2026-08-19T16:00:00.000Z",
                ),
                HttpTokenEndpoint(baseUrl).fetch(),
            )
        }

        assertEquals("POST", method.get())
        assertEquals(0, bodySize.get())
    }

    @Test
    fun nonSuccessResponseThrowsTokenFetchException() {
        withServer(status = 404, response = "not found") { baseUrl ->
            assertThrows(TokenFetchException::class.java) {
                HttpTokenEndpoint(baseUrl).fetch()
            }
        }
    }

    @Test
    fun malformedJsonThrowsTokenFetchException() {
        withServer(response = "{not json") { baseUrl ->
            assertThrows(TokenFetchException::class.java) {
                HttpTokenEndpoint(baseUrl).fetch()
            }
        }
    }

    @Test
    fun nonWebSocketUrlThrowsTokenFetchException() {
        val response = """
            {
              "token": "jwt-token",
              "room": "android-room",
              "url": "https://voice.example.com",
              "expires_at": "2026-08-19T16:00:00.000Z"
            }
        """.trimIndent()

        withServer(response = response) { baseUrl ->
            assertThrows(TokenFetchException::class.java) {
                HttpTokenEndpoint(baseUrl).fetch()
            }
        }
    }

    @Test
    fun nonIsoExpiryThrowsTokenFetchException() {
        val response = """
            {
              "token": "jwt-token",
              "room": "android-room",
              "url": "wss://voice.example.com",
              "expires_at": "tomorrow"
            }
        """.trimIndent()

        withServer(response = response) { baseUrl ->
            assertThrows(TokenFetchException::class.java) {
                HttpTokenEndpoint(baseUrl).fetch()
            }
        }
    }

    @Test
    fun refusedConnectionThrowsTokenFetchException() {
        val port = ServerSocket(0, 1, InetAddress.getLoopbackAddress()).use { it.localPort }

        assertThrows(TokenFetchException::class.java) {
            HttpTokenEndpoint("http://127.0.0.1:$port").fetch()
        }
    }

    private fun withServer(
        status: Int = 200,
        response: String,
        onRequest: (String, ByteArray) -> Unit = { _, _ -> },
        block: (String) -> Unit,
    ) {
        val server = HttpServer.create(InetSocketAddress(InetAddress.getLoopbackAddress(), 0), 0)
        server.createContext("/v1/voice/token") { exchange ->
            val requestBody = exchange.requestBody.use { it.readBytes() }
            onRequest(exchange.requestMethod, requestBody)
            val bytes = response.toByteArray()
            exchange.sendResponseHeaders(status, bytes.size.toLong())
            exchange.responseBody.use { it.write(bytes) }
        }
        server.start()
        try {
            block("http://127.0.0.1:${server.address.port}")
        } finally {
            server.stop(0)
        }
    }
}
