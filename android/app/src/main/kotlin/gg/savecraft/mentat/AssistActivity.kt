package gg.savecraft.mentat

import android.app.Activity
import android.os.Bundle
import android.util.Log

class AssistActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        logAssistIntent()
        finish()
    }

    override fun onNewIntent(intent: android.content.Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        logAssistIntent()
        finish()
    }

    private fun logAssistIntent() {
        Log.i("MentatAssist", "MENTAT_ASSIST_RECEIVED action=" + intent.action)
    }
}
