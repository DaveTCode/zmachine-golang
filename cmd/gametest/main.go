package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/davetcode/goz/zmachine"
)

// TestResult captures the outcome of running a single game
type TestResult struct {
	Filename     string   `json:"filename"`
	Version      uint8    `json:"version"`
	Success      bool     `json:"success"`
	PanicMessage string   `json:"panic_message,omitempty"`
	StackTrace   string   `json:"stack_trace,omitempty"`
	FirstScreen  []string `json:"first_screen,omitempty"`
	ErrorMessage string   `json:"error_message,omitempty"`
}

func main() {
	storiesDir := flag.String("stories", "stories", "Directory containing Z-machine story files")
	outputDir := flag.String("output", "testdata", "Directory to write results to")
	singleGame := flag.String("game", "", "Test a single game file instead of all games")
	flag.Parse()

	if *singleGame != "" {
		runSingleGame(*singleGame)
		return
	}

	runAllGames(*storiesDir, *outputDir)
}

func runAllGames(storiesDir, outputDir string) {
	// Check if stories directory exists
	if _, err := os.Stat(storiesDir); os.IsNotExist(err) {
		fmt.Printf("Stories directory not found: %s\n", storiesDir)
		fmt.Println("Run 'go run ./cmd/scraper' first to download games.")
		os.Exit(1)
	}

	// Find all game files
	entries, err := os.ReadDir(storiesDir)
	if err != nil {
		fmt.Printf("Failed to read stories directory: %v\n", err)
		os.Exit(1)
	}

	var games []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".z1") || strings.HasSuffix(name, ".z2") ||
			strings.HasSuffix(name, ".z3") || strings.HasSuffix(name, ".z4") ||
			strings.HasSuffix(name, ".z5") || strings.HasSuffix(name, ".z6") ||
			strings.HasSuffix(name, ".z7") || strings.HasSuffix(name, ".z8") {
			games = append(games, filepath.Join(storiesDir, name))
		}
	}

	if len(games) == 0 {
		fmt.Printf("No game files found in %s\n", storiesDir)
		os.Exit(1)
	}

	fmt.Printf("Found %d games to test\n", len(games))

	var results []TestResult

	for i, gamePath := range games {
		filename := filepath.Base(gamePath)
		result := runGameTest(gamePath)
		results = append(results, result)

		status := "✓"
		if !result.Success {
			status = "✗"
		}
		fmt.Printf("[%d/%d] %s %s\n", i+1, len(games), status, filename)
		if !result.Success && result.ErrorMessage != "" {
			fmt.Printf("        Error: %s\n", result.ErrorMessage)
		}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Write results to JSON file
	resultsPath := filepath.Join(outputDir, "test_results.json")
	resultsJSON, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(resultsPath, resultsJSON, 0644); err != nil {
		fmt.Printf("Failed to write results: %v\n", err)
	} else {
		fmt.Printf("\nResults written to %s\n", resultsPath)
	}

	// Write summary
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Success {
			passed++
		} else {
			failed++
		}
	}
	fmt.Printf("\n=== SUMMARY ===\nPassed: %d\nFailed: %d\nTotal: %d\n", passed, failed, len(results))

	// Write screenshots to a separate file
	screenshotsPath := filepath.Join(outputDir, "screenshots.txt")
	var screenshots strings.Builder
	for _, r := range results {
		fmt.Fprintf(&screenshots, "=== %s (v%d) ===\n", r.Filename, r.Version)
		if r.Success {
			for _, line := range r.FirstScreen {
				screenshots.WriteString(line + "\n")
			}
		} else {
			fmt.Fprintf(&screenshots, "ERROR: %s\n", r.ErrorMessage)
			if r.PanicMessage != "" {
				fmt.Fprintf(&screenshots, "PANIC: %s\n", r.PanicMessage)
			}
		}
		screenshots.WriteString("\n")
	}
	os.WriteFile(screenshotsPath, []byte(screenshots.String()), 0644) // nolint:errcheck
}

