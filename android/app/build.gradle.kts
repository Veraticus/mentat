plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "gg.savecraft.mentat"
    compileSdk = 36
    buildToolsVersion = "36.0.0"

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        applicationId = "gg.savecraft.mentat"
        minSdk = 34
        targetSdk = 36
        versionCode = 1
        versionName = "1.0"
    }
}

dependencies {
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}

tasks.withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>().configureEach {
    if (name.contains("UnitTest")) {
        compilerOptions.noJdk.set(false)
        compilerOptions.freeCompilerArgs.add("-Xadd-modules=jdk.httpserver")
    }
}

tasks.withType<org.gradle.api.tasks.testing.Test>().configureEach {
    jvmArgs("--add-modules", "jdk.httpserver")
    reports.html.required.set(false)
}
