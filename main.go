package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

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

type applicationModel struct {
	receiveChannel <-chan string
	sendChannel    chan<- string
	zMachine       *ZMachine
	outputText     string
}

func (m applicationModel) Init() tea.Cmd {
	return tea.Batch(
		waitForText(m.receiveChannel),
		runInterpreter(m.zMachine),
		//tea.SetWindowTitle(romFilePath),
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
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	case textUpdateMessage:
		m.outputText += string(msg)
		return m, waitForText(m.receiveChannel)
	}

	return m, nil
}

func (m applicationModel) View() string {
	s := strings.Builder{}

	s.WriteString(appStyle.Render(m.outputText))

	return s.String()
}

func waitForText(sub <-chan string) tea.Cmd {
	return func() tea.Msg {
		return textUpdateMessage(<-sub)
	}
}

func init() {
	flag.StringVar(&romFilePath, "rom", "zork1.z1", "The path of a z-machine rom")
	flag.Parse()
}

func main() {
	zMachineOutputChannel := make(chan string)
	zMachineInputChannel := make(chan string)

	romFileBytes, err := os.ReadFile(romFilePath)
	if err != nil {
		panic(err)
	}
	zMachine := LoadRom(romFileBytes, zMachineInputChannel, zMachineOutputChannel)

	appModel := applicationModel{
		receiveChannel: zMachineOutputChannel,
		sendChannel:    zMachineInputChannel,
		zMachine:       zMachine,
	}
	tui := tea.NewProgram(appModel)

	if _, err := tui.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
