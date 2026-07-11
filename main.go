package main

import (
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

func main() {
	flag.Parse()

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

	if *bundle {
		projectContext, err := BundleProject(*projectDir)
		if err != nil {
			log.Fatalf("Error: %v\n", err)
			return
		}
		fmt.Println(projectContext)
		return
	}

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

	buf, err := os.ReadFile(fname)
	if err == nil {
		log.Println("cached result: ", fname)
		var resp genai.GenerateContentResponse
		if err := json.Unmarshal(buf, &resp); err != nil {
			log.Fatal(err)
		}
		if *verbose {
			fmt.Println(resp.Text())
		}
		printResponse(&resp)
		return
	}

	ctx := context.Background()
	conf := genai.ClientConfig{
		APIKey: APIKey,
	}
	client, err := genai.NewClient(ctx, &conf)
	if err != nil {
		log.Fatal(err)
	}

	codebaseContext, err := BundleProject(*projectDir)
	if err != nil {
		log.Fatalf("Code bundling failed: %v", err)
	}

	systemInstruction := &genai.Content{
		Parts: []*genai.Part{
			{
				Text: "You are an expert Go developer assistant.\n\n" +
					"CRITICAL FORMATTING RULE:\n" +
					"Whenever you create, modify, or output file contents in your response, you MUST always format each file using the exact block structure below:\n\n" +
					"### " + "File: `path/to/file.ext`\n" +
					"``" + "`language\n" +
					"[file content]\n" +
					"``" + "`\n" +
					"### " + "End of file: `path/to/file.ext`\n\n" +
					"This marker structure is strictly parsed by automation tools to save changes directly to disk. Do not omit the '### " + "File: `path`' or '### " + "End of file: `path`' markers or change the backticks formatting under any circumstances.\n\n" +
					"PROPOSED COMMIT MESSAGE RULE:\n" +
					"If you suggest creating or modifying any files, you MUST also provide a brief, conventional commit message describing the changes. " +
					"Format the commit message block exactly as follows at the end of your response:\n\n" +
					"### Proposed " + "commit message:\n" +
					"``" + "`\n" +
					"type(scope): description of changes\n" +
					"``" + "`\n\n" +
					codebaseContext,
			},
		},
	}

	var result *genai.GenerateContentResponse
	maxRetries := 5
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err = client.Models.GenerateContent(
			ctx,
			modelName,
			genai.Text(prompt),
			&genai.GenerateContentConfig{
				SystemInstruction: systemInstruction,
			},
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
		fmt.Println(result.Text())
	}

	buf, err = json.Marshal(result)
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
	printResponse(result)
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
						fmt.Printf("✓ Saved proposed commit message to: %s\n", cmPath)
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
