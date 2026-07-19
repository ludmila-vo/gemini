package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

var APIKey = ""

var listModels = flag.Bool("l", false, "list models")
var prompt = flag.String("p", "", "prompt")
var bundle = flag.Bool("b", false, "bundle all project files without sending")
var verbose = flag.Bool("v", false, "verbose output (print raw response text)")
var projectDir = flag.String("d", ".", "project directory path")
var showVersion = flag.Bool("version", false, "print version/git revision and exit")
var excludePatterns = flag.String("exclude", "", "comma-separated list of file/directory patterns to exclude from bundling")
var includePatterns = flag.String("include", "", "comma-separated list of file/directory patterns to include in bundling (ignores all other files)")
var noCache = flag.Bool("no-cache", false, "ignore previously cached response and force fresh request")

func parsePatterns(raw string) []string {
	var parsed []string
	if raw == "" {
		return parsed
	}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parsed = append(parsed, p)
		}
	}
	return parsed
}

type FileResult struct {
	Filename    string `json:"filename" description:"The name of the file with extension, e.g., main.go"`
	CodeContent string `json:"code_content" description:"The complete, valid Go source code for this file"`
}

type MultipleFilesResponse struct {
	Description           string       `json:"description"`
	ProposedCommitMessage string       `json:"proposed_commit_message"`
	Files                 []FileResult `json:"files" description:"List of generated or modified Go files"`
}

func main() {
	flag.Parse()

	if len(os.Args) == 1 {
		flag.Usage()
		return
	}

	if *showVersion {
		printVersion()
		return
	}

	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: No .env file found, falling back to system env")
	}

	APIKey = os.Getenv("GEMINI_API_KEY")
	if APIKey == "" {
		log.Fatal("Critical: GEMINI_API_KEY is not set in the environment or .env file")
	}

	if *listModels {
		listGeminiModels()
		return
	}

	//	parsedExcludes := parsePatterns(*excludePatterns)
	//	parsedIncludes := parsePatterns(*includePatterns)

	if *prompt == "" {
		log.Fatal("no prompt specified")
		return
	}

	modelName := "gemini-3.5-flash"
	prompt := *prompt

	log.Println(modelName+":", prompt)

	dir := os.Getenv("HOME") + "/.cache/airesponses/" + modelName
	hashBytes := sha256.Sum256([]byte(prompt))
	hashString := hex.EncodeToString(hashBytes[:])
	fname := dir + "/" + hashString + ".json"

	var buf []byte
	if !*noCache {
		buf, err = os.ReadFile(fname)
		if err == nil {
			log.Println("cached result: ", fname)
			var resp genai.GenerateContentResponse
			if err := json.Unmarshal(buf, &resp); err != nil {
				log.Fatal(err)
			}
			if *verbose {
				fmt.Println(resp.Text())
			}
			//			printResponse(&resp)
			return
		}
	}

	ctx := context.Background()
	conf := genai.ClientConfig{
		APIKey: APIKey,
	}
	client, err := genai.NewClient(ctx, &conf)
	if err != nil {
		log.Fatal(err)
	}

	text := "You are an expert golang developer assistant.\n\n" +
		"If you suggest creating or modifying any files, you MUST also provide a brief, conventional commit message describing the changes into proposed_commit_message.\n" +
		"Format the commit message block exactly as: type(scope): description of changes\n" +
		"Provide changes details into description." +
		"What to do: " + prompt

	if *verbose {
		fmt.Println("============================== text", text)
		fmt.Println(text)
	}

	parts := []*genai.Part{
		{
			Text: text,
		},
	}

	filePaths, _ := loadPathsFromListFile("list.txt")

	for _, path := range filePaths {
		fileBytes, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("Failed to read file %s: %v", path, err)
		}

		// Optional: Add a text header before each file to tell the model its name
		parts = append(parts, &genai.Part{
			Text: fmt.Sprintf("--- File: %s ---", path),
		})

		// Append the actual file content
		parts = append(parts, &genai.Part{
			InlineData: &genai.Blob{
				Data:     fileBytes,
				MIMEType: "text/plain",
			},
		})
		log.Println("added", path)
	}

	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"description":             {},
				"proposed_commit_message": {},
				"files": {
					Type: genai.TypeArray,
					Items: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"filename":    {Type: genai.TypeString},
							"codeContent": {Type: genai.TypeString},
						},
						Required: []string{"filename", "codeContent"},
					},
				},
			},
			Required: []string{"description", "files"},
		},
	}

	var resp *genai.GenerateContentResponse
	maxRetries := 5
	backoff := 1 * time.Second

	contents := []*genai.Content{{Parts: parts}}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err = client.Models.GenerateContent(
			ctx,
			modelName,
			contents,
			config,
		)
		if err == nil {
			break
		}

		errStr := err.Error()
		isUnavailable := strings.Contains(errStr, "503") || strings.Contains(strings.ToUpper(errStr), "UNAVAILABLE")

		if isUnavailable && attempt < maxRetries {
			log.Printf("Warning: API returned 503 UNAVAILABLE. Retrying in %v... (Attempt %d/%d)", backoff, attempt, maxRetries)
			select {
			case <-ctx.Done():
				sendNotification("Gemini Call Failed", ctx.Err().Error())
				log.Fatal(ctx.Err())
			case <-time.After(backoff):
			}
			backoff *= 2
			continue
		}

		sendNotification("Gemini Call Failed", err.Error())
		log.Fatalf("API call failed: %v", err)
	}

	if *verbose {
		fmt.Println(resp.Text())
	}

	buf, err = json.Marshal(resp)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(fname, buf, 0600); err != nil {
		log.Fatal(err)
	}

	if *verbose {
		log.Println(len(buf), "bytes saved to", fname)
	}

	var output MultipleFilesResponse
	err = json.Unmarshal([]byte(resp.Text()), &output)
	if err != nil {
		log.Fatalf("Failed to parse JSON response: %v\nRaw response: %s", err, resp.Text())
	}

	fmt.Printf("%+v", output)
}

