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
  Get a free API key at **[Google AI Studio](https://aistudio.google.com/)**:
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
  Get a free key at **[Groq Console](https://console.groq.com/keys)**:
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

1. Navigate to: 👉 **[http://127.0.0.1:3000](http://127.0.0.1:3000)**
2. **Allow Microphone Access**: Click **"Allow"** when your browser prompts for microphone permission.
3. **VOX Visual Feedback**: Switch on **`VOX`** (or click `REC`). The live audio level bar reacts as you speak and glows red whenever you pass the noise gate threshold.
4. **Try your first commands**:
   - 🎙️ *"Status report: check our hull, shields, and fuel."*
   - 🎙️ *"Deploy landing gear and turn on lights."*
   - 🎙️ *"Where are we, and tell me about our destination target system?"*
   - 🎙️ *"Put four pips into engines and two to shields."*
   - 🎙️ *"Plot a neutron star highway route to Colonia."*
