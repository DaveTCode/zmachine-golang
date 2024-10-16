package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davetcode/goz/zmachine"
	"github.com/muesli/reflow/wordwrap"
)

var (
	romFilePath string

	appStyle = lipgloss.NewStyle()

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	statusMessageStyle = appStyle.Reverse(true)
)

type textUpdateMessage string
type stateUpdateMessage zmachine.StateChangeRequest
type StatusBarMessage zmachine.StatusBar

type appState int

const (
	appRunning         appState = iota
	appWaitingForInput appState = iota
)

type applicationModel struct {
	textChannel        <-chan string
	stateChangeChannel <-chan zmachine.StateChangeRequest
	sendChannel        chan<- string
	statusBarChannel   <-chan zmachine.StatusBar
	zMachine           *zmachine.ZMachine
	statusBar          zmachine.StatusBar
	outputText         string
	appState           appState
	inputBox           textinput.Model
	width              int
	height             int
}

func (m applicationModel) Init() tea.Cmd {
	return tea.Batch(
		waitForText(m.textChannel),
		waitForStateChange(m.stateChangeChannel),
		waitForStatusBar(m.statusBarChannel),
		runInterpreter(m.zMachine),
		tea.SetWindowTitle(romFilePath),
	)
}

func runInterpreter(z *zmachine.ZMachine) tea.Cmd {
	return func() tea.Msg {
		for {
			z.StepMachine()
		}
	}
}

func (m applicationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg: // Handle window resize events
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			os.Exit(0)
		}

		switch msg.Type {
		case tea.KeyEnter: // TODO - Some versions have different keys which trigger this
			m.appState = appRunning
			m.outputText += m.inputBox.Value() + "\n"
			m.sendChannel <- m.inputBox.Value()
			m.inputBox.SetValue("")
			return m, nil
		}

	case textUpdateMessage:
		m.outputText += string(msg)
		return m, waitForText(m.textChannel)

	case stateUpdateMessage:
		switch zmachine.StateChangeRequest(msg) {
		case zmachine.WaitForInput:
			m.appState = appWaitingForInput
		case zmachine.Running:
			m.appState = appRunning
		}
		return m, waitForStateChange(m.stateChangeChannel)

	case StatusBarMessage:
		m.statusBar = zmachine.StatusBar(msg)
		return m, waitForStatusBar(m.statusBarChannel)
	}

	if m.appState == appWaitingForInput {
		m.inputBox, cmd = m.inputBox.Update(msg)
	}

	return m, cmd
}

func createStatusLine(width int, placeName string, scoreOrHours int, movesOrMinutes int, isTimeBasedGame bool) string {
	rightHandSide := fmt.Sprintf("Score: %d    Moves %d", scoreOrHours, movesOrMinutes)

	if isTimeBasedGame {
		rightHandSide = fmt.Sprintf("Time: %d:%d", scoreOrHours, movesOrMinutes)
	}

	// Too narrow to show properly so just show as much of the score/time/moves as we can manage
	if len(rightHandSide) >= width {
		return rightHandSide[:width]
	}

	if len(placeName)+len(rightHandSide)+1 >= width {
		return fmt.Sprintf("%s %s", placeName[:width-len(rightHandSide)-1], rightHandSide)
	}

	numberSpaces := width - len(placeName) - len(rightHandSide)

	return fmt.Sprintf("%s%s%s", placeName, strings.Repeat(" ", numberSpaces), rightHandSide)
}

func (m applicationModel) View() string {
	// Wait until the screen has loaded properly to print anything
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	s := strings.Builder{}

	if m.statusBar.PlaceName != "" {
		s.WriteString(statusMessageStyle.Render(createStatusLine(m.width, m.statusBar.PlaceName, m.statusBar.Score, m.statusBar.Moves, m.statusBar.IsTimeBased)))
		s.WriteString(appStyle.Render("\n"))
	}

	wordWrappedBody := wordwrap.String(appStyle.Render(m.outputText), m.width)

	lines := strings.Split(wordWrappedBody, "\n")

	if len(lines) > m.height-2 {
		lines = lines[len(lines)-m.height+2:]
	}
	s.WriteString(strings.Join(lines, "\n"))

	if m.appState == appWaitingForInput {
		s.WriteString(appStyle.Render(m.inputBox.View()))
	}

	return s.String()
}

func waitForText(sub <-chan string) tea.Cmd {
	return func() tea.Msg {
		return textUpdateMessage(<-sub)
	}
}

func waitForStateChange(sub <-chan zmachine.StateChangeRequest) tea.Cmd {
	return func() tea.Msg {
		return stateUpdateMessage(<-sub)
	}
}

func waitForStatusBar(sub <-chan zmachine.StatusBar) tea.Cmd {
	return func() tea.Msg {
		return StatusBarMessage(<-sub)
	}
}

func init() {
	flag.StringVar(&romFilePath, "rom", "zork1.z1", "The path of a z-machine rom")
	flag.Parse()
}

func newApplicationModel(zMachine *zmachine.ZMachine, inputChannel chan<- string, textOutputChannel <-chan string, stateChangeChannel <-chan zmachine.StateChangeRequest, statusBarChannel <-chan zmachine.StatusBar) applicationModel {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	ti.Prompt = ""

	return applicationModel{
		textChannel:        textOutputChannel,
		sendChannel:        inputChannel,
		stateChangeChannel: stateChangeChannel,
		statusBarChannel:   statusBarChannel,
		zMachine:           zMachine,
		appState:           appRunning,
		inputBox:           ti,
	}
}

func main() {
	zMachineTextChannel := make(chan string)
	zMachineStateChangeChannel := make(chan zmachine.StateChangeRequest)
	zMachineInputChannel := make(chan string)
	zMachineStatusBarChannel := make(chan zmachine.StatusBar)

	romFileBytes, err := os.ReadFile(romFilePath)
	if err != nil {
		panic(err)
	}
	zMachine := zmachine.LoadRom(romFileBytes, zMachineInputChannel, zMachineTextChannel, zMachineStateChangeChannel, zMachineStatusBarChannel)

	appModel := newApplicationModel(zMachine, zMachineInputChannel, zMachineTextChannel, zMachineStateChangeChannel, zMachineStatusBarChannel)
	tui := tea.NewProgram(appModel) //, tea.WithAltScreen())

	if _, err := tui.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