// loadPathsFromListFile opens a file and extracts non-empty lines
func loadPathsFromListFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Ignore empty lines and comment lines (starting with #)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		paths = append(paths, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return paths, nil
}

func printResponse(resp *genai.GenerateContentResponse) {
	var writtenFiles []string
	for i, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for j, part := range cand.Content.Parts {
			if *verbose {
				fmt.Println("================= candidate", i, "part", j)
				fmt.Println(part.Text)
			}
			files := ExtractFilesFromMarkdown(part.Text)
			err := WriteFilesToDisk(*projectDir, files)
			if err != nil {
				fmt.Printf("Critical Error saving files: %v\n", err)
				sendNotification("Gemini Task Failed", fmt.Sprintf("Error writing files: %v", err))
				return
			}
			for _, f := range files {
				writtenFiles = append(writtenFiles, f.Name)
			}

			if len(files) > 0 {
				commitMsg := ExtractCommitMessage(part.Text)
				if commitMsg != "" {
					fmt.Println("\n### Proposed " + "commit message:")
					fmt.Println(commitMsg)

					cmPath := filepath.Join(*projectDir, "proposed-cm~.txt")
					err := os.WriteFile(cmPath, []byte(commitMsg+"\n"), 0644)
					if err != nil {
						fmt.Printf("Warning: failed to write commit message to %s: %v\n", cmPath, err)
					} else {
						fmt.Printf("Saved proposed commit message to: %s\n", cmPath)
					}
				}
			}
		}
	}

	summary := "Gemini prompt completed"
	body := "Response processed successfully."
	if len(writtenFiles) > 0 {
		body = fmt.Sprintf("Updated files:\n%s", strings.Join(writtenFiles, "\n"))
	}
	sendNotification(summary, body)
}

func sendNotification(summary, body string) {
	cmd := exec.Command("notify-send", summary, body)
	err := cmd.Run()
	if err != nil && *verbose {
		log.Printf("notify-send error: %v", err)
	}
}

func listGeminiModels() {
	ctx := context.Background()
	conf := genai.ClientConfig{
		APIKey: APIKey,
	}
	client, err := genai.NewClient(ctx, &conf)
	if err != nil {
		log.Fatal(err)
	}

	page, err := client.Models.List(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	for i, model := range page.Items {
		displayName := model.DisplayName
		if displayName == "" {
			displayName = "N/A"
		}
		fmt.Printf("%d %s (%s)\n", i+1, model.Name, displayName)
		fmt.Printf("    Description: %s\n", model.Description)
		fmt.Printf("    Input Token Limit:  %d\n", model.InputTokenLimit)
		fmt.Printf("    Output Token Limit: %d\n", model.OutputTokenLimit)
		fmt.Printf("    Supported Actions:  %v\n\n", model.SupportedActions)
	}
}