func runSingleGame(gamePath string) {
	if _, err := os.Stat(gamePath); os.IsNotExist(err) {
		fmt.Printf("Game file not found: %s\n", gamePath)
		os.Exit(1)
	}

	result := runGameTest(gamePath)

	fmt.Printf("Game: %s\n", result.Filename)
	fmt.Printf("Version: %d\n", result.Version)
	fmt.Printf("Success: %v\n", result.Success)

	if result.PanicMessage != "" {
		fmt.Printf("Panic: %s\n", result.PanicMessage)
		fmt.Printf("Stack: %s\n", result.StackTrace)
	}

	if result.ErrorMessage != "" {
		fmt.Printf("Error: %s\n", result.ErrorMessage)
	}

	fmt.Printf("First Screen:\n%s\n", strings.Join(result.FirstScreen, "\n"))
}

func runGameTest(gamePath string) (result TestResult) {
	filename := filepath.Base(gamePath)
	result.Filename = filename

	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			result.Success = false
			result.PanicMessage = fmt.Sprintf("%v", r)
			result.StackTrace = string(debug.Stack())
		}
	}()

	// Load the game file
	storyBytes, err := os.ReadFile(gamePath)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Failed to read file: %v", err)
		return
	}

	// Basic validation - check minimum size for header
	if len(storyBytes) < 64 {
		result.Success = false
		result.ErrorMessage = "File too small to be a valid Z-machine file"
		return
	}

	result.Version = storyBytes[0]

	// Create channels
	outputChannel := make(chan any, 100)
	inputChannel := make(chan zmachine.InputResponse, 10)
	saveRestoreChannel := make(chan zmachine.SaveRestoreResponse, 10)

	// Load the Z-machine
	z := zmachine.LoadRom(storyBytes, inputChannel, saveRestoreChannel, outputChannel)

	// Commands to try - these are common adventure game commands that should
	// exercise various parts of the interpreter
	commands := []string{
		// Initial prompts - some games need multiple enters/spaces to start
		" ",
		" ",
		"",
		"",
		"",

		// Yes/No responses (common for game prompts)
		"y",
		"yes",
		"n",
		"no",

		// Help and meta commands
		"help",
		"about",
		"info",
		"hint",
		"hints",
		"score",
		"version",
		"credits",
		"verbose",
		"brief",
		"superbrief",

		// Movement commands
		"north",
		"south",
		"east",
		"west",
		"northeast",
		"northwest",
		"southeast",
		"southwest",
		"up",
		"down",
		"in",
		"out",
		"enter",
		"exit",
		"go north",
		"n",
		"s",
		"e",
		"w",
		"ne",
		"nw",
		"se",
		"sw",
		"u",
		"d",

		// Looking and examining
		"look",
		"l",
		"look around",
		"examine self",
		"examine me",
		"x me",
		"inventory",
		"i",
		"look at floor",
		"examine room",
		"search",
		"search room",

		// Object interaction
		"take all",
		"get all",
		"drop all",
		"take everything",
		"pick up all",
		"put all in bag",
		"open door",
		"close door",
		"open all",
		"close all",
		"push button",
		"pull lever",
		"turn knob",
		"move rug",
		"lift rug",
		"read sign",
		"read book",
		"read note",
		"read all",

		// Common object names
		"take lamp",
		"take sword",
		"take key",
		"take book",
		"take coin",
		"take food",
		"take bottle",
		"drop lamp",
		"drop sword",
		"drop key",

		// Using objects
		"use key",
		"use lamp",
		"light lamp",
		"turn on lamp",
		"turn off lamp",
		"eat food",
		"drink water",
		"wear cloak",
		"remove cloak",

		// Combat and interaction with NPCs
		"attack troll",
		"kill troll with sword",
		"hit monster",
		"talk to man",
		"ask man about key",
		"tell man about treasure",
		"give coin to man",
		"show book to woman",
		"follow thief",

		// Complex commands
		"put key in lock",
		"unlock door with key",
		"open door with key",
		"tie rope to hook",
		"throw ball at window",
		"climb tree",
		"climb up",
		"climb down",
		"jump",
		"swim",
		"wait",
		"z",
		"sleep",
		"wake up",
		"stand",
		"sit",
		"lie down",

		// Container manipulation
		"open box",
		"close box",
		"look in box",
		"search box",
		"empty bag",
		"fill bottle with water",

		// Unusual inputs to stress test parser
		"xyzzy",
		"plugh",
		"plover",
		"frotz",
		"hello",
		"hello sailor",
		"damn",
		"pray",
		"sing",
		"dance",
		"think",
		"smell",
		"listen",
		"taste wall",
		"touch wall",
		"feel wall",

		// Numbers (some games ask for numeric input)
		"0",
		"1",
		"42",
		"100",
		"999",

		// Edge cases
		"   ", // Just spaces
		"a",   // Single letter
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // Long input
		"take the small brass lamp from the table",                                         // Long command
		"n. n. n. e. e. s.", // Multiple commands (some parsers support this)
		"north then east",

		// Save/restore/quit (usually intercepted)
		"save",
		"restore",
		"load",
		"restart",
		"undo",
		"quit",
		"q",
	}
	commandIndex := 0

	// Collect output until we hit input request or timeout
	var screenOutput []string
	done := make(chan bool)
	timeout := time.After(30 * time.Second) // Longer timeout for multiple commands

	go func() {
		defer func() {
			if r := recover(); r != nil {
				result.Success = false
				result.PanicMessage = fmt.Sprintf("Panic in Run: %v", r)
				result.StackTrace = string(debug.Stack())
				done <- true
			}
		}()
		z.Run()
		done <- true
	}()

	collectOutput := true
	lastCommand := "(initial startup)"
	for collectOutput {
		select {
		case msg := <-outputChannel:
			switch v := msg.(type) {
			case string:
				// Collect text output
				lines := strings.Split(v, "\n")
				screenOutput = append(screenOutput, lines...)
			case zmachine.InputRequest:
				// Game is waiting for line input - send next command
				if commandIndex < len(commands) {
					lastCommand = commands[commandIndex]
					inputChannel <- zmachine.InputResponse{Text: commands[commandIndex], TerminatingKey: 13}
					commandIndex++
				} else {
					// We've sent all commands, stop collecting
					collectOutput = false
				}
			case zmachine.StateChangeRequest:
				if v == zmachine.WaitForCharacter {
					// Game is waiting for character input - send next command
					if commandIndex < len(commands) {
						lastCommand = commands[commandIndex]
						inputChannel <- zmachine.InputResponse{Text: commands[commandIndex], TerminatingKey: 13}
						commandIndex++
					} else {
						// We've sent all commands, stop collecting
						collectOutput = false
					}
				}
			case zmachine.Save:
				// For testing, always respond with failure (not saving)
				saveRestoreChannel <- zmachine.SaveResponse{Success: false, Result: 0}
			case zmachine.Restore:
				// For testing, always respond with failure (no save file)
				saveRestoreChannel <- zmachine.RestoreResponse{Success: false, Result: 0}
			case zmachine.Quit:
				collectOutput = false
			case zmachine.Restart:
				collectOutput = false
			case zmachine.RuntimeError:
				result.Success = false
				result.ErrorMessage = fmt.Sprintf("After command %d %q: %s", commandIndex, lastCommand, string(v))
				return
			}
		case <-timeout:
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("Timeout after command %d %q", commandIndex, lastCommand)
			return
		case <-done:
			collectOutput = false
		}
	}

	result.Success = true
	result.FirstScreen = screenOutput
	return
}
