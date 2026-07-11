# Gemini Code Assistant & Bundler

A lightweight, powerful command-line tool built in Go that bundles your codebase context and interacts with the Google Gemini API (`gemini-3.5-flash`) to generate, modify, and automatically apply file changes directly back to your project.

## Features

- **Codebase Bundling**: Automatically walks your project directory, filters out binaries and dependency directories (`vendor`, `.git`, `node_modules`, etc.), and bundles target source files (`.go`, `.mod`, `.sum`, `.json`, `.yaml`, `.md`, etc.) into a clean Markdown representation to use as prompt context.
- **Inclusion & Exclusion Filters**: Explicitly filter which files are bundled into the codebase context using comma-separated pattern lists (e.g., only include specific files, or exclude certain directories/tests).
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

## Usage Instructions

First, build the executable (optional but recommended for convenient CLI use):
```bash
go build -o gemini-assistant .
```

### 1. Send a Code Generation or Modification Prompt
Send an instruction prompt along with your entire bundled codebase context directly to Gemini. This will automatically update your project files and suggest a conventional Git commit message:

```bash
./gemini-assistant -p "Add a new helper function in utility.go to reverse strings"
```

### 2. Run in Verbose Mode
Use the `-v` flag to output additional details, including the raw text response received from the Gemini API and caching metrics:

```bash
./gemini-assistant -v -p "Explain how main.go is structured"
```

### 3. Specify a Custom Project Directory
By default, the assistant scans and works within the current directory (`.`). To point it to a different project or a subdirectory, use the `-d` flag:

```bash
./gemini-assistant -d "/path/to/my/go-project" -p "Refactor the routing package"
```

### 4. Codebase Context Bundler (Dry-Run)
If you want to see exactly what context (source code files, package structures) will be bundled and sent to the Gemini API, use the `-b` flag. This prints the Markdown representation of your repository directly to standard output and **does not** send any requests to the Gemini API:

```bash
./gemini-assistant -b
```

### 5. Bundle with Inclusion & Exclusion Filters
Only bundle specific files (e.g. only bundle `main.go` and files inside `pkg` directory) and ignore everything else:

```bash
./gemini-assistant -include "main.go,pkg" -b
```

Or bundle the whole repository but exclude tests and markdown documentation files:

```bash
./gemini-assistant -exclude "*_test.go,*.md" -p "Optimize memory allocations"
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

### All options
	
	Usage of ./gemini-assistant:
	  -b	bundle all project files without sending
	  -d string
		project directory path (default ".")
	  -exclude string
		comma-separated list of file/directory patterns to exclude from bundling
	  -include string
		comma-separated list of file/directory patterns to include in bundling (ignores all other files)
	  -l	list models
	  -no-cache
		ignore previously cached response and force fresh request
	  -p string
		prompt
	  -v	verbose output (print raw response text)
	  -version
		print version/git revision and exit

---

## How It Works Under the Hood

1. **Context Collection**: The assistant scans your local directory, skipping files and directories defined in `bundle.go` (e.g., `.git`, `node_modules`, `vendor`).
2. **Context Compression**: Found text-based source files are packaged into a structured Markdown string containing path declarations and fenced code blocks.
3. **API Dispatch**: The system prompt instructs Gemini to output code changes using strict `### File: path/to/file` blocks.
4. **File Application**: Upon receiving a response, the assistant automatically parses out modified files and writes them directly back to your project directory.
5. **Git Commit Suggestion**: If the AI proposed a conventional commit message, it's saved locally into `proposed-cm~.txt`. You can quickly commit with:
   ```bash
   git commit -F proposed-cm~.txt
   ```
6. **Smart Caching**: Every unique prompt hashes to a cache file inside `~/.cache/airesponses/gemini-3.5-flash/`. Sending the exact same prompt again reads directly from this cache instantaneously, saving your API quota.
