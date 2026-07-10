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

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

var APIKey = ""

var listModels = flag.Bool("l", false, "list models")
var prompt = flag.String("p", "", "prompt")
var bundle = flag.Bool("b", false, "bundle all project files without sending")
var verbose = flag.Bool("v", false, "verbose output (print raw response text)")
var projectDir = flag.String("d", ".", "project directory path")

func main() {
	flag.Parse()

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
					"### File: `path/to/file.ext`\n" +
					"```language\n" +
					"[file content]\n" +
					"```\n\n" +
					"This marker structure is strictly parsed by automation tools to save changes directly to disk. Do not omit the '### File: `path`' marker or change the backticks formatting under any circumstances.\n\n" +
					codebaseContext,
			},
		},
	}

	result, err := client.Models.GenerateContent(
		ctx,
		modelName,
		genai.Text(prompt),
		&genai.GenerateContentConfig{
			SystemInstruction: systemInstruction,
		},
	)
	if err != nil {
		log.Fatal(err)
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

	log.Println(len(buf), "bytes saved to", fname)

	printResponse(result)
}

func printResponse(resp *genai.GenerateContentResponse) {
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
				return
			}
		}
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
		fmt.Printf("%d %s\n    %s\n    Actions: %v\n\n", i+1, model.Name, model.Description, model.SupportedActions)
	}
}
