# 🚀 Quick Start Guide: Elite Dangerous Cockpit Assistant (COVAS)

Get up and running in **under 2 minutes** using the pre-compiled binary. No Go installation, compiling, or development tools are needed!

---

## 1. Download the Binary

Head over to the [GitHub Releases](https://github.com/gerifield/ed-assist/releases) page and download the executable for your platform:

- **Windows**: `ed-assist-web.exe`
- **Linux**: `ed-assist-web-linux-amd64`

Place the downloaded binary in any folder of your choice (e.g., `C:\Games\ed-assist\` or `~/ed-assist/`).

---

## 2. Setup `config.ini`

1. Download the template [`config.ini.example`](../config.ini.example).
2. Place it in the **same folder** as your executable.
3. Rename the file to **`config.ini`**.

---

## 3. Configure Your Settings

Open `config.ini` in any text editor (Notepad, VS Code, Nano, etc.) and update the following settings:

### A. Set Your AI Model & API Key
- **Google Gemini (Default)**:
  ```ini
  [ai]
  ai_provider = gemini
  gemini_api_key = AIzaSy...your_gemini_api_key_here
  gemini_model = gemini-flash-lite-latest
  ```
- **Or OpenAI-Compatible (Groq / DeepSeek / Ollama / LM Studio)**:
  ```ini
  [ai]
  ai_provider = openai
  openai_api_key = gsk_...your_api_key_here
  openai_model = llama-3.3-70b-versatile
  openai_base_url = https://api.groq.com/openai/v1
  ```

### B. Voice Input / Speech-to-Text (STT) *(Optional)*
Depending on your AI provider, choose the simplest option:

- **Easiest: Google Gemini Native Audio (No extra STT key needed)**  
  Gemini (`gemini-flash-lite-latest`) natively handles raw microphone audio directly:
  ```ini
  [general]
  audio_input_mode = native
  ```

- **Groq Whisper Pre-Transcription (For DeepSeek, Ollama, OpenAI, or live HUD bubble text)**  
  If your AI provider does not support native audio input, or if you prefer having speech transcribed to text before sending:
  ```ini
  [general]
  audio_input_mode = transcribe

  [stt]
  groq_api_key = gsk_...your_groq_api_key_here
  language = en
  ```

### C. Enable Cockpit Controls *(Optional)*
If you want your AI assistant to physically execute ship commands (deploy landing gear, open cargo scoop, toggle lights, night vision, boost, power distribution pips):
```ini
[general]
game_control = true
; (or enable_control = true)
```
> [!NOTE]
> `ed-assist` automatically detects and maps your active in-game keybindings from Elite's `.binds` file.

---

## 4. Start the Assistant

- **Windows**: Double-click `ed-assist-web.exe` (or run it in PowerShell / Command Prompt).
- **Linux**: Run `./ed-assist-web-linux-amd64`.

The console will show that telemetry monitoring and the web server have started.

---

## 5. Open in Your Browser

Open your browser and navigate to:

👉 **[http://127.0.0.1:3000](http://127.0.0.1:3000)**

That's it! Your COVAS Cockpit Assistant is ready. You can:
- **Speak** via Automatic Noise Gate (`VOX`) or Push-to-Record (`REC`).
- **Type** questions or flight commands directly in the cockpit HUD.
- Ask for telemetry, ship status, fuel, visited systems, navigation, or give ship commands.
