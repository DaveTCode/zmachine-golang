package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	romFilePath string

	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	statusMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#04B575"}).
				Render
)

type textUpdateMessage string
type stateUpdateMessage stateChangeRequest

type stateChangeRequest int
type appState int

const (
	waitForInput stateChangeRequest = iota
	running      stateChangeRequest = iota
)

const (
	appRunning         appState = iota
	appWaitingForInput appState = iota
)

type applicationModel struct {
	textChannel        <-chan string
	stateChangeChannel <-chan stateChangeRequest
	sendChannel        chan<- string
	zMachine           *ZMachine
	outputText         string
	appState           appState
	inputBox           textinput.Model
}

func (m applicationModel) Init() tea.Cmd {
	return tea.Batch(
		waitForText(m.textChannel),
		waitForStateChange(m.stateChangeChannel),
		runInterpreter(m.zMachine),
		tea.SetWindowTitle(romFilePath),
	)
}

func runInterpreter(z *ZMachine) tea.Cmd {
	return func() tea.Msg {
		for {
			z.StepMachine()
		}
	}
}

func (m applicationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter: // TODO - Some versions have different keys which trigger this
			m.appState = appRunning
			m.sendChannel <- m.inputBox.Value()
			m.inputBox.SetValue("")
			return m, nil
		}
	case textUpdateMessage:
		m.outputText += string(msg)
		return m, waitForText(m.textChannel)
	case stateUpdateMessage:
		switch stateChangeRequest(msg) {
		case waitForInput:
			// TODO - Refresh status bar here (maybe version dependent?)
			m.appState = appWaitingForInput
		case running:
			m.appState = appRunning
		}
		return m, waitForStateChange(m.stateChangeChannel)
	}

	if m.appState == appWaitingForInput {
		m.inputBox, cmd = m.inputBox.Update(msg)
	}

	return m, cmd
}

func (m applicationModel) View() string {
	s := strings.Builder{}

	s.WriteString(appStyle.Render(m.outputText))

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

func waitForStateChange(sub <-chan stateChangeRequest) tea.Cmd {
	return func() tea.Msg {
		return stateUpdateMessage(<-sub)
	}
}

func init() {
	flag.StringVar(&romFilePath, "rom", "zork1.z1", "The path of a z-machine rom")
	flag.Parse()
}

func newApplicationModel(zMachine *ZMachine, inputChannel chan<- string, textOutputChannel <-chan string, stateChangeChannel <-chan stateChangeRequest) applicationModel {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	ti.Prompt = ""

	return applicationModel{
		textChannel:        textOutputChannel,
		sendChannel:        inputChannel,
		stateChangeChannel: stateChangeChannel,
		zMachine:           zMachine,
		appState:           appRunning,
		inputBox:           ti,
	}
}

func main() {
	zMachineTextChannel := make(chan string)
	zMachineStateChangeChannel := make(chan stateChangeRequest)
	zMachineInputChannel := make(chan string)

	romFileBytes, err := os.ReadFile(romFilePath)
	if err != nil {
		panic(err)
	}
	zMachine := LoadRom(romFileBytes, zMachineInputChannel, zMachineTextChannel, zMachineStateChangeChannel)

	appModel := newApplicationModel(zMachine, zMachineInputChannel, zMachineTextChannel, zMachineStateChangeChannel)
	tui := tea.NewProgram(appModel, tea.WithAltScreen())

	if _, err := tui.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
