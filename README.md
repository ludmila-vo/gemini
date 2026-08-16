# Gemini Code Assistant

A lightweight, powerful command-line tool built in Go that interacts with the Google Gemini API (`gemini-3.5-flash`) to generate, modify, and automatically apply file changes directly back to your project based on files listed in `list.txt`.

## Features

- **Selected File Context**: Instead of auto-bundling everything, specify exactly which files to send to the Gemini model in a simple `list.txt` file.
- **Smart Response Parsing**: Uses structured JSON output from Gemini to automatically write new or modified files back to your local workspace securely.
- **Automatic Commit Messages**: Extracts proposed conventional commit messages from the response and saves them to `proposed-cm~.txt` for easy Git commits.
- **Resilience**: Features automatic retry with exponential backoff on `503 Service Unavailable` API errors.
- **Response Caching**: Caches raw API responses in `~/.cache/airesponses/gemini-3.5-flash/` to avoid redundant API billing and speeds up repeated queries.
- **Notifications**: Triggers system-level notifications via `notify-send` on success or failure.

---

## Installation & Setup

### Prerequisites

- **Go**: Version 1.20 or later.
- **Gemini API Key**: Get an API key from Google AI Studio.
- **notify-send** (Optional): Standard on most Linux distributions for desktop notifications.

### Setup

1. Clone or copy this repository to your system.
2. Build the executable:
   ```bash
   go build -o gemini-assistant .
   ```
3. Run the tool once or manually create the config file at `~/.config/gemini-assistant/config`:
   ```env
   GEMINI_API_KEY=your_actual_gemini_api_key_here
   ```

---

## Usage Instructions

### 1. Define files to include in context
Create a file named `list.txt` in your execution directory. List the files you want to pass as context (one per line). Empty lines and lines starting with `#` are ignored:

```text
# Context files for Gemini
main.go
version.go
```

### 2. Send a Code Generation or Modification Prompt
Send an instruction prompt along with your selected files context. This will automatically update your project files and suggest a conventional Git commit message:

```bash
./gemini-assistant -p "Add a new helper function to format dates"
```

### 3. Run in Verbose Mode
Use the `-v` flag to output additional details, including the raw response text and caching details:

```bash
./gemini-assistant -v -p "Explain the current structure of main.go"
```

### 4. Custom Project Directory
By default, the assistant works within the current directory (`.`). You can specify a different project directory using the `-d` flag:

```bash
./gemini-assistant -d "/path/to/my/project" -p "Refactor the routing package"
```

### 5. Bypass Cache
Force a fresh request to the Gemini API, bypassing any previously cached responses:

```bash
./gemini-assistant -no-cache -p "Optimize main.go"
```

### 6. List Available Gemini Models
List all accessible models via your Gemini API key, including descriptions, input/output token limits, and supported actions:

```bash
./gemini-assistant -l
```

### 7. Show Version
Display current build version information and Git VCS revisions:

```bash
./gemini-assistant -version
```

### All CLI Options

```text
Usage of ./gemini-assistant:
  -d string
        project directory path (default ".")
  -l    list models
  -no-cache
        ignore previously cached response and force fresh request
  -p string
        prompt
  -v    verbose output (print raw response text)
  -version
        print version/git revision and exit
```

---

## How It Works Under the Hood

1. **Context Collection**: The assistant reads files listed in `list.txt` from your local directory.
2. **API Dispatch**: The prompt and file contents are sent to the Gemini API with instructions to return a structured JSON response containing the file changes.
3. **File Application**: Upon receiving a response, the assistant automatically parses the JSON and writes new or modified files directly back to your project workspace.
4. **Git Commit Suggestion**: If the AI proposed a conventional commit message, it's saved locally into `proposed-cm~.txt`. You can commit with:
   ```bash
   git commit -F proposed-cm~.txt
   ```
5. **Smart Caching**: Every unique prompt hashes to a cache file inside `~/.cache/airesponses/gemini-3.5-flash/`. Sending the exact same prompt again reads directly from this cache.
