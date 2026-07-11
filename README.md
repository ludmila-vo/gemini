# Gemini Code Assistant & Bundler

A lightweight, powerful command-line tool built in Go that bundles your codebase context and interacts with the Google Gemini API (`gemini-3.5-flash`) to generate, modify, and automatically apply file changes directly back to your project.

## Features

- **Codebase Bundling**: Automatically walks your project directory, filters out binaries and dependency directories (`vendor`, `.git`, `node_modules`, etc.), and bundles target source files (`.go`, `.mod`, `.sum`, `.json`, `.yaml`, `.md`, etc.) into a clean Markdown representation to use as prompt context.
- **Smart Response Parsing**: Parses special formatting markers from the Gemini model response and automatically writes new or modified files back to your local workspace securely.
- **Automatic Commit Messages**: Extracts proposed conventional commit messages from the response and saves them to `proposed-cm~.txt` for easy Git commits.
- **Resilience**: Features automatic retry with exponential backoff on `503 Service Unavailable` API errors.
- **Response Caching**: Caches raw API responses in `~/.cache/airesponses/gemini-3.5-flash/` to avoid redundant API billing and speeds up repeated queries.
- **Notifications**: Triggers system-level notifications via `notify-send` on success or failure.

---

## Installation & Setup

### Prerequisites

- **Go**: Version 1.26 or later.
- **Gemini API Key**: Get an API key from Google AI Studio.
- **notify-send** (Optional): Standard on most Linux distributions for desktop notifications.

### Setup

1. Clone or copy this repository to your system.
2. Initialize and download dependencies:
   ```bash
   go mod download
   ```
3. Create a `.env` file in the root directory (or set it in your environment shell):
   ```env
   GEMINI_API_KEY=your_actual_gemini_api_key_here
   ```

---

## Usage

Run the tool using `go run .` or build a binary using `go build -o gemini-assistant`.

### 1. Send a Code Generation Prompt
Send a instruction prompt along with your entire bundled project codebase context to Gemini:

	./gemini-assistant -p 'Add option to show version'
